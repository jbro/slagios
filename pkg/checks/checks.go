package checks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/shlex"
)

const defaultCheckInterval = "60s"

type serviceState int

func (s serviceState) String() string {
	return [...]string{"OK", "Warning", "Critical", "Unknown"}[s]
}
func (s serviceState) emoji() string {
	return [...]string{":large_green_circle:", ":large_yellow_circle:",
		":red_circle:", ":large_purple_circle:"}[s]
}

const (
	ok       serviceState = 0
	warning               = 1
	critical              = 2
	unknown               = 3
)

type check struct {
	name     string
	command  string
	output   string
	state    serviceState
	checknow chan bool
	schedule *time.Ticker
	interval time.Duration
}

func newCheck(name string, cmd string) *check {
	defaultDuration := time.Hour
	return &check{name, cmd, "", ok, make(chan bool), time.NewTicker(defaultDuration), defaultDuration}
}

func (c *check) notify(oldState serviceState) {
	if webhook, ok := os.LookupEnv("SLAGIOS_webhook"); ok {
		serviceText := c.output
		serviceText = strings.Split(serviceText, "\n")[0]
		serviceText = strings.SplitN(serviceText, "|", 2)[0]
		serviceTextJSON, _ := json.Marshal(fmt.Sprintf("Check output: `%s`", serviceText))

		commandJSON, _ := json.Marshal(fmt.Sprintf("Check command: `%s`", c.command))

		intervalJSON, _ := json.Marshal(fmt.Sprintf("Check interval: %s", c.interval.String()))

		buf := strings.NewReader(fmt.Sprintf(stateChangeTemplate,
			c.name, c.state.emoji(), string(commandJSON), string(serviceTextJSON), string(intervalJSON)))
		http.Post(webhook, "application/json", buf)
	}
}

func (c *check) resetInterval() {
	interval := defaultCheckInterval

	if c.state == ok {
		if value, ok := os.LookupEnv("SLAGIOS_interval"); ok {
			interval = value
		}
		if value, ok := os.LookupEnv(strings.Replace(c.name, "check", "interval", 1)); ok {
			interval = value
		}
	} else {
		if value, ok := os.LookupEnv("SLAGIOS_rinterval"); ok {
			interval = value
		}
		if value, ok := os.LookupEnv(strings.Replace(c.name, "check", "rinterval", 1)); ok {
			interval = value
		}
	}

	dur, err := time.ParseDuration(interval)
	if err != nil {
		log.Panicf("Could not parse duration: \"%s\" for %s", interval, c.name)
	}

	c.interval = dur
	c.schedule.Reset(dur)
}

func (c *check) run() {
	s, err := shlex.Split(c.command)
	if err != nil {
		log.Panicf("Could not parse command line: \"%s\" for %s %s",
			c.command, c.name, err)
	}

	prvState := c.state

	log.Printf("Running %s: %s", c.name, c.command)
	cmd := exec.Command(s[0], s[1:]...)
	out, _ := cmd.Output()

	c.output = string(out)
	c.state = serviceState(cmd.ProcessState.ExitCode())

	if prvState != c.state {
		c.resetInterval()
		log.Printf("State changed %s: %s->%s, rechecking in %s", c.name, prvState, c.state, c.interval)
		c.notify(prvState)
	}
}

func verifyRequest(r *http.Request) bool {
	key := []byte(os.Getenv("SLAGIOS_signingkey"))
	mac := hmac.New(sha256.New, key)

	ts := r.Header.Get("X-Slack-Request-Timestamp")

	its, _ := strconv.ParseInt(ts, 10, 64)
	then := time.Unix(its, 0)
	now := time.Now()

	if now.Sub(then) > 5*time.Minute {
		return false
	}

	body, _ := ioutil.ReadAll(r.Body)
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	base := []byte(fmt.Sprintf("v0:%s:%s", ts, body))
	mac.Write(base)

	sum := mac.Sum(nil)
	signature := strings.TrimPrefix(r.Header.Get("X-Slack-Signature"), "v0=")
	rawsiganture, _ := hex.DecodeString(signature)

	return hmac.Equal(sum, rawsiganture)
}

func slashCmdHandler(w http.ResponseWriter, r *http.Request, checks []*check) {
	switch r.Method {
	case "POST":
		if !verifyRequest(r) {
			w.WriteHeader(http.StatusUnauthorized)
			log.Printf("Unauthorized %s %s %s from %s", r.Method, r.URL.Path, r.Method, r.RemoteAddr)
			return
		}

		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			log.Printf("Could not parse form data %s %s %s from %s", r.Method, r.URL.Path, r.Method, r.RemoteAddr)
			return
		}

		//TODO parse and dispatch request here

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		log.Printf("Unsupported method %s %s %s from %s", r.Method, r.URL.Path, r.Method, r.RemoteAddr)
		return
	}
}

func load() []*check {
	var checks []*check

	for _, e := range os.Environ() {
		p := strings.SplitN(e, "=", 2)
		name := p[0]
		cmd := p[1]

		if strings.HasPrefix(name, "SLAGIOS_check_") {
			c := newCheck(name, cmd)
			checks = append(checks, c)

			log.Printf("Loaded %s: %s", name, cmd)
		}
	}

	return checks
}

func Start() {
	checks := load()

	var wg sync.WaitGroup

	log.Println("Starting schdeuler")
	for _, c := range checks {
		c.resetInterval()

		go func(cc *check) {
			log.Printf("Schdeuled %s: %s", cc.name, cc.command)

			for {
				select {
				case <-cc.schedule.C:
					cc.run()
				case <-cc.checknow:
					cc.run()
				}
			}
		}(c)
		wg.Add(1)

		c.checknow <- true
	}

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			slashCmdHandler(w, r, checks)
		})

		log.Println("Starting slash command listerner on port 80")
		http.ListenAndServe(":80", nil)
	}()

	wg.Wait()
}

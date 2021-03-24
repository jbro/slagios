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
	name      string
	command   string
	output    string
	state     serviceState
	lastCheck time.Time
	checknow  chan bool
	schedule  *time.Ticker
	interval  time.Duration
}

func newCheck(name string, cmd string) *check {
	defaultDuration := time.Hour
	return &check{name, cmd, "", ok, time.Now(), make(chan bool), time.NewTicker(defaultDuration), defaultDuration}
}

func (c *check) notify() {
	if webhook, ok := os.LookupEnv("SLAGIOS_webhook"); ok {
		serviceText := c.output
		serviceText = strings.Split(serviceText, "\n")[0]
		serviceText = strings.SplitN(serviceText, "|", 2)[0]
		serviceTextJSON, _ := json.Marshal(fmt.Sprintf("Check output: `%s`", serviceText))

		commandJSON, _ := json.Marshal(fmt.Sprintf("Check command: `%s`", c.command))

		nextCheck := c.lastCheck.Add(c.interval).Sub(time.Now()).Round(time.Second)
		intervalJSON, _ := json.Marshal(fmt.Sprintf("Lastcheck: %s\nNext check in: %s", c.lastCheck.Format(time.RFC3339), nextCheck))

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

	c.lastCheck = time.Now()

	if prvState != c.state {
		c.resetInterval()
		log.Printf("State changed %s: %s->%s, rechecking in %s", c.name, prvState, c.state, c.interval)
		c.notify()
	}
}

func requestVerifier(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.Method != "POST" {
			http.Error(w, fmt.Sprintf("Method not allowd: %s", r.Method), http.StatusMethodNotAllowed)
			return
		}

		key := []byte(os.Getenv("SLAGIOS_signingkey"))
		mac := hmac.New(sha256.New, key)

		ts := r.Header.Get("X-Slack-Request-Timestamp")

		its, _ := strconv.ParseInt(ts, 10, 64)
		then := time.Unix(its, 0)
		now := time.Now()

		if now.Sub(then) > 5*time.Minute {
			http.Error(w, "Unauthorised: expired", http.StatusUnauthorized)
			return
		}

		body, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		base := []byte(fmt.Sprintf("v0:%s:%s", ts, body))
		mac.Write(base)

		sum := mac.Sum(nil)
		signature := r.Header.Get("X-Slack-Signature")
		signature = strings.TrimPrefix(signature, "v0=")
		rawsiganture, _ := hex.DecodeString(signature)

		if !hmac.Equal(sum, rawsiganture) {
			http.Error(w, "Unauthorised: invalid signature", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type loggerStatusResponseWriter struct {
	http.ResponseWriter
	statusCode int
	response   string
}

func (l *loggerStatusResponseWriter) WriteHeader(statusCode int) {
	l.ResponseWriter.WriteHeader(statusCode)
	l.statusCode = statusCode
}

func (l *loggerStatusResponseWriter) Write(p []byte) (int, error) {
	l.response += strings.TrimSpace(string(p))
	return l.ResponseWriter.Write(p)
}

func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := loggerStatusResponseWriter{w, http.StatusOK, ""}

		next.ServeHTTP(&l, r)

		log.Printf("Request from %s %s %s %s %d \"%s\" \"%s\"", r.RemoteAddr, r.Method, r.RequestURI, r.Proto, l.statusCode, l.response, r.UserAgent())
	})
}

func slashCmdHandler(checks []*check) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request: could not parse form data", http.StatusBadRequest)
			return
		}

		//TODO parse and dispatch request here
	})
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
		handler := slashCmdHandler(checks)
		verifier := requestVerifier(handler)
		logging := logger(verifier)
		http.Handle("/", logging)

		log.Println("Starting slash command listerner on port 80")
		http.ListenAndServe(":80", nil)
	}()

	wg.Wait()
}

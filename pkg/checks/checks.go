package checks

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
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
}

func newCheck(name string, cmd string) *check {
	return &check{name, cmd, "", ok, make(chan bool), time.NewTicker(time.Hour)}
}

func (c *check) notify(oldState serviceState) {
	if os.Getenv("SLAGIOS_webhook") != "" {
		serviceText := c.output
		serviceText = strings.Split(serviceText, "\n")[0]
		serviceText = strings.SplitN(serviceText, "|", 2)[0]
		serviceTextJSON, _ := json.Marshal(fmt.Sprintf("Check output: `%s`", serviceText))

		commandJSON, _ := json.Marshal(fmt.Sprintf("Check command: `%s`", c.command))

		buf := strings.NewReader(fmt.Sprintf(stateChangeTemplate,
			c.name, oldState.emoji(), c.state.emoji(),
			string(commandJSON), string(serviceTextJSON)))
		http.Post(os.Getenv("SLAGIOS_webhook"), "application/json", buf)
	}
}

func (c check) resetInterval() time.Duration {
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

	c.schedule.Reset(dur)

	return dur
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
		newInterval := c.resetInterval()
		log.Printf("State changed %s: %s->%s, rechecking in %s", c.name, prvState, c.state, newInterval)
		c.notify(prvState)
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
		c.checknow <- true
		c.resetInterval()
		wg.Add(1)
	}

	wg.Wait()
}

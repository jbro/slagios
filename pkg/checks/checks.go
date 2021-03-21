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

const checkPrefix = "SLAGIOS_check_"
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
	interval time.Duration
	checknow chan bool
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
		log.Printf("State changed %s: %s->%s", c.name, prvState, c.state)
		c.notify(prvState)
	}
}

func load() []*check {
	var checks []*check

	for _, e := range os.Environ() {
		p := strings.SplitN(e, "=", 2)
		name := p[0]
		cmd := p[1]

		interval := os.Getenv("SLAGIOS_interval")
		if interval == "" {
			interval = defaultCheckInterval
		}

		if strings.HasPrefix(name, checkPrefix) {
			checkInerval := os.Getenv(strings.Replace(name, "check", "interval", 1))
			if checkInerval != "" {
				interval = checkInerval
			}

			dur, err := time.ParseDuration(interval)
			if err != nil {
				log.Panicf("Could not parse duration: \"%s\" for %s", interval, name)
			}

			c := check{name, cmd, "", ok, dur, make(chan bool)}
			checks = append(checks, &c)

			log.Printf("Loaded %s: %s (%s)", name, cmd, dur)
		}
	}

	return checks
}

func Start() {
	checks := load()

	var wg sync.WaitGroup

	log.Println("Starting schdeuler")
	for _, c := range checks {
		ticker := time.NewTicker(c.interval)

		go func(cc *check) {
			log.Printf("Schdeuled %s: %s (%s)", cc.name, cc.command, cc.interval)

			for {
				select {
				case <-ticker.C:
					cc.run()
				case <-cc.checknow:
					cc.run()
				}
			}
		}(c)

		c.checknow <- true

		wg.Add(1)
	}

	wg.Wait()
}

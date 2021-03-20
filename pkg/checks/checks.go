package checks

import (
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
}

func (check check) notify(oldState serviceState) {
	if os.Getenv("SLAGIOS_webhook") != "" {
		serviceText := check.output
		serviceText = strings.Split(serviceText, "\n")[0]
		serviceText = strings.SplitN(serviceText, "|", 2)[0]

		buf := strings.NewReader(fmt.Sprintf("{\"text\":\"Check: %s\nCommand: %s (%s)\nChanged state %s->%s\nOutput: %s\"}",
			check.name, check.command, check.interval, oldState, check.state, serviceText))
		http.Post(os.Getenv("SLAGIOS_webhook"), "application/json", buf)
	}
}

func (check *check) run() {
	s, err := shlex.Split(check.command)
	if err != nil {
		log.Panicf("Could not parse command line: \"%s\" for %s %s",
			check.command, check.name, err)
	}

	prvState := check.state

	log.Printf("Running %s: %s", check.name, check.command)
	c := exec.Command(s[0], s[1:]...)
	out, _ := c.Output()

	check.output = string(out)
	check.state = serviceState(c.ProcessState.ExitCode())

	if prvState != check.state {
		log.Printf("State changed %s: %s->%s", check.name, prvState, check.state)
		check.notify(prvState)
	}
}

func load() []check {
	var checks []check

	interval := os.Getenv("SLAGIOS_interval")
	if interval == "" {
		interval = defaultCheckInterval
	}

	for _, e := range os.Environ() {
		p := strings.SplitN(e, "=", 2)
		name := p[0]
		cmd := p[1]

		if strings.HasPrefix(name, checkPrefix) {
			checkInerval := os.Getenv(strings.Replace(name, "check", "interval", 1))
			if checkInerval != "" {
				interval = checkInerval
			}

			dur, err := time.ParseDuration(interval)
			if err != nil {
				log.Panicf("Could not parse duration: \"%s\" for %s", interval, name)
			}

			c := check{name, cmd, "", unknown, dur}
			checks = append(checks, c)

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

		go func(cc check) {
			log.Printf("Schdeuled %s: %s (%s)", cc.name, cc.command, cc.interval)

			for {
				select {
				case <-ticker.C:
					cc.run()
				}
			}
		}(c)
		wg.Add(1)
	}
	wg.Wait()
}

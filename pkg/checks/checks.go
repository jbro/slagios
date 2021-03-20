package checks

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/shlex"
)

const checkPrefix = "SLAGIOS_check_"
const defaultCheckInterval = "60s"

type ServiceState int

func (s ServiceState) String() string {
	return [...]string{"OK", "Warning", "Critical", "Unknown"}[s]
}

const (
	OK       ServiceState = 0
	Warning               = 1
	Critical              = 2
	Unknown               = 3
)

type Check struct {
	Name     string
	Command  string
	Output   string
	State    ServiceState
	Interval time.Duration
}

func (check Check) Run() {
	s, err := shlex.Split(check.Command)
	if err != nil {
		log.Panicf("Could not parse command line: \"%s\" for %s %s",
			check.Command, check.Name, err)
	}

	prvState := check.State

	log.Printf("Running %s: %s", check.Name, check.Command)
	c := exec.Command(s[0], s[1:]...)
	out, _ := c.Output()

	check.Output = string(out)
	check.State = ServiceState(c.ProcessState.ExitCode())

	if prvState != check.State {
		log.Printf("State changed %s: %s->%s", check.Name, prvState, check.State)
	}
}

func Load() []Check {
	var checks []Check

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

			c := Check{name, cmd, "", Unknown, dur}
			checks = append(checks, c)

			log.Printf("Loaded %s: %s (%s)", name, cmd, dur)
		}
	}

	return checks
}

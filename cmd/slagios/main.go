package main

import (
	"github.com/jbro/slagios/pkg/checks"
)

func main() {
	for _, c := range checks.Load() {
		c.Run()
	}
}

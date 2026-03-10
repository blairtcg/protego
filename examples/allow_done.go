// Package main demonstrates the two-step Allow/Done pattern.
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/blairtcg/protego"
)

func main() {
	cb := protego.New(protego.Config{
		Name:    "twostep",
		Timeout: 5 * time.Second,
	})

	for i := 0; i < 5; i++ {
		// Two-step: check if allowed, then do work, then report result
		ticket, err := cb.Allow()
		if err != nil {
			fmt.Printf("Request %d: rejected (%v)\n", i, err)
			continue
		}

		// Do work
		err = doWork(i)

		// Report success or failure
		cb.Done(&ticket, err == nil)

		fmt.Printf("Request %d: success=%v\n", i, err == nil)
	}

	fmt.Printf("Final state: %s, Counts: %+v\n", cb.State(), cb.Counts())
}

// doWork simulates making a request.
func doWork(i int) error {
	if i%2 == 1 {
		return errors.New("work failed")
	}
	time.Sleep(10 * time.Millisecond)
	return nil
}

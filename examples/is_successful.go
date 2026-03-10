// Package main demonstrates custom error handling with IsSuccessful.
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/blairtcg/protego"
)

func main() {
	cb := protego.New(protego.Config{
		Name:    "custom-errors",
		Timeout: 10 * time.Second,
		IsSuccessful: func(err error) bool {
			// Treat context.Canceled as success (not a failure)
			if errors.Is(err, contextCanceled) {
				return true
			}
			return err == nil
		},
	})

	for i := 0; i < 5; i++ {
		err := cb.Execute(func() error {
			if i == 2 {
				return contextCanceled
			}
			if i == 3 {
				return errors.New("real error")
			}
			return nil
		})
		fmt.Printf("Request %d: err=%v, state=%s\n", i, err, cb.State())
	}
}

var contextCanceled = errors.New("context canceled")

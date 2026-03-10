// Package main demonstrates using protego with an http server.
package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/blairtcg/protego"
)

func main() {
	cb := protego.New(protego.Config{
		Name:    "http-service",
		Timeout: 30 * time.Second,
		ReadyToTrip: func(c protego.Counts) bool {
			return c.ConsecutiveFailures >= 5
		},
	})

	http.HandleFunc("/api", handleRequest(cb))
	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// handleRequest wraps an http handler with a circuit breaker.
func handleRequest(cb *protego.Breaker) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := cb.Execute(func() error {
			time.Sleep(100 * time.Millisecond)
			if r.URL.Query().Get("fail") == "true" {
				return errors.New("service unavailable")
			}
			fmt.Fprintf(w, "OK")
			return nil
		})
		if err != nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		}
	}
}

// Package main demonstrates using protego with a database client.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/blairtcg/protego"
)

func main() {
	db := NewDBClient()

	// Simulate making queries
	for i := 0; i < 5; i++ {
		err := db.Query(ctx, "SELECT * FROM users")
		fmt.Printf("Query %d: err=%v\n", i, err)
	}

	fmt.Printf("Final state: %s\n", db.cb.State())
}

var ctx = context.Background()

// DBClient wraps a database connection with a circuit breaker.
type DBClient struct {
	cb *protego.Breaker
}

func NewDBClient() *DBClient {
	return &DBClient{
		cb: protego.New(protego.Config{
			Name:    "db-client",
			Timeout: 10 * time.Second,
			ReadyToTrip: func(c protego.Counts) bool {
				return c.ConsecutiveFailures >= 3
			},
		}),
	}
}

// Query executes a database query with circuit breaker protection.
func (db *DBClient) Query(ctx context.Context, query string) error {
	return db.cb.Execute(func() error {
		// Simulate database call
		time.Sleep(10 * time.Millisecond)
		return nil
	})
}

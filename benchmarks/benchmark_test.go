// Package benchmarks compares different circuit breakers.
package benchmarks

import (
	"fmt"
	"protego"
	"testing"
	"time"
)

const (
	neverTripThreshold = 1000000
	openTimeout        = 1 * time.Millisecond
)

// Runner defines the interface for circuit breaker benchmarks.
type Runner interface {
	Name() string
	ClosedSuccess(b *testing.B)
	ClosedFailure(b *testing.B)
	HalfOpenSuccess(b *testing.B)
	HalfOpenFailure(b *testing.B)
	OpenState(b *testing.B)
	State(b *testing.B)
	Counts(b *testing.B)
}

// AllRunners returns all circuit breakers to benchmark.
func AllRunners() []Runner {
	return []Runner{
		&ProtegoRunner{},
		&SonyRunner{},
		&Cep21Runner{},
		&GoHystrixRunner{},
		&RubyistRunner{},
		&HandyRunner{},
	}
}

func BenchmarkAll(b *testing.B) {
	concurrentLevels := []int{1, 10, 50, 100}
	passFail := []bool{true, false}

	for _, runner := range AllRunners() {
		runner := runner
		b.Run(runner.Name(), func(b *testing.B) {
			for _, concurrent := range concurrentLevels {
				for _, pass := range passFail {
					concurrent := concurrent
					pass := pass
					name := fmt.Sprintf("concurrent=%d/%s", concurrent, map[bool]string{true: "pass", false: "fail"}[pass])
					b.Run(name, func(b *testing.B) {
						if pass {
							runner.ClosedSuccess(b)
						} else {
							runner.ClosedFailure(b)
						}
					})
				}
			}
			b.Run("halfopen-success", func(b *testing.B) {
				runner.HalfOpenSuccess(b)
			})
			b.Run("halfopen-failure", func(b *testing.B) {
				runner.HalfOpenFailure(b)
			})
			b.Run("open-state", func(b *testing.B) {
				runner.OpenState(b)
			})
			b.Run("state-only", func(b *testing.B) {
				runner.State(b)
			})
			b.Run("counts-only", func(b *testing.B) {
				runner.Counts(b)
			})
		})
	}
}

func BenchmarkClosedOnly(b *testing.B) {
	concurrentLevels := []int{1, 10, 50, 100}
	passFail := []bool{true, false}

	for _, runner := range AllRunners() {
		runner := runner
		b.Run(runner.Name(), func(b *testing.B) {
			for _, concurrent := range concurrentLevels {
				for _, pass := range passFail {
					concurrent := concurrent
					pass := pass
					name := fmt.Sprintf("concurrent=%d/%s", concurrent, map[bool]string{true: "pass", false: "fail"}[pass])
					b.Run(name, func(b *testing.B) {
						if pass {
							runner.ClosedSuccess(b)
						} else {
							runner.ClosedFailure(b)
						}
					})
				}
			}
		})
	}
}

// ProtegoRunner benchmarks protego.
type ProtegoRunner struct{}

func (r *ProtegoRunner) Name() string { return "protego" }

func (r *ProtegoRunner) ClosedSuccess(b *testing.B) {
	cb := protego.New(protego.Config{
		ReadyToTrip: func(c protego.Counts) bool { return c.ConsecutiveFailures >= neverTripThreshold },
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ticket, err := cb.Allow()
		if err == nil {
			cb.Done(&ticket, true)
		}
	}
}

func (r *ProtegoRunner) ClosedFailure(b *testing.B) {
	cb := protego.New(protego.Config{
		ReadyToTrip: func(c protego.Counts) bool { return c.ConsecutiveFailures >= neverTripThreshold },
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ticket, err := cb.Allow()
		if err == nil {
			cb.Done(&ticket, false)
		}
	}
}

func (r *ProtegoRunner) HalfOpenSuccess(b *testing.B) {
	cb := protego.New(protego.Config{
		MaxRequests: 10,
		Timeout:     openTimeout,
	})

	for i := 0; i < 10; i++ {
		ticket, _ := cb.Allow()
		cb.Done(&ticket, false)
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ticket, err := cb.Allow()
		if err == nil {
			cb.Done(&ticket, true)
		}
	}
}

func (r *ProtegoRunner) HalfOpenFailure(b *testing.B) {
	cb := protego.New(protego.Config{
		MaxRequests: 10,
		Timeout:     openTimeout,
	})

	for i := 0; i < 10; i++ {
		ticket, _ := cb.Allow()
		cb.Done(&ticket, false)
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ticket, err := cb.Allow()
		if err == nil {
			cb.Done(&ticket, false)
		}
	}
}

func (r *ProtegoRunner) OpenState(b *testing.B) {
	cb := protego.New(protego.Config{
		ReadyToTrip: func(c protego.Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	for i := 0; i < 10; i++ {
		ticket, _ := cb.Allow()
		cb.Done(&ticket, false)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cb.Allow()
	}
}

func (r *ProtegoRunner) State(b *testing.B) {
	cb := protego.New(protego.Config{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cb.State()
	}
}

func (r *ProtegoRunner) Counts(b *testing.B) {
	cb := protego.New(protego.Config{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cb.Counts()
	}
}

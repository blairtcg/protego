package benchmarks

import (
	"testing"
	"time"

	"github.com/streadway/handy/breaker"
)

// HandyRunner benchmarks streadway/handy/breaker.
type HandyRunner struct{}

func (r *HandyRunner) Name() string { return "handy-breaker" }

func (r *HandyRunner) ClosedSuccess(b *testing.B) {
	cb := breaker.NewBreaker(0.9)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if cb.Allow() {
			cb.Success(0)
		}
	}
}

func (r *HandyRunner) ClosedFailure(b *testing.B) {
	cb := breaker.NewBreaker(0.9)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if cb.Allow() {
			cb.Failure(0)
		}
	}
}

func (r *HandyRunner) HalfOpenSuccess(b *testing.B) {
	cb := breaker.NewBreaker(0.9)

	for i := 0; i < 10; i++ {
		if cb.Allow() {
			cb.Failure(0)
		}
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if cb.Allow() {
			cb.Success(0)
		}
	}
}

func (r *HandyRunner) HalfOpenFailure(b *testing.B) {
	cb := breaker.NewBreaker(0.9)

	for i := 0; i < 10; i++ {
		if cb.Allow() {
			cb.Failure(0)
		}
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if cb.Allow() {
			cb.Failure(0)
		}
	}
}

func (r *HandyRunner) OpenState(b *testing.B) {
	cb := breaker.NewBreaker(0.1)

	for i := 0; i < 10; i++ {
		if cb.Allow() {
			cb.Failure(0)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = cb.Allow()
	}
}

func (r *HandyRunner) State(b *testing.B) {
	cb := breaker.NewBreaker(0.9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Allow()
	}
}

func (r *HandyRunner) Counts(b *testing.B) {
	cb := breaker.NewBreaker(0.9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Allow()
	}
}

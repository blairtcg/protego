package benchmarks

import (
	"fmt"
	"testing"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
)

// SonyRunner benchmarks sony/gobreaker.
type SonyRunner struct{}

func (r *SonyRunner) Name() string { return "sony" }

func (r *SonyRunner) ClosedSuccess(b *testing.B) {
	cb := gobreaker.NewCircuitBreaker[gobreaker.Counts](gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= neverTripThreshold
		},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, nil
		})
	}
}

func (r *SonyRunner) ClosedFailure(b *testing.B) {
	cb := gobreaker.NewCircuitBreaker[gobreaker.Counts](gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= neverTripThreshold
		},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, fmt.Errorf("error")
		})
	}
}

func (r *SonyRunner) HalfOpenSuccess(b *testing.B) {
	cb := gobreaker.NewCircuitBreaker[gobreaker.Counts](gobreaker.Settings{
		MaxRequests: 10,
		Timeout:     openTimeout,
	})

	for i := 0; i < 10; i++ {
		cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, fmt.Errorf("error")
		})
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, nil
		})
	}
}

func (r *SonyRunner) HalfOpenFailure(b *testing.B) {
	cb := gobreaker.NewCircuitBreaker[gobreaker.Counts](gobreaker.Settings{
		MaxRequests: 10,
		Timeout:     openTimeout,
	})

	for i := 0; i < 10; i++ {
		cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, fmt.Errorf("error")
		})
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, fmt.Errorf("error")
		})
	}
}

func (r *SonyRunner) OpenState(b *testing.B) {
	cb := gobreaker.NewCircuitBreaker[gobreaker.Counts](gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	})

	for i := 0; i < 10; i++ {
		cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, fmt.Errorf("error")
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cb.Execute(func() (gobreaker.Counts, error) {
			return gobreaker.Counts{}, fmt.Errorf("error")
		})
	}
}

func (r *SonyRunner) State(b *testing.B) {
	cb := gobreaker.NewCircuitBreaker[gobreaker.Counts](gobreaker.Settings{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cb.State()
	}
}

func (r *SonyRunner) Counts(b *testing.B) {
	cb := gobreaker.NewCircuitBreaker[gobreaker.Counts](gobreaker.Settings{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = cb.Counts()
	}
}

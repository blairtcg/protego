package benchmarks

import (
	"fmt"
	"testing"
	"time"

	circuit "github.com/rubyist/circuitbreaker"
)

// RubyistRunner benchmarks rubyist/circuitbreaker.
type RubyistRunner struct{}

func (r *RubyistRunner) Name() string { return "rubyist-circuitbreaker" }

func (r *RubyistRunner) ClosedSuccess(b *testing.B) {
	circ := circuit.NewThresholdBreaker(neverTripThreshold)
	circ.Reset()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = circ.Call(func() error { return nil }, 0)
	}
}

func (r *RubyistRunner) ClosedFailure(b *testing.B) {
	circ := circuit.NewThresholdBreaker(neverTripThreshold)
	circ.Reset()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = circ.Call(func() error { return fmt.Errorf("error") }, 0)
	}
}

func (r *RubyistRunner) HalfOpenSuccess(b *testing.B) {
	circ := circuit.NewThresholdBreaker(1)
	circ.Reset()

	for i := 0; i < 10; i++ {
		circ.Call(func() error { return fmt.Errorf("error") }, 0)
	}
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = circ.Call(func() error { return nil }, 0)
	}
}

func (r *RubyistRunner) HalfOpenFailure(b *testing.B) {
	circ := circuit.NewThresholdBreaker(1)
	circ.Reset()

	for i := 0; i < 10; i++ {
		circ.Call(func() error { return fmt.Errorf("error") }, 0)
	}
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = circ.Call(func() error { return fmt.Errorf("error") }, 0)
	}
}

func (r *RubyistRunner) OpenState(b *testing.B) {
	circ := circuit.NewThresholdBreaker(1)
	circ.Reset()

	for i := 0; i < 10; i++ {
		circ.Call(func() error { return fmt.Errorf("error") }, 0)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = circ.Call(func() error { return fmt.Errorf("error") }, 0)
	}
}

func (r *RubyistRunner) State(b *testing.B) {
	circ := circuit.NewThresholdBreaker(neverTripThreshold)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = circ.Ready()
	}
}

func (r *RubyistRunner) Counts(b *testing.B) {
	circ := circuit.NewThresholdBreaker(neverTripThreshold)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = circ.Ready()
	}
}

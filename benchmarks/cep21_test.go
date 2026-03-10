package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	circuit "github.com/cep21/circuit/v4"
)

// Cep21Runner benchmarks cep21/circuit.
type Cep21Runner struct{}

func (r *Cep21Runner) Name() string { return "cep21-circuit" }

func (r *Cep21Runner) ClosedSuccess(b *testing.B) {
	h := circuit.Manager{}
	c := h.MustCreateCircuit("test", circuit.Config{
		Execution: circuit.ExecutionConfig{
			MaxConcurrentRequests: 100000,
		},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = c.Execute(context.Background(), func(ctx context.Context) error { return nil }, nil)
	}
}

func (r *Cep21Runner) ClosedFailure(b *testing.B) {
	h := circuit.Manager{}
	c := h.MustCreateCircuit("test", circuit.Config{
		Execution: circuit.ExecutionConfig{MaxConcurrentRequests: 100000},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = c.Execute(context.Background(), func(ctx context.Context) error { return fmt.Errorf("error") }, nil)
	}
}

func (r *Cep21Runner) HalfOpenSuccess(b *testing.B) {
	h := circuit.Manager{}
	c := h.MustCreateCircuit("test-halfopen", circuit.Config{
		Execution: circuit.ExecutionConfig{
			MaxConcurrentRequests: 100000,
		},
	})

	for i := 0; i < 10; i++ {
		c.Execute(context.Background(), func(ctx context.Context) error { return fmt.Errorf("error") }, nil)
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = c.Execute(context.Background(), func(ctx context.Context) error { return nil }, nil)
	}
}

func (r *Cep21Runner) HalfOpenFailure(b *testing.B) {
	h := circuit.Manager{}
	c := h.MustCreateCircuit("test-halfopen", circuit.Config{
		Execution: circuit.ExecutionConfig{
			MaxConcurrentRequests: 100000,
		},
	})

	for i := 0; i < 10; i++ {
		c.Execute(context.Background(), func(ctx context.Context) error { return fmt.Errorf("error") }, nil)
	}
	time.Sleep(2 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = c.Execute(context.Background(), func(ctx context.Context) error { return fmt.Errorf("error") }, nil)
	}
}

func (r *Cep21Runner) OpenState(b *testing.B) {
	h := circuit.Manager{}
	c := h.MustCreateCircuit("test", circuit.Config{
		Execution: circuit.ExecutionConfig{MaxConcurrentRequests: 100000},
	})

	for i := 0; i < 10; i++ {
		c.Execute(context.Background(), func(ctx context.Context) error { return fmt.Errorf("error") }, nil)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = c.Execute(context.Background(), func(ctx context.Context) error { return fmt.Errorf("error") }, nil)
	}
}

func (r *Cep21Runner) State(b *testing.B) {
	h := circuit.Manager{}
	c := h.MustCreateCircuit("test", circuit.Config{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.IsOpen()
	}
}

func (r *Cep21Runner) Counts(b *testing.B) {
	h := circuit.Manager{}
	c := h.MustCreateCircuit("test", circuit.Config{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.IsOpen()
	}
}

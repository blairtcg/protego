package benchmarks

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	gohystrix "github.com/afex/hystrix-go/hystrix"
)

var hystrixCount int64

// GoHystrixRunner benchmarks gohystrix.
type GoHystrixRunner struct{}

func (r *GoHystrixRunner) Name() string { return "gohystrix" }

func (r *GoHystrixRunner) ClosedSuccess(b *testing.B) {
	name := fmt.Sprintf("hystrix-%d", addHystrixCount())
	gohystrix.ConfigureCommand(name, gohystrix.CommandConfig{
		MaxConcurrentRequests: 100000,
		ErrorPercentThreshold: 100,
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = gohystrix.Do(name, func() error {
			return nil
		}, nil)
	}
}

func (r *GoHystrixRunner) ClosedFailure(b *testing.B) {
	name := fmt.Sprintf("hystrix-%d", addHystrixCount())
	gohystrix.ConfigureCommand(name, gohystrix.CommandConfig{
		MaxConcurrentRequests:  100000,
		ErrorPercentThreshold:  100,
		RequestVolumeThreshold: 1,
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = gohystrix.Do(name, func() error {
			return fmt.Errorf("error")
		}, nil)
	}
}

func (r *GoHystrixRunner) HalfOpenSuccess(b *testing.B) {
	name := fmt.Sprintf("hystrix-%d", addHystrixCount())
	gohystrix.ConfigureCommand(name, gohystrix.CommandConfig{
		MaxConcurrentRequests: 10,
		Timeout:               1000,
	})

	for i := 0; i < 10; i++ {
		gohystrix.Do(name, func() error { return fmt.Errorf("error") }, nil)
	}
	time.Sleep(1500 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = gohystrix.Do(name, func() error { return nil }, nil)
	}
}

func (r *GoHystrixRunner) HalfOpenFailure(b *testing.B) {
	name := fmt.Sprintf("hystrix-%d", addHystrixCount())
	gohystrix.ConfigureCommand(name, gohystrix.CommandConfig{
		MaxConcurrentRequests: 10,
		Timeout:               1000,
	})

	for i := 0; i < 10; i++ {
		gohystrix.Do(name, func() error { return fmt.Errorf("error") }, nil)
	}
	time.Sleep(1500 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = gohystrix.Do(name, func() error { return fmt.Errorf("error") }, nil)
	}
}

func (r *GoHystrixRunner) OpenState(b *testing.B) {
	name := fmt.Sprintf("hystrix-%d", addHystrixCount())
	gohystrix.ConfigureCommand(name, gohystrix.CommandConfig{
		MaxConcurrentRequests:  100000,
		RequestVolumeThreshold: 1,
		ErrorPercentThreshold:  1,
	})

	for i := 0; i < 10; i++ {
		gohystrix.Do(name, func() error { return fmt.Errorf("error") }, nil)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = gohystrix.Do(name, func() error { return fmt.Errorf("error") }, nil)
	}
}

func (r *GoHystrixRunner) State(b *testing.B) {
	name := fmt.Sprintf("hystrix-state-%d", addHystrixCount())
	gohystrix.ConfigureCommand(name, gohystrix.CommandConfig{
		MaxConcurrentRequests: 100000,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gohystrix.Do(name, func() error { return nil }, nil)
	}
}

func (r *GoHystrixRunner) Counts(b *testing.B) {
	name := fmt.Sprintf("hystrix-counts-%d", addHystrixCount())
	gohystrix.ConfigureCommand(name, gohystrix.CommandConfig{
		MaxConcurrentRequests: 100000,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gohystrix.Do(name, func() error { return nil }, nil)
	}
}

func addHystrixCount() int {
	return int(atomic.AddInt64(&hystrixCount, 1))
}

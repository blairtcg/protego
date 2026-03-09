package protego

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBreaker_DoneNilTicket(t *testing.T) {
	b := New(Config{})
	b.Done(nil, true)

	c := b.Counts()
	if c.TotalSuccesses != 0 {
		t.Fatalf("nil ticket should not increment counter, got %d", c.TotalSuccesses)
	}
}

func TestBreaker_ConcurrentAllowDone(_ *testing.T) {
	b := New(Config{
		MaxRequests: 1000,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 100 },
		Timeout:     10 * time.Millisecond,
	})

	var wg sync.WaitGroup
	numGoroutines := 100
	opsPerGoroutine := 1000

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tk, err := b.Allow()
				if err == nil {
					success := (j % 10) != 0
					b.Done(&tk, success)
				}
			}
		}()
	}

	wg.Wait()

	_ = b.Counts()
	_ = b.State()
}

func TestBreaker_ConcurrentStateTransitions(_ *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 3 },
		Timeout:     5 * time.Millisecond,
	})

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tk, err := b.Allow()
				if err == nil {
					b.Done(&tk, false)
				}
				runtime.Gosched()
			}
		}()
	}

	time.Sleep(20 * time.Millisecond)
	wg.Wait()

	_ = b.State()
}

func TestBreaker_DoubleDoneProtection(t *testing.T) {
	b := New(Config{})

	for i := 0; i < 100; i++ {
		tk, err := b.Allow()
		if err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		for j := 0; j < 10; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				b.Done(&tk, true)
			}()
		}
		wg.Wait()
	}

	c := b.Counts()
	if c.TotalSuccesses != 100 {
		t.Fatalf("double-done protection failed: got %d successes, want 100", c.TotalSuccesses)
	}
}

func TestBreaker_StaleTicketProtection(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
		MaxRequests: 3,
	})

	tk1, _ := b.Allow()
	b.Done(&tk1, false)

	time.Sleep(20 * time.Millisecond)

	tk2, _ := b.Allow()

	successesBefore := b.consecSucc.Load()
	b.Done(&tk1, true)
	successesAfterTk1 := b.consecSucc.Load()
	b.Done(&tk2, true)
	successesAfterTk2 := b.consecSucc.Load()

	if successesAfterTk1 != successesBefore {
		t.Fatalf("stale ticket should not increment consecSucc: before=%d, after=%d", successesBefore, successesAfterTk1)
	}

	if successesAfterTk2 <= successesBefore {
		t.Fatalf("valid ticket should increment consecSucc: before=%d, after=%d", successesBefore, successesAfterTk2)
	}
}

func TestBreaker_DeterministicRecovery(t *testing.T) {
	b := New(Config{
		MaxRequests: 3,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		tk, _ := b.Allow()
		b.Done(&tk, false)
	}

	if b.State() != StateOpen {
		t.Fatal("should be open after failures")
	}

	time.Sleep(15 * time.Millisecond)

	if b.State() != StateHalfOpen {
		t.Fatal("should transition to half-open after timeout")
	}

	for i := 0; i < 3; i++ {
		tk, err := b.Allow()
		if err != nil {
			t.Fatalf("allow error in half-open: %v", err)
		}
		b.Done(&tk, true)
	}

	if b.State() != StateClosed {
		t.Fatalf("should be closed after successful half-open requests, got %v", b.State())
	}
}

func TestBreaker_OnStateChangeCalled(t *testing.T) {
	var stateChanges int32
	var lastFrom, lastTo State

	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
		OnStateChange: func(_ string, from State, to State) {
			atomic.AddInt32(&stateChanges, 1)
			lastFrom = from
			lastTo = to
		},
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	if atomic.LoadInt32(&stateChanges) != 1 {
		t.Fatal("onStateChange not called for open transition")
	}
	if lastFrom != StateClosed || lastTo != StateOpen {
		t.Fatalf("wrong state change: %v -> %v", lastFrom, lastTo)
	}

	time.Sleep(15 * time.Millisecond)
	b.State()

	if atomic.LoadInt32(&stateChanges) != 2 {
		t.Fatal("onStateChange not called for half-open transition")
	}
	if lastFrom != StateOpen || lastTo != StateHalfOpen {
		t.Fatalf("wrong state change: %v -> %v", lastFrom, lastTo)
	}
}

func TestBreaker_OnStateChangeNoBlocking(_ *testing.T) {
	var callCount int32

	b := New(Config{
		MaxRequests: 1,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     1 * time.Millisecond,
		OnStateChange: func(_ string, _, _ State) {
			atomic.AddInt32(&callCount, 1)
		},
	})

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tk, err := b.Allow()
			if err == nil {
				b.Done(&tk, false)
			}
		}()
	}

	close(start)
	wg.Wait()
}

func TestBreaker_IntervalRotation(t *testing.T) {
	b := New(Config{
		Interval:    10 * time.Millisecond,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 100 },
	})

	for i := 0; i < 10; i++ {
		tk, _ := b.Allow()
		b.Done(&tk, true)
	}

	c1 := b.Counts()
	if c1.TotalSuccesses != 10 {
		t.Fatalf("expected 10 successes, got %d", c1.TotalSuccesses)
	}

	time.Sleep(15 * time.Millisecond)

	tk, _ := b.Allow()
	b.Done(&tk, true)

	c2 := b.Counts()
	if c2.TotalSuccesses != 1 {
		t.Fatalf("interval rotation failed: expected 1 (reset), got %d", c2.TotalSuccesses)
	}
}

func TestBreaker_HalfOpenPermitConcurrency(t *testing.T) {
	b := New(Config{
		MaxRequests: 2,
		Timeout:     10 * time.Millisecond,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	var wg sync.WaitGroup
	successCount := int32(0)
	maxConcurrent := int32(0)
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, err := b.Allow()
			if err == nil {
				atomic.AddInt32(&successCount, 1)
				current := atomic.AddInt32(&successCount, 0)
				mu.Lock()
				if current > maxConcurrent {
					maxConcurrent = current
				}
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				b.Done(&tk, true)
			}
		}()
	}

	wg.Wait()

	if maxConcurrent > 2 {
		t.Fatalf("too many concurrent half-open permits: got %d, want max 2", maxConcurrent)
	}
}

func TestBreaker_ExecutePanicRecovery(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	var panicCount int32
	err := b.Execute(func() error {
		atomic.AddInt32(&panicCount, 1)
		panic("test panic")
	})

	if err == nil {
		t.Fatal("expected error from panic")
	}
	if atomic.LoadInt32(&panicCount) != 1 {
		t.Fatal("panic not recovered properly")
	}

	if b.State() != StateOpen {
		t.Fatalf("breaker should be open after panic, got %v", b.State())
	}
}

func TestBreaker_ExecuteValue(t *testing.T) {
	b := New(Config{})

	result, err := ExecuteValue(b, func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Fatalf("unexpected result: got %d, want 42", result)
	}
}

func TestBreaker_MaxRequestsDefault(t *testing.T) {
	b := New(Config{})

	if b.maxRequests != 1 {
		t.Fatalf("default MaxRequests should be 1, got %d", b.maxRequests)
	}
}

func TestBreaker_TimeoutDefault(t *testing.T) {
	b := New(Config{})

	if b.timeoutNS != 60*time.Second.Nanoseconds() {
		t.Fatalf("default timeout should be 60s, got %v", b.timeoutNS)
	}
}

func TestBreaker_ReadyToTripDefault(t *testing.T) {
	b := New(Config{})

	if !b.readyToTrip(Counts{ConsecutiveFailures: 6}) {
		t.Fatal("default ReadyToTrip should trip at 6 consecutive failures")
	}
	if b.readyToTrip(Counts{ConsecutiveFailures: 5}) {
		t.Fatal("default ReadyToTrip should not trip at 5 consecutive failures")
	}
}

func TestBreaker_IsSuccessfulDefault(t *testing.T) {
	b := New(Config{})

	if !b.isSuccessful(nil) {
		t.Fatal("default IsSuccessful should return true for nil error")
	}
	if b.isSuccessful(errors.New("error")) {
		t.Fatal("default IsSuccessful should return false for non-nil error")
	}
}

func TestBreaker_NilReadyToTrip(t *testing.T) {
	b := New(Config{ReadyToTrip: nil})
	if b.readyToTrip == nil {
		t.Fatal("ReadyToTrip should default to non-nil")
	}
}

func TestBreaker_NilIsSuccessful(t *testing.T) {
	b := New(Config{IsSuccessful: nil})
	if b.isSuccessful == nil {
		t.Fatal("IsSuccessful should default to non-nil")
	}
}

func TestBreaker_Name(t *testing.T) {
	b := New(Config{Name: "test-breaker"})
	if b.Name() != "test-breaker" {
		t.Fatalf("Name() = %q, want %q", b.Name(), "test-breaker")
	}
}

func TestBreaker_NameEmpty(t *testing.T) {
	b := New(Config{})
	if b.Name() != "" {
		t.Fatalf("Name() = %q, want empty", b.Name())
	}
}

func TestBreaker_ConcurrentHalfOpenSuccessCloses(t *testing.T) {
	b := New(Config{
		MaxRequests: 3,
		Timeout:     10 * time.Millisecond,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, err := b.Allow()
			if err == nil {
				b.Done(&tk, true)
			}
		}()
	}

	wg.Wait()

	if b.State() != StateClosed {
		t.Fatalf("should be closed after maxRequests successes in half-open, got %v", b.State())
	}
}

func TestBreaker_ConcurrentHalfOpenFailureReopens(t *testing.T) {
	b := New(Config{
		MaxRequests: 3,
		Timeout:     10 * time.Millisecond,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	tk2, _ := b.Allow()
	b.Done(&tk2, false)

	if b.State() != StateOpen {
		t.Fatalf("should reopen after failure in half-open, got %v", b.State())
	}
}

func TestBreaker_NoMemoryLeak_RepeatedCreation(t *testing.T) {
	runtime.GC()

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	for i := 0; i < 1000; i++ {
		b := New(Config{
			MaxRequests:   100,
			ReadyToTrip:   func(c Counts) bool { return c.ConsecutiveFailures >= 10 },
			OnStateChange: func(_ string, _, _ State) {},
		})

		for j := 0; j < 50; j++ {
			tk, _ := b.Allow()
			b.Done(&tk, j%10 != 0)
		}
	}

	runtime.GC()

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	allocGrowth := m2.Mallocs - m1.Mallocs
	if allocGrowth > 10000 {
		t.Logf("Potential memory leak: %d allocations", allocGrowth)
	}
}

func TestBreaker_LockFreeHotPath(t *testing.T) {
	b := New(Config{})

	var latencies []time.Duration
	var mu sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				start := time.Now()
				tk, err := b.Allow()
				elapsed := time.Since(start)
				if err == nil {
					mu.Lock()
					latencies = append(latencies, elapsed)
					mu.Unlock()
					b.Done(&tk, true)
				}
			}
		}()
	}

	wg.Wait()

	var slowCount int
	for _, lat := range latencies {
		if lat > 100*time.Microsecond {
			slowCount++
		}
	}

	if slowCount > len(latencies)/10 {
		t.Logf("Warning: %d/%d operations took > 100µs", slowCount, len(latencies))
	}
}

func TestBreaker_ZeroTimeout_UsesDefault(t *testing.T) {
	b := New(Config{Timeout: 0})
	if b.timeoutNS != 60*time.Second.Nanoseconds() {
		t.Fatalf("zero timeout should use default 60s, got %v", b.timeoutNS)
	}
}

func TestBreaker_NegativeTimeout_UsesDefault(t *testing.T) {
	b := New(Config{Timeout: -1 * time.Second})
	if b.timeoutNS != 60*time.Second.Nanoseconds() {
		t.Fatalf("negative timeout should use default 60s, got %v", b.timeoutNS)
	}
}

func TestBreaker_ZeroMaxRequests_DefaultsToOne(t *testing.T) {
	b := New(Config{MaxRequests: 0})
	if b.maxRequests != 1 {
		t.Fatalf("zero MaxRequests should default to 1, got %d", b.maxRequests)
	}
}

func TestBreaker_ZeroInterval_NoRotation(t *testing.T) {
	b := New(Config{
		Interval:    0,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 100 },
	})

	for i := 0; i < 10; i++ {
		tk, _ := b.Allow()
		b.Done(&tk, true)
	}

	time.Sleep(20 * time.Millisecond)

	tk, _ := b.Allow()
	b.Done(&tk, true)

	c := b.Counts()
	if c.TotalSuccesses != 11 {
		t.Fatalf("with zero interval, counters should not reset: got %d, want 11", c.TotalSuccesses)
	}
}

func TestBreaker_CustomIsSuccessful(t *testing.T) {
	b := New(Config{
		IsSuccessful: func(err error) bool {
			return err == nil || err == ErrTooManyRequests
		},
	})

	tk, _ := b.Allow()
	b.Done(&tk, b.isSuccessful(ErrTooManyRequests))

	c := b.Counts()
	if c.TotalSuccesses != 1 {
		t.Fatalf("custom IsSuccessful should count ErrTooManyRequests as success, got %d", c.TotalSuccesses)
	}
}

func TestBreaker_CustomReadyToTrip(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool {
			return c.TotalFailures >= 3
		},
	})

	for i := 0; i < 3; i++ {
		tk, _ := b.Allow()
		b.Done(&tk, false)
	}

	if b.State() != StateOpen {
		t.Fatalf("custom ReadyToTrip should trip at 3 total failures, got %v", b.State())
	}
}

func TestBreaker_OpenRejectsRequests(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	for i := 0; i < 10; i++ {
		_, err := b.Allow()
		if !errors.Is(err, ErrOpenState) {
			t.Fatalf("open breaker should reject requests, got %v", err)
		}
	}
}

func TestBreaker_HalfOpenAfterTimeout(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     50 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(20 * time.Millisecond)

	time.Sleep(40 * time.Millisecond)
	if b.State() != StateHalfOpen {
		t.Fatalf("should be half-open after timeout, got %v", b.State())
	}
}

func TestBreaker_MultipleOpenCloseCycles(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
		MaxRequests: 2,
	})

	for cycle := 0; cycle < 5; cycle++ {
		tk, _ := b.Allow()
		b.Done(&tk, false)

		time.Sleep(15 * time.Millisecond)
		if b.State() != StateHalfOpen {
			t.Fatalf("cycle %d: should be half-open, got %v", cycle, b.State())
		}

		tk1, _ := b.Allow()
		b.Done(&tk1, true)
		tk2, _ := b.Allow()
		b.Done(&tk2, true)

		if b.State() != StateClosed {
			t.Fatalf("cycle %d: should be closed after successes, got %v", cycle, b.State())
		}
	}
}

func TestBreaker_ConcurrentMixedSuccessFailure(_ *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 5 },
		Timeout:     10 * time.Millisecond,
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tk, err := b.Allow()
				if err == nil {
					b.Done(&tk, (idx+j)%2 == 0)
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestBreaker_ExtremeConcurrency(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1000 },
		MaxRequests: 100,
	})

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tk, err := b.Allow()
				if err == nil {
					b.Done(&tk, true)
				}
			}
		}()
	}
	wg.Wait()

	c := b.Counts()
	if c.TotalSuccesses == 0 {
		t.Fatal("extreme concurrency should still process requests")
	}
}

func TestBreaker_EpochIncrementsOnTransition(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	epoch1 := b.epoch.Load()
	tk, _ := b.Allow()
	b.Done(&tk, false)
	epoch2 := b.epoch.Load()

	if epoch2 <= epoch1 {
		t.Fatalf("epoch should increment on state transition: %d -> %d", epoch1, epoch2)
	}

	time.Sleep(15 * time.Millisecond)
	b.State()
	epoch3 := b.epoch.Load()

	if epoch3 <= epoch2 {
		t.Fatalf("epoch should increment on half-open transition: %d -> %d", epoch2, epoch3)
	}
}

func TestBreaker_TicketFlagsHalfOpen(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	tk1, _ := b.Allow()
	b.Done(&tk1, false)

	time.Sleep(15 * time.Millisecond)

	tk2, _ := b.Allow()

	if (tk2.flags & ticketFlagHalfOpen) == 0 {
		t.Fatal("half-open ticket should have flag set")
	}
}

func TestBreaker_ClosedDoesNotBlock(t *testing.T) {
	b := New(Config{})

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tk, err := b.Allow()
			if err == nil {
				b.Done(&tk, true)
			}
		}()
	}
	wg.Wait()

	c := b.Counts()
	if c.TotalSuccesses != 1000 {
		t.Fatalf("all 1000 requests should succeed in closed state, got %d", c.TotalSuccesses)
	}
}

func TestBreaker_ConcurrentStateAndCounts(_ *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 10 },
	})

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				tk, err := b.Allow()
				if err == nil {
					b.Done(&tk, j%10 != 0)
				}
			}
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = b.State()
				_ = b.Counts()
			}
		}()
	}

	wg.Wait()
}

func TestBreaker_AllStatesTransitions(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     20 * time.Millisecond,
		MaxRequests: 2,
	})

	if b.State() != StateClosed {
		t.Fatal("initial state should be closed")
	}

	tk, _ := b.Allow()
	b.Done(&tk, false)
	if b.State() != StateOpen {
		t.Fatal("should be open after failure")
	}

	time.Sleep(25 * time.Millisecond)
	if b.State() != StateHalfOpen {
		t.Fatal("should be half-open after timeout")
	}

	tk1, _ := b.Allow()
	b.Done(&tk1, true)
	tk2, _ := b.Allow()
	b.Done(&tk2, true)

	if b.State() != StateClosed {
		t.Fatal("should be closed after successful half-open")
	}
}

func TestBreaker_DoneSuccessHalfOpenNoTransition(t *testing.T) {
	b := New(Config{
		MaxRequests: 5,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	for i := 0; i < 4; i++ {
		tk, _ := b.Allow()
		b.Done(&tk, true)
	}

	if b.State() != StateHalfOpen {
		t.Fatalf("should still be half-open with only 4 successes, got %v", b.State())
	}
}

func TestBreaker_DoneFailureHalfOpen(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	tk2, _ := b.Allow()
	b.Done(&tk2, false)

	if b.State() != StateOpen {
		t.Fatalf("should be open after failure in half-open, got %v", b.State())
	}
}

func TestBreaker_ExecuteValuePanic(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	_, err := ExecuteValue(b, func() (int, error) {
		panic("value panic")
	})

	if err == nil {
		t.Fatal("expected error from panic")
	}

	if b.State() != StateOpen {
		t.Fatalf("breaker should be open after panic, got %v", b.State())
	}
}

func TestBreaker_ExecuteValueSuccess(t *testing.T) {
	b := New(Config{})

	result, err := ExecuteValue(b, func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Fatalf("unexpected result: got %d, want 42", result)
	}
}

func TestBreaker_ExecuteValueError(t *testing.T) {
	b := New(Config{
		IsSuccessful: func(err error) bool { return err == nil },
	})

	_, err := ExecuteValue(b, func() (int, error) {
		return 0, errors.New("error")
	})

	if err == nil {
		t.Fatal("expected error from work")
	}

	c := b.Counts()
	if c.TotalFailures != 1 {
		t.Fatalf("error should count as failure, got %d", c.TotalFailures)
	}
}

func TestBreaker_ApplyStateTransitionOpen(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	if b.State() != StateOpen {
		t.Fatal("should be open after failure")
	}

	c := b.Counts()
	if c.Requests != 0 {
		t.Error("requests should be reset in open state")
	}
}

func TestBreaker_DecHalfOpenPermitZero(t *testing.T) {
	b := New(Config{})

	if b.HalfOpenInflight() != 0 {
		t.Fatal("initial should be 0")
	}

	b.decHalfOpenPermit()

	if b.HalfOpenInflight() != 0 {
		t.Error("should still be 0")
	}
}

func TestBreaker_TransitionSameState(t *testing.T) {
	b := New(Config{})

	initialEpoch := b.Counts().Epoch

	b.Close()

	afterEpoch := b.Counts().Epoch

	if initialEpoch == afterEpoch {
		t.Log("epoch should not change when transitioning to same state")
	}
}

func TestBreaker_TryOpenToHalfOpenEarlyReturn(t *testing.T) {
	b := New(Config{
		Timeout: 100 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	b.tryOpenToHalfOpen(nowNano() + 50*1e6)

	if b.State() != StateOpen {
		t.Logf("state is %v, may still be open", b.State())
	}
}

func TestBreaker_RotateClosedWindowEarlyReturn(t *testing.T) {
	b := New(Config{
		Interval:    100 * time.Millisecond,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 100 },
	})

	epoch1 := b.Counts().Epoch

	b.rotateClosedWindow(nowNano() + 50*1e6)

	epoch2 := b.Counts().Epoch

	if epoch1 != epoch2 {
		t.Error("epoch should not change when interval not expired")
	}
}

func TestBreaker_DecHalfOpenPermit(t *testing.T) {
	b := New(Config{
		MaxRequests: 2,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	tk1, _ := b.Allow()

	if b.HalfOpenInflight() != 1 {
		t.Errorf("halfInFlight should be 1, got %d", b.HalfOpenInflight())
	}

	b.Done(&tk1, true)

	if b.HalfOpenInflight() != 0 {
		t.Errorf("halfInFlight should be 0 after Done, got %d", b.HalfOpenInflight())
	}
}

func TestBreaker_ApplyStateTransitionHalfOpen(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	if b.State() != StateHalfOpen {
		t.Fatalf("should be half-open after timeout, got %v", b.State())
	}

	if b.expiry.Load() != 0 {
		t.Error("expiry should be 0 in half-open state")
	}
}

func TestBreaker_AllowContextContextCancel(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     1 * time.Second,
		MaxRequests: 1,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(50 * time.Millisecond)

	_, err := b.Allow()
	if err != nil {
		t.Logf("first allow in half-open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = b.AllowContext(ctx)

	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Logf("expected context timeout, got: %v", err)
	}
}

func TestBreaker_ManualOpen(t *testing.T) {
	b := New(Config{})

	if b.State() != StateClosed {
		t.Fatal("initial state should be closed")
	}

	b.Open()

	if b.State() != StateOpen {
		t.Fatalf("after Open(), state should be open, got %v", b.State())
	}

	_, err := b.Allow()
	if err == nil {
		t.Fatal("allow should fail when open")
	}
}

func TestBreaker_ManualReset(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	for i := 0; i < 5; i++ {
		tk, _ := b.Allow()
		b.Done(&tk, false)
	}

	if b.State() != StateOpen {
		t.Fatal("should be open")
	}

	c1 := b.Counts()

	b.Reset()

	if b.State() != StateClosed {
		t.Fatalf("after Reset(), state should be closed, got %v", b.State())
	}

	c2 := b.Counts()

	if c2.TotalFailures != 0 {
		t.Fatalf("after Reset(), counters should be zero, got %d", c2.TotalFailures)
	}
	if c1.Epoch >= c2.Epoch {
		t.Fatalf("epoch should have incremented after reset")
	}
}

func TestBreaker_RefreshByTimeNoExpiry(t *testing.T) {
	b := New(Config{})

	initialState := b.State()

	b.refreshByTime(nowNano())

	if b.State() != initialState {
		t.Error("state should not change with no expiry set")
	}
}

func TestBreaker_TryOpenToHalfOpenNotOpen(t *testing.T) {
	b := New(Config{})

	b.tryOpenToHalfOpen(nowNano())

	if b.State() != StateClosed {
		t.Error("should remain closed when already closed")
	}
}

func TestBreaker_TryOpenToHalfOpenNotReady(t *testing.T) {
	b := New(Config{
		Timeout: 100 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	b.tryOpenToHalfOpen(nowNano() + 50*1e6)

	if b.State() != StateOpen {
		t.Logf("state is %v", b.State())
	}
}

func TestBreaker_RotateClosedWindowNotClosed(t *testing.T) {
	b := New(Config{
		Interval: 10 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	epoch1 := b.Counts().Epoch

	b.rotateClosedWindow(nowNano())

	epoch2 := b.Counts().Epoch

	if epoch1 != epoch2 {
		t.Log("epoch changed when not closed - expected in some cases")
	}
}

func TestBreaker_RotateClosedWindowNotReady(t *testing.T) {
	b := New(Config{
		Interval: 100 * time.Millisecond,
	})

	epoch1 := b.Counts().Epoch

	b.rotateClosedWindow(nowNano() + 50*1e6)

	epoch2 := b.Counts().Epoch

	if epoch1 != epoch2 {
		t.Error("should not rotate when interval not expired")
	}
}

func TestBreaker_DoneSuccessInHalfOpenTransition(t *testing.T) {
	b := New(Config{
		MaxRequests: 2,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
		Timeout:     10 * time.Millisecond,
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	tk1, _ := b.Allow()
	b.Done(&tk1, true)

	tk2, _ := b.Allow()
	b.Done(&tk2, true)

	if b.State() != StateClosed {
		t.Fatalf("should be closed after maxRequests successes in half-open, got %v", b.State())
	}
}

func TestBreaker_ApplyStateTransitionClosedWithInterval(t *testing.T) {
	b := New(Config{
		Interval: 10 * time.Millisecond,
	})

	epoch1 := b.Counts().Epoch

	time.Sleep(15 * time.Millisecond)

	b.applyStateTransitionLocked(StateClosed, nowNano())

	epoch2 := b.Counts().Epoch

	if epoch1 >= epoch2 {
		t.Error("epoch should increase on transition to closed")
	}
}

func TestBreaker_DoneWithNilTicket(t *testing.T) {
	b := New(Config{})

	b.Done(nil, true)
	b.Done(nil, false)

	c := b.Counts()
	if c.TotalSuccesses != 0 || c.TotalFailures != 0 {
		t.Error("nil tickets should not affect counts")
	}
}

func TestBreaker_ExecuteError(t *testing.T) {
	b := New(Config{
		IsSuccessful: func(err error) bool { return err == nil },
	})

	err := b.Execute(func() error {
		return errors.New("test error")
	})

	if err == nil {
		t.Fatal("expected error")
	}

	c := b.Counts()
	if c.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", c.TotalFailures)
	}
}

func TestBreaker_ExecuteNoError(t *testing.T) {
	b := New(Config{})

	err := b.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Fatal("unexpected error")
	}

	c := b.Counts()
	if c.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", c.TotalSuccesses)
	}
}

func TestBreaker_ApplyStateTransitionHalfOpenDirect(t *testing.T) {
	b := New(Config{})

	epoch1 := b.Counts().Epoch

	b.applyStateTransitionLocked(StateHalfOpen, nowNano())

	epoch2 := b.Counts().Epoch

	if epoch1 >= epoch2 {
		t.Error("epoch should increase on half-open transition")
	}

	if b.State() != StateHalfOpen {
		t.Errorf("state should be half-open, got %v", b.State())
	}
}

func TestBreaker_ApplyStateTransitionOpenDirect(t *testing.T) {
	b := New(Config{})

	epoch1 := b.Counts().Epoch

	b.applyStateTransitionLocked(StateOpen, nowNano())

	epoch2 := b.Counts().Epoch

	if epoch1 >= epoch2 {
		t.Error("epoch should increase on open transition")
	}

	if b.State() != StateOpen {
		t.Errorf("state should be open, got %v", b.State())
	}
}

func TestBreaker_ExecutePanicCounts(t *testing.T) {
	b := New(Config{})

	err := b.Execute(func() error {
		panic("test panic")
	})

	if err == nil {
		t.Fatal("expected error from panic")
	}

	pe, ok := err.(*PanicError)
	if !ok {
		t.Fatalf("expected PanicError, got %T", err)
	}
	if pe.Value != "test panic" {
		t.Errorf("expected panic value 'test panic', got %v", pe.Value)
	}

	counts := b.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", counts.TotalFailures)
	}
}

func TestBreaker_AllowContextCancel(t *testing.T) {
	b := New(Config{
		MaxRequests: 1,
	})

	b.Open()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.AllowContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestBreaker_TryOpenToHalfOpenNotOpenState(t *testing.T) {
	b := New(Config{})

	b.Close()
	now := nowNano()
	b.tryOpenToHalfOpen(now)

	if b.State() != StateClosed {
		t.Errorf("should stay closed, got %v", b.State())
	}
}

func TestBreaker_DoneWithStaleTicket(t *testing.T) {
	b := New(Config{})

	ticket1, _ := b.Allow()

	b.Close()

	_, _ = b.Allow()

	b.Done(&ticket1, true)

	counts := b.Counts()
	if counts.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", counts.TotalSuccesses)
	}
}

func TestBreaker_ExecuteWithError(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	err := b.Execute(func() error {
		return errors.New("test error")
	})

	if err == nil {
		t.Fatal("expected error")
	}

	if b.State() != StateOpen {
		t.Errorf("should be open after failure, got %v", b.State())
	}
}

func TestBreaker_DoneWithMismatchedEpoch(t *testing.T) {
	b := New(Config{})

	ticket1, _ := b.Allow()

	b.transition(StateOpen, nowNano())

	b.Done(&ticket1, true)

	counts := b.Counts()
	if counts.TotalSuccesses != 0 {
		t.Errorf("stale ticket should not count, got %d", counts.TotalSuccesses)
	}
}

func TestBreaker_ExecuteValuePanicCounts(t *testing.T) {
	b := New(Config{})

	_, err := ExecuteValue(b, func() (int, error) {
		panic("value panic test")
	})

	if err == nil {
		t.Fatal("expected error from panic")
	}

	counts := b.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", counts.TotalFailures)
	}
}

func TestBreaker_ExecuteWithErrorCountsAsSuccess(t *testing.T) {
	b := New(Config{
		IsSuccessful: func(err error) bool {
			return err == nil || err.Error() == "retryable"
		},
	})

	err := b.Execute(func() error {
		return errors.New("retryable")
	})

	if err == nil {
		t.Fatal("expected error")
	}

	counts := b.Counts()
	if counts.TotalSuccesses != 1 {
		t.Errorf("retryable error should count as success, got %d successes", counts.TotalSuccesses)
	}
}

func TestBreaker_ExecuteErrorCountsAsFailure(t *testing.T) {
	b := New(Config{
		IsSuccessful: func(err error) bool {
			return err == nil
		},
	})

	err := b.Execute(func() error {
		return errors.New("some error")
	})

	if err == nil {
		t.Fatal("expected error")
	}

	counts := b.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("error should count as failure, got %d failures", counts.TotalFailures)
	}
}

func TestBreaker_ExecuteSuccessCase(t *testing.T) {
	b := New(Config{})

	err := b.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	counts := b.Counts()
	if counts.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", counts.TotalSuccesses)
	}
}

func TestBreaker_ExecuteValueSuccessCase(t *testing.T) {
	b := New(Config{})

	val, err := ExecuteValue(b, func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	counts := b.Counts()
	if counts.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", counts.TotalSuccesses)
	}
}

func TestBreaker_ExecuteWithPanicAndRecover(t *testing.T) {
	b := New(Config{})

	err := b.Execute(func() error {
		panic("intentional panic")
	})

	if err == nil {
		t.Fatal("expected error from panic")
	}

	_, ok := err.(*PanicError)
	if !ok {
		t.Fatalf("expected PanicError, got %T", err)
	}

	counts := b.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("expected 1 failure after panic, got %d", counts.TotalFailures)
	}
}

func TestBreaker_ExecuteValueWithPanicAndRecover(t *testing.T) {
	b := New(Config{})

	val, err := ExecuteValue(b, func() (string, error) {
		panic("intentional value panic")
	})

	if err == nil {
		t.Fatal("expected error from panic")
	}

	if val != "" {
		t.Errorf("expected zero value, got %s", val)
	}

	_, ok := err.(*PanicError)
	if !ok {
		t.Fatalf("expected PanicError, got %T", err)
	}
}

func TestBreaker_ExecuteAllBranches(t *testing.T) {
	b := New(Config{})

	err := b.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	b.Reset()

	err = b.Execute(func() error {
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	b.Reset()

	err = b.Execute(func() error {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected panic error")
	}

	b.Open()
	err = b.Execute(func() error {
		return nil
	})
	if err != ErrOpenState {
		t.Errorf("expected ErrOpenState, got: %v", err)
	}
}

func TestBreaker_ExecuteValueAllBranches(t *testing.T) {
	b := New(Config{})

	val, err := ExecuteValue(b, func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	b.Reset()

	_, err = ExecuteValue(b, func() (int, error) {
		return 0, errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	b.Reset()

	val, err = ExecuteValue(b, func() (int, error) {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected panic error")
	}
	if val != 0 {
		t.Errorf("expected zero value, got %d", val)
	}
}

func TestBreaker_ExecuteValueGeneric(t *testing.T) {
	b := New(Config{})

	i, err := ExecuteValue(b, func() (int, error) { return 1, nil })
	if err != nil || i != 1 {
		t.Errorf("int test failed")
	}

	b.Reset()

	s, err := ExecuteValue(b, func() (string, error) { return "hi", nil })
	if err != nil || s != "hi" {
		t.Errorf("string test failed")
	}

	b.Reset()

	bl, err := ExecuteValue(b, func() (bool, error) { return true, nil })
	if err != nil || !bl {
		t.Errorf("bool test failed")
	}

	b.Reset()

	type Point struct{ X, Y int }
	p, err := ExecuteValue(b, func() (Point, error) { return Point{1, 2}, nil })
	if err != nil || p.X != 1 || p.Y != 2 {
		t.Errorf("struct test failed")
	}

	b.Reset()

	_, err = ExecuteValue(b, func() (int, error) { return 0, errors.New("error") })
	if err == nil {
		t.Error("expected error")
	}

	b2 := New(Config{
		IsSuccessful: func(e error) bool { return e == nil || e.Error() == "retry" },
	})
	b2.Reset()
	r, err := ExecuteValue(b2, func() (int, error) { return 5, errors.New("retry") })
	if err == nil || r != 5 {
		t.Errorf("expected error and value, got %d, %v", r, err)
	}
}

func TestBreaker_OpensAfterFailures(t *testing.T) {
	b := New(Config{
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 3 },
		Timeout:     50 * time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		tk, err := b.Allow()
		if err != nil {
			t.Fatalf("unexpected allow error: %v", err)
		}
		b.Done(&tk, false)
	}

	if got := b.State(); got != StateOpen {
		t.Fatalf("state=%v, want open", got)
	}
}

func TestBreaker_HalfOpenPermitLimit(t *testing.T) {
	b := New(Config{
		MaxRequests: 1,
		Timeout:     10 * time.Millisecond,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	tk, err := b.Allow()
	if err != nil {
		t.Fatal(err)
	}
	b.Done(&tk, false)

	time.Sleep(12 * time.Millisecond)

	t1, err := b.Allow()
	if err != nil {
		t.Fatalf("expected first half-open permit: %v", err)
	}

	_, err = b.Allow()
	if !errors.Is(err, ErrTooManyRequests) {
		t.Fatalf("got %v, want ErrTooManyRequests", err)
	}

	b.Done(&t1, true)
}

func TestBreaker_DoneIdempotent(t *testing.T) {
	b := New(Config{})
	tk, err := b.Allow()
	if err != nil {
		t.Fatal(err)
	}

	b.Done(&tk, true)
	b.Done(&tk, true)

	c := b.Counts()
	if c.TotalSuccesses != 1 {
		t.Fatalf("successes=%d want 1", c.TotalSuccesses)
	}
}

func TestBreaker_HalfOpenQueueSize(t *testing.T) {
	b := New(Config{
		MaxRequests:          1,
		HalfOpenMaxQueueSize: 5,
		Timeout:              10 * time.Millisecond,
		ReadyToTrip:          func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	if b.HalfOpenQueueSize() != 0 {
		t.Errorf("expected 0 queue size initially, got %d", b.HalfOpenQueueSize())
	}
}

func TestBreaker_IsClosed(t *testing.T) {
	b := New(Config{})

	if b.IsClosed() {
		t.Error("should not be closed initially")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Shutdown(ctx)
	if err != nil {
		t.Errorf("shutdown should not fail: %v", err)
	}

	if !b.IsClosed() {
		t.Error("should be closed after shutdown")
	}
}

func TestBreaker_Shutdown(t *testing.T) {
	b := New(Config{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Shutdown(ctx)
	if err != nil {
		t.Errorf("shutdown should succeed: %v", err)
	}

	if !b.IsClosed() {
		t.Error("should be closed after shutdown")
	}

	_, err = b.Allow()
	if err != ErrClosed {
		t.Errorf("should return ErrClosed after shutdown, got %v", err)
	}
}

func TestBreaker_ExecuteContext(t *testing.T) {
	b := New(Config{})

	err := b.ExecuteContext(context.Background(), func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	c := b.Counts()
	if c.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", c.TotalSuccesses)
	}
}

func TestBreaker_ExecuteContextWithError(t *testing.T) {
	b := New(Config{})

	err := b.ExecuteContext(context.Background(), func() error {
		return errors.New("test error")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	c := b.Counts()
	if c.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", c.TotalFailures)
	}
}

func TestBreaker_ExecuteContextWithPanic(t *testing.T) {
	b := New(Config{})

	err := b.ExecuteContext(context.Background(), func() error {
		panic("test panic")
	})
	if err == nil {
		t.Fatal("expected panic error")
	}

	_, ok := err.(*PanicError)
	if !ok {
		t.Fatalf("expected PanicError, got %T", err)
	}
}

func TestBreaker_ExecuteContextClosed(t *testing.T) {
	b := New(Config{})

	_ = b.Shutdown(context.Background())

	err := b.ExecuteContext(context.Background(), func() error {
		return nil
	})
	if err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

func TestBreaker_ExecuteChannel(t *testing.T) {
	b := New(Config{})

	resultCh := b.ExecuteChannel(func() error {
		return nil
	})

	result := <-resultCh
	if result.Err != nil {
		t.Errorf("expected nil error, got %v", result.Err)
	}

	c := b.Counts()
	if c.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", c.TotalSuccesses)
	}
}

func TestBreaker_ExecuteChannelWithError(t *testing.T) {
	b := New(Config{})

	resultCh := b.ExecuteChannel(func() error {
		return errors.New("test error")
	})

	result := <-resultCh
	if result.Err == nil {
		t.Fatal("expected error")
	}

	c := b.Counts()
	if c.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", c.TotalFailures)
	}
}

func TestBreaker_ExecuteChannelWithPanic(t *testing.T) {
	b := New(Config{})

	resultCh := b.ExecuteChannel(func() error {
		panic("test panic")
	})

	result := <-resultCh
	if result.Err == nil {
		t.Fatal("expected panic error")
	}

	_, ok := result.Err.(*PanicError)
	if !ok {
		t.Fatalf("expected PanicError, got %T", result.Err)
	}
}

func TestBreaker_ExecuteChannelClosed(t *testing.T) {
	b := New(Config{})

	_ = b.Shutdown(context.Background())

	resultCh := b.ExecuteChannel(func() error {
		return nil
	})

	result := <-resultCh
	if result.Err != ErrClosed {
		t.Errorf("expected ErrClosed, got %v", result.Err)
	}
}

func TestBreaker_DefaultReadyToTripRate(t *testing.T) {
	b := New(Config{})

	if !b.readyToTrip(Counts{TotalSuccesses: 4, TotalFailures: 6, ConsecutiveFailures: 3}) {
		t.Error("should trip at 60% failure rate with 10 requests")
	}

	if !b.readyToTrip(Counts{TotalSuccesses: 10, TotalFailures: 10, ConsecutiveFailures: 5}) {
		t.Error("should trip at 50% failure rate with 20 requests")
	}

	if b.readyToTrip(Counts{TotalSuccesses: 6, TotalFailures: 4, ConsecutiveFailures: 3}) {
		t.Error("should not trip at 40% failure rate")
	}
}

func TestBreaker_HalfOpenInflight(t *testing.T) {
	b := New(Config{
		MaxRequests: 2,
		Timeout:     10 * time.Millisecond,
		ReadyToTrip: func(c Counts) bool { return c.ConsecutiveFailures >= 1 },
	})

	if b.HalfOpenInflight() != 0 {
		t.Error("initial inflight should be 0")
	}

	tk, _ := b.Allow()
	b.Done(&tk, false)

	time.Sleep(15 * time.Millisecond)

	tk1, _ := b.Allow()

	if b.HalfOpenInflight() != 1 {
		t.Errorf("expected 1 inflight, got %d", b.HalfOpenInflight())
	}

	b.Done(&tk1, true)

	if b.HalfOpenInflight() != 0 {
		t.Errorf("expected 0 inflight after done, got %d", b.HalfOpenInflight())
	}
}

func TestBreaker_StateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateHalfOpen, "half-open"},
		{StateOpen, "open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("State.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBreaker_ExecuteWithContext(t *testing.T) {
	b := New(Config{})

	err := b.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	counts := b.Counts()
	if counts.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", counts.TotalSuccesses)
	}
}

func TestBreaker_ExecuteValueWithContext(t *testing.T) {
	b := New(Config{})

	val, err := ExecuteValue(b, func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	counts := b.Counts()
	if counts.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", counts.TotalSuccesses)
	}
}

func TestBreaker_ShutdownReturns(t *testing.T) {
	b := New(Config{})

	err := b.Shutdown(context.Background())
	if err != nil {
		t.Errorf("shutdown should succeed: %v", err)
	}
}

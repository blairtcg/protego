package protego

import (
	"testing"
	"time"
)

func TestState_Values(t *testing.T) {
	if StateClosed != 1 {
		t.Errorf("StateClosed should be 1, got %d", StateClosed)
	}
	if StateHalfOpen != 2 {
		t.Errorf("StateHalfOpen should be 2, got %d", StateHalfOpen)
	}
	if StateOpen != 3 {
		t.Errorf("StateOpen should be 3, got %d", StateOpen)
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}

	if cfg.MaxRequests != 0 {
		t.Errorf("default MaxRequests should be 0, got %d", cfg.MaxRequests)
	}
	if cfg.Interval != 0 {
		t.Errorf("default Interval should be 0, got %v", cfg.Interval)
	}
	if cfg.Timeout != 0 {
		t.Errorf("default Timeout should be 0, got %v", cfg.Timeout)
	}
}

func TestConfig_WithOptions(t *testing.T) {
	var called bool
	cfg := Config{
		Name:        "test-breaker",
		MaxRequests: 10,
		Interval:    5 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(c Counts) bool {
			return c.TotalFailures >= 3
		},
		IsSuccessful: func(err error) bool {
			return err == nil
		},
		OnStateChange: func(_ string, _, _ State) {
			called = true
		},
	}

	if cfg.Name != "test-breaker" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-breaker")
	}
	if cfg.MaxRequests != 10 {
		t.Errorf("MaxRequests = %d, want 10", cfg.MaxRequests)
	}
	if cfg.Interval != 5*time.Second {
		t.Errorf("Interval = %v, want 5s", cfg.Interval)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if !cfg.ReadyToTrip(Counts{TotalFailures: 3}) {
		t.Error("ReadyToTrip should return true for 3 failures")
	}
	if cfg.ReadyToTrip(Counts{TotalFailures: 2}) {
		t.Error("ReadyToTrip should return false for 2 failures")
	}
	if !cfg.IsSuccessful(nil) {
		t.Error("IsSuccessful should return true for nil")
	}
	if cfg.IsSuccessful(nil) == false {
		t.Error("IsSuccessful should return false for error")
	}
	cfg.OnStateChange("test", StateClosed, StateOpen)
	if !called {
		t.Error("OnStateChange should be called")
	}
}

func TestTicket_ConsumeOnce(t *testing.T) {
	ticket := Ticket{used: 1}

	if !ticket.consumeOnce() {
		t.Error("consumeOnce should return true on first call")
	}

	if ticket.consumeOnce() {
		t.Error("consumeOnce should return false on second call")
	}

	if ticket.consumeOnce() {
		t.Error("consumeOnce should return false on third call")
	}
}

func TestTicket_Fresh(t *testing.T) {
	ticket := Ticket{used: 0}

	if ticket.consumeOnce() {
		t.Error("consumeOnce should return false when used is 0")
	}
}

func TestTicketFlag(t *testing.T) {
	if ticketFlagHalfOpen != 1 {
		t.Errorf("ticketFlagHalfOpen should be 1, got %d", ticketFlagHalfOpen)
	}
}

func TestCounts_ZeroValue(t *testing.T) {
	var counts Counts

	if counts.Epoch != 0 {
		t.Errorf("Epoch should be 0, got %d", counts.Epoch)
	}
	if counts.Requests != 0 {
		t.Errorf("Requests should be 0, got %d", counts.Requests)
	}
	if counts.TotalSuccesses != 0 {
		t.Errorf("TotalSuccesses should be 0, got %d", counts.TotalSuccesses)
	}
	if counts.TotalFailures != 0 {
		t.Errorf("TotalFailures should be 0, got %d", counts.TotalFailures)
	}
	if counts.ConsecutiveSuccesses != 0 {
		t.Errorf("ConsecutiveSuccesses should be 0, got %d", counts.ConsecutiveSuccesses)
	}
	if counts.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures should be 0, got %d", counts.ConsecutiveFailures)
	}
}

package protego

import (
	"context"
	"sync/atomic"
	"time"
)

// State represents the state of a circuit breaker.
type State uint32

// These constants are the possible states of a CircuitBreaker.
const (
	StateClosed   State = 1 // Closed is the normal state where requests pass through.
	StateHalfOpen State = 2 // HalfOpen allows a limited number of requests through to test if the service has recovered.
	StateOpen     State = 3 // Open rejects all requests because the service is not working.
)

// String implements the stringer interface.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// Counts holds the numbers of requests and their successes/failures.
// The breaker clears the internal Counts when the state changes or at closed-state intervals.
type Counts struct {
	Epoch                uint64
	Requests             uint64
	TotalSuccesses       uint64
	TotalFailures        uint64
	ConsecutiveSuccesses uint64
	ConsecutiveFailures  uint64
}

// Config configures a CircuitBreaker.
//
// Name is the name of the CircuitBreaker.
//
// MaxRequests is the maximum number of requests allowed to pass through
// when the CircuitBreaker is half-open.
// If MaxRequests is 0, the CircuitBreaker allows only 1 request.
//
// Interval is the cyclic period of the closed state
// for the CircuitBreaker to clear the internal counts.
// If Interval is 0, the CircuitBreaker does not clear counts during the closed state.
//
// Timeout is the period of the open state,
// after which the state of the CircuitBreaker becomes half-open.
// If Timeout is 0, the timeout is set to 60 seconds.
//
// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state.
// If ReadyToTrip returns true, the CircuitBreaker will be placed into the open state.
// If ReadyToTrip is nil, the default ReadyToTrip is used.
// Default ReadyToTrip returns true when there are 6 consecutive failures.
//
// OnStateChange is called whenever the state of the CircuitBreaker changes.
//
// IsSuccessful is called with the error returned from a request.
// If IsSuccessful returns true, the error is counted as a success.
// If IsSuccessful is nil, default IsSuccessful is used, which returns false for all non-nil errors.
//
// HalfOpenMaxQueueSize is the maximum number of requests that can wait in the queue
// when the breaker is in half-open state. If 0, no queue is used.
//
// OnRequestQueued is called when a request is added to the half-open queue.
// OnRequestDropped is called when a request is dropped because the queue is full.
type Config struct {
	Name string // Name is the name of the CircuitBreaker.

	MaxRequests uint32 // MaxRequests is the max requests in half-open state.

	Interval time.Duration // Interval is the rotation interval for closed state.

	Timeout time.Duration // Timeout is the time before transitioning to half-open.

	ReadyToTrip func(Counts) bool // ReadyToTrip determines when to open the breaker.

	IsSuccessful func(error) bool // IsSuccessful determines if an error is successful.

	OnStateChange func(name string, from State, to State) // OnStateChange is called on state transitions.

	HalfOpenMaxQueueSize uint32 // HalfOpenMaxQueueSize is the max queue size in half-open state.

	OnRequestQueued func(name string, queueSize uint32) // OnRequestQueued is called when a request is queued.

	OnRequestDropped func(name string) // OnRequestDropped is called when a request is dropped.
}

// Ticket is used to report the result of a request.
type Ticket struct {
	epoch uint64
	flags uint32
	used  uint32
}

const ticketFlagHalfOpen = 1

func (t *Ticket) consumeOnce() bool {
	return atomic.CompareAndSwapUint32(&t.used, 1, 0)
}

// BreakerInterface defines the interface for a circuit breaker.
type BreakerInterface interface {
	Name() string
	State() State
	Counts() Counts
	Allow() (Ticket, error)
	Done(t *Ticket, success bool)
	Execute(work func() error) error
	Open()
	Close()
	Reset()
	Shutdown(ctx context.Context) error
}

var _ BreakerInterface = (*Breaker)(nil)

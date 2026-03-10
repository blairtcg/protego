// Package protego implements the Circuit Breaker pattern.
// See https://learn.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker
package protego

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Breaker is a state machine that prevents sending requests that are likely to fail.
type Breaker struct {
	name string

	maxRequests uint32
	intervalNS  int64
	timeoutNS   int64

	readyToTrip  func(Counts) bool
	isSuccessful func(error) bool
	onStateChg   func(name string, from State, to State)

	state  atomic.Uint32
	epoch  atomic.Uint64
	expiry atomic.Int64

	req          atomic.Uint64
	succ         atomic.Uint64
	fail         atomic.Uint64
	consecSucc   atomic.Uint64
	consecFail   atomic.Uint64
	halfInFlight atomic.Uint32

	halfOpenQueue    atomic.Uint32
	halfOpenMaxQueue uint32
	onRequestQueued  func(name string, queueSize uint32)
	onRequestDropped func(name string)

	halfOpenCond *sync.Cond
	closed       atomic.Bool

	transMu sync.Mutex
}

const defaultTimeout = 60 * time.Second

const (
	failureRateThreshold    = 0.5
	minRequestsForRateCheck = 10
	consecutiveFailureTrip  = 6
)

func defaultReadyToTrip(c Counts) bool {
	if c.TotalRequests() < minRequestsForRateCheck {
		return c.ConsecutiveFailures >= consecutiveFailureTrip
	}
	failureRate := float64(c.TotalFailures) / float64(c.TotalRequests())
	return failureRate >= failureRateThreshold || c.ConsecutiveFailures >= consecutiveFailureTrip
}

// New creates a new Breaker with the given configuration.
func New(cfg Config) *Breaker {
	b := &Breaker{
		name:             cfg.Name,
		halfOpenMaxQueue: cfg.HalfOpenMaxQueueSize,
		onRequestQueued:  cfg.OnRequestQueued,
		onRequestDropped: cfg.OnRequestDropped,
	}

	if cfg.MaxRequests == 0 {
		b.maxRequests = 1
	} else {
		b.maxRequests = cfg.MaxRequests
	}

	if cfg.Interval > 0 {
		b.intervalNS = cfg.Interval.Nanoseconds()
	}
	if cfg.Timeout <= 0 {
		b.timeoutNS = int64(defaultTimeout)
	} else {
		b.timeoutNS = cfg.Timeout.Nanoseconds()
	}

	if cfg.ReadyToTrip == nil {
		b.readyToTrip = defaultReadyToTrip
	} else {
		b.readyToTrip = cfg.ReadyToTrip
	}

	if cfg.IsSuccessful == nil {
		b.isSuccessful = func(err error) bool { return err == nil }
	} else {
		b.isSuccessful = cfg.IsSuccessful
	}

	b.onStateChg = cfg.OnStateChange

	b.halfOpenCond = sync.NewCond(&b.transMu)

	b.state.Store(uint32(StateClosed))
	b.epoch.Store(1)
	if b.intervalNS > 0 {
		b.expiry.Store(nowNano() + b.intervalNS)
	}
	return b
}

// Name returns the name of the breaker.
func (b *Breaker) Name() string {
	return b.name
}

// State returns the current state of the breaker.
func (b *Breaker) State() State {
	b.refreshByTime(nowNano())
	return State(b.state.Load())
}

// Counts returns the current counts for the breaker.
func (b *Breaker) Counts() Counts {
	return Counts{
		Epoch:                b.epoch.Load(),
		Requests:             b.req.Load(),
		TotalSuccesses:       b.succ.Load(),
		TotalFailures:        b.fail.Load(),
		ConsecutiveSuccesses: b.consecSucc.Load(),
		ConsecutiveFailures:  b.consecFail.Load(),
	}
}

// TotalRequests returns the total number of requests.
func (c Counts) TotalRequests() uint64 {
	return c.TotalSuccesses + c.TotalFailures
}

// HalfOpenInflight returns the number of in-flight requests in half-open state.
func (b *Breaker) HalfOpenInflight() uint32 {
	return b.halfInFlight.Load()
}

// HalfOpenQueueSize returns the current queue size in half-open state.
func (b *Breaker) HalfOpenQueueSize() uint32 {
	return b.halfOpenQueue.Load()
}

// IsClosed returns whether the breaker has been shut down.
func (b *Breaker) IsClosed() bool {
	return b.closed.Load()
}

func (b *Breaker) countsWithState() Counts {
	for {
		epoch := b.epoch.Load()
		counts := Counts{
			Epoch:                epoch,
			Requests:             b.req.Load(),
			TotalSuccesses:       b.succ.Load(),
			TotalFailures:        b.fail.Load(),
			ConsecutiveSuccesses: b.consecSucc.Load(),
			ConsecutiveFailures:  b.consecFail.Load(),
		}
		if epoch == b.epoch.Load() {
			return counts
		}
	}
}

// Allow checks if a request can proceed.
func (b *Breaker) Allow() (Ticket, error) {
	if b.closed.Load() {
		return Ticket{}, ErrClosed
	}

	now := nowNano()
	exp := b.expiry.Load()

	var stateVal uint32
	if exp != 0 {
		stateVal = b.state.Load()
		if stateVal == uint32(StateOpen) && now >= exp {
			b.tryOpenToHalfOpen(now)
		} else if stateVal == uint32(StateClosed) && now >= exp {
			b.rotateClosedWindow(now)
		}
		stateVal = b.state.Load()
	} else {
		stateVal = b.state.Load()
	}

	return b.issueTicket(stateVal)
}

func (b *Breaker) issueTicket(stateVal uint32) (Ticket, error) {
	if stateVal == uint32(StateClosed) {
		ep := b.epoch.Load()
		b.req.Add(1)
		return Ticket{epoch: ep, used: 1}, nil
	}

	if stateVal == uint32(StateOpen) {
		return Ticket{}, NewOpenError(nil)
	}

	if !b.tryAcquireHalfOpenPermit() {
		if b.halfOpenMaxQueue > 0 {
			queueSize := b.halfOpenQueue.Add(1)
			if queueSize > b.halfOpenMaxQueue {
				b.halfOpenQueue.Add(^uint32(0))
				if b.onRequestDropped != nil {
					b.onRequestDropped(b.name)
				}
				return Ticket{}, NewTooManyRequestsError(nil)
			}
			if b.onRequestQueued != nil {
				b.onRequestQueued(b.name, queueSize)
			}
			b.halfOpenCond.Wait()
			b.halfOpenQueue.Add(^uint32(0))
		}
		if !b.tryAcquireHalfOpenPermit() {
			return Ticket{}, NewTooManyRequestsError(nil)
		}
	}

	ep := b.epoch.Load()
	b.req.Add(1)
	return Ticket{epoch: ep, used: 1, flags: ticketFlagHalfOpen}, nil
}

// Done reports the result of a request.
func (b *Breaker) Done(t *Ticket, success bool) {
	if t == nil || !t.consumeOnce() {
		return
	}

	if b.epoch.Load() != t.epoch {
		return
	}

	if (t.flags & ticketFlagHalfOpen) != 0 {
		b.decHalfOpenPermit()
		b.signalHalfOpenWaiters()
	}

	if success {
		b.succ.Add(1)
		b.consecSucc.Add(1)
		b.consecFail.Store(0)

		b.transMu.Lock()
		if b.state.Load() == uint32(StateHalfOpen) && b.consecSucc.Load() >= uint64(b.maxRequests) {
			from := StateHalfOpen
			b.applyStateTransitionLocked(StateClosed, nowNano())
			b.transMu.Unlock()
			b.doStateChangeCallback(from, StateClosed)
		} else {
			b.transMu.Unlock()
		}
		return
	}

	b.fail.Add(1)
	b.consecFail.Add(1)
	b.consecSucc.Store(0)

	stateVal := b.state.Load()
	if stateVal == uint32(StateClosed) {
		if b.readyToTrip(b.countsWithState()) {
			b.transition(StateOpen, nowNano())
		}
	} else if stateVal == uint32(StateHalfOpen) {
		b.transition(StateOpen, nowNano())
	}
}

func (b *Breaker) signalHalfOpenWaiters() {
	b.halfOpenCond.Broadcast()
}

// Execute runs the given work function through the breaker.
// Execute returns an error instantly if the Breaker rejects the request.
// Otherwise, Execute returns the result of the work function.
// If a panic occurs in the work function, the Breaker handles it as an error
// and causes the same panic again.
func (b *Breaker) Execute(work func() error) (err error) {
	if b.closed.Load() {
		return ErrClosed
	}

	t, err := b.Allow()
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			b.Done(&t, false)
			err = &PanicError{Value: r}
			return
		}
	}()

	err = work()
	b.Done(&t, b.isSuccessful(err))
	return err
}

// ExecuteValue runs the given work function and returns a value.
// ExecuteValue returns an error instantly if the Breaker rejects the request.
// Otherwise, ExecuteValue returns the result of the work function.
// If a panic occurs in the work function, the Breaker handles it as an error
// and causes the same panic again.
func ExecuteValue[T any](b *Breaker, work func() (T, error)) (v T, err error) {
	if b.closed.Load() {
		return v, ErrClosed
	}

	t, err := b.Allow()
	if err != nil {
		return v, err
	}

	defer func() {
		if r := recover(); r != nil {
			b.Done(&t, false)
			err = &PanicError{Value: r}
			return
		}
	}()

	v, err = work()
	b.Done(&t, b.isSuccessful(err))
	return v, err
}

// Open transitions the breaker to the open state.
// When the breaker is open, it will reject all requests.
func (b *Breaker) Open() {
	b.transition(StateOpen, nowNano())
}

// Close transitions the breaker to the closed state.
// When the breaker is closed, requests pass through normally.
func (b *Breaker) Close() {
	b.transition(StateClosed, nowNano())
}

// Reset transitions the breaker to closed state and clears counts.
func (b *Breaker) Reset() {
	b.transMu.Lock()
	defer b.transMu.Unlock()

	oldState := State(b.state.Load())
	b.applyStateTransitionLocked(StateClosed, nowNano())
	b.doStateChangeCallback(oldState, StateClosed)
}

// Shutdown gracefully shuts down the breaker, waiting for in-flight requests.
// Shutdown sets a flag that stops the breaker from accepting new requests.
// It then waits until all in-flight requests have completed.
// If the context is cancelled before all requests complete, Shutdown returns the context error.
func (b *Breaker) Shutdown(ctx context.Context) error {
	b.closed.Store(true)

	for {
		inflight := b.halfInFlight.Load()
		queueSize := b.halfOpenQueue.Load()
		if inflight == 0 && queueSize == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// AllowContext checks if a request can proceed with context support.
// It will wait if the breaker is open until it transitions to half-open.
func (b *Breaker) AllowContext(ctx context.Context) (Ticket, error) {
	if b.closed.Load() {
		return Ticket{}, ErrClosed
	}

	timer := time.NewTimer(15 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return Ticket{}, ctx.Err()
		default:
		}

		ticket, err := b.Allow()
		if err == nil {
			return ticket, nil
		}

		isOpen := err == ErrOpenState
		isTooMany := err == ErrTooManyRequests
		if !isOpen && !isTooMany {
			return ticket, err
		}

		currentState := b.State()
		if currentState == StateHalfOpen {
			return b.Allow()
		}

		if currentState == StateOpen {
			select {
			case <-ctx.Done():
				return Ticket{}, ctx.Err()
			case <-timer.C:
				timer.Reset(15 * time.Millisecond)
			}
		}
	}
}

// ExecuteContext executes work with context support.
// ExecuteContext returns an error instantly if the Breaker rejects the request.
// Otherwise, ExecuteContext returns the result of the work function.
// If a panic occurs in the work function, the Breaker handles it as an error
// and causes the same panic again.
func (b *Breaker) ExecuteContext(ctx context.Context, work func() error) error {
	if b.closed.Load() {
		return ErrClosed
	}

	ticket, err := b.AllowContext(ctx)
	if err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				b.Done(&ticket, false)
				done <- &PanicError{Value: r}
				return
			}
		}()
		err := work()
		b.Done(&ticket, b.isSuccessful(err))
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// ExecuteChannel executes work and returns a channel for the result.
// The work function runs in a separate goroutine.
// The channel returns the result when the work function completes.
func (b *Breaker) ExecuteChannel(work func() error) chan Result {
	if b.closed.Load() {
		resultCh := make(chan Result, 1)
		resultCh <- Result{Err: ErrClosed}
		close(resultCh)
		return resultCh
	}

	resultCh := make(chan Result, 1)
	go func() {
		defer close(resultCh)

		ticket, err := b.Allow()
		if err != nil {
			resultCh <- Result{Err: err}
			return
		}

		defer func() {
			if r := recover(); r != nil {
				b.Done(&ticket, false)
				resultCh <- Result{Err: &PanicError{Value: r}}
				return
			}
		}()

		err = work()
		b.Done(&ticket, b.isSuccessful(err))
		resultCh <- Result{Err: err}
	}()
	return resultCh
}

// Result holds the result of an asynchronous execution.
type Result struct {
	Err error
}

func (b *Breaker) refreshByTime(now int64) {
	stateVal := b.state.Load()
	exp := b.expiry.Load()

	if exp == 0 {
		return
	}

	if stateVal == uint32(StateOpen) && now >= exp {
		b.tryOpenToHalfOpen(now)
	} else if stateVal == uint32(StateClosed) && b.intervalNS > 0 && now >= exp {
		b.rotateClosedWindow(now)
	}
}

func (b *Breaker) tryOpenToHalfOpen(now int64) {
	b.transMu.Lock()
	defer b.transMu.Unlock()

	if b.state.Load() != uint32(StateOpen) {
		return
	}
	exp := b.expiry.Load()
	if exp == 0 || now < exp {
		return
	}
	from := StateOpen
	to := StateHalfOpen
	b.applyStateTransitionLocked(to, now)
	b.doStateChangeCallback(from, to)
}

func (b *Breaker) doStateChangeCallback(from, to State) {
	cb := b.onStateChg
	if cb != nil {
		cb(b.name, from, to)
	}
}

func (b *Breaker) rotateClosedWindow(now int64) {
	b.transMu.Lock()
	defer b.transMu.Unlock()

	if b.state.Load() != uint32(StateClosed) {
		return
	}
	exp := b.expiry.Load()
	if exp == 0 || now < exp {
		return
	}

	b.epoch.Add(1)
	b.resetCountersLocked()
	b.expiry.Store(now + b.intervalNS)
}

func (b *Breaker) transition(to State, now int64) {
	b.transMu.Lock()

	from := State(b.state.Load())
	if from == to {
		b.transMu.Unlock()
		return
	}
	b.applyStateTransitionLocked(to, now)
	b.transMu.Unlock()

	b.doStateChangeCallback(from, to)
}

func (b *Breaker) applyStateTransitionLocked(to State, now int64) {
	b.state.Store(uint32(to))
	b.epoch.Add(1)

	switch to {
	case StateClosed:
		b.req.Store(0)
		b.succ.Store(0)
		b.fail.Store(0)
		b.consecSucc.Store(0)
		b.consecFail.Store(0)
		b.halfInFlight.Store(0)
		if b.intervalNS > 0 {
			b.expiry.Store(now + b.intervalNS)
		} else {
			b.expiry.Store(0)
		}
	case StateOpen:
		b.req.Store(0)
		b.succ.Store(0)
		b.fail.Store(0)
		b.consecSucc.Store(0)
		b.consecFail.Store(0)
		b.expiry.Store(now + b.timeoutNS)
	case StateHalfOpen:
		b.expiry.Store(0)
	}
}

func (b *Breaker) resetCountersLocked() {
	b.req.Store(0)
	b.succ.Store(0)
	b.fail.Store(0)
	b.consecSucc.Store(0)
	b.consecFail.Store(0)
	b.halfInFlight.Store(0)
}

func (b *Breaker) tryAcquireHalfOpenPermit() bool {
	maxReq := b.maxRequests
	if maxReq <= 1 {
		return b.halfInFlight.CompareAndSwap(0, 1)
	}
	for {
		n := b.halfInFlight.Load()
		if n >= maxReq {
			return false
		}
		if b.halfInFlight.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

func (b *Breaker) decHalfOpenPermit() {
	for {
		n := b.halfInFlight.Load()
		if n == 0 {
			return
		}
		if b.halfInFlight.CompareAndSwap(n, n-1) {
			return
		}
	}
}

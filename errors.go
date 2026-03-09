// Package protego provides a circuit breaker implementation.
package protego

import (
	"errors"
	"fmt"
)

// ErrOpenState is returned when the breaker is open and rejecting requests.
// Your code should stop calling the service and wait for recovery.
var ErrOpenState = errors.New("circuit breaker is open")

// ErrTooManyRequests is returned when the breaker is half-open and the request count
// is over the max requests allowed.
var ErrTooManyRequests = errors.New("too many requests")

// ErrClosed is returned when the breaker has been shut down and no longer accepts requests.
// Call Shutdown to gracefully stop accepting new requests.
var ErrClosed = errors.New("circuit breaker is closed")

// BreakerError wraps an error with more context.
// Use this to add details when returning errors from your code.
type BreakerError struct {
	Code    string // short code, for example "ERR_OPEN_STATE"
	Message string // readable message
	Err     error  // underlying error, if any
}

// Error returns a readable string for the error.
// If there was an underlying error, it includes that too.
func (e *BreakerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
// Use this to check if the error has a cause.
func (e *BreakerError) Unwrap() error {
	return e.Err
}

// Is checks if this error matches a target.
// This supports errors.Is() for comparing errors.
func (e *BreakerError) Is(target error) bool {
	switch target {
	case ErrOpenState:
		return e.Code == "ERR_OPEN_STATE"
	case ErrTooManyRequests:
		return e.Code == "ERR_TOO_MANY_REQUESTS"
	}
	return false
}

// openError wraps BreakerError to support error matching for open state.
type openError struct {
	*BreakerError
}

// Is lets you use errors.Is() to check for open state errors.
func (openError) Is(target error) bool {
	return target == ErrOpenState
}

// tooManyRequestsError wraps BreakerError to support error matching for half-open state.
type tooManyRequestsError struct {
	*BreakerError
}

// Is lets you use errors.Is() to check for too many requests errors.
func (tooManyRequestsError) Is(target error) bool {
	return target == ErrTooManyRequests
}

// NewOpenError creates an error for when the breaker is open.
// Pass the original error that caused the open state, or nil if there was none.
func NewOpenError(err error) error {
	if err == nil {
		return ErrOpenState
	}
	return &openError{&BreakerError{
		Code:    "ERR_OPEN_STATE",
		Message: "circuit breaker is open",
		Err:     err,
	}}
}

// NewTooManyRequestsError creates an error for when too many requests are waiting.
// Pass the original error, or nil if there was none.
func NewTooManyRequestsError(err error) error {
	if err == nil {
		return ErrTooManyRequests
	}
	return &tooManyRequestsError{&BreakerError{
		Code:    "ERR_TOO_MANY_REQUESTS",
		Message: "too many requests",
		Err:     err,
	}}
}

// PanicError holds a value that panicked in your code.
// The breaker catches the panic and returns this error instead of crashing.
type PanicError struct {
	Value interface{} // the value that was passed to panic()
}

// Error returns a message saying a panic happened and shows what panicked.
func (p *PanicError) Error() string {
	return fmt.Sprintf("panic recovered: %v", p.Value)
}

// Unwrap returns the error if the panicked value was an error.
// This lets you use errors.As() to recover the original error.
func (p *PanicError) Unwrap() error {
	if err, ok := p.Value.(error); ok {
		return err
	}
	return nil
}

// Is lets PanicError act like ErrOpenState or ErrTooManyRequests.
// This helps with error handling in code that expects those errors.
func (p *PanicError) Is(target error) bool {
	return target == ErrOpenState || target == ErrTooManyRequests
}

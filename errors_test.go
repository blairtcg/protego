package protego

import (
	"errors"
	"testing"
)

func TestErrOpenState(t *testing.T) {
	if ErrOpenState.Error() != "circuit breaker is open" {
		t.Errorf("ErrOpenState.Error() = %q", ErrOpenState.Error())
	}
}

func TestErrTooManyRequests(t *testing.T) {
	if ErrTooManyRequests.Error() != "too many requests" {
		t.Errorf("ErrTooManyRequests.Error() = %q", ErrTooManyRequests.Error())
	}
}

func TestBreakerError(t *testing.T) {
	inner := errors.New("inner error")
	err := &BreakerError{
		Code:    "TEST_CODE",
		Message: "test message",
		Err:     inner,
	}

	if err.Error() != "TEST_CODE: test message: inner error" {
		t.Errorf("Error() = %q", err.Error())
	}

	if !errors.Is(err, inner) {
		t.Error("Unwrap should return inner error")
	}
}

func TestBreakerError_NoInner(t *testing.T) {
	err := &BreakerError{
		Code:    "TEST_CODE",
		Message: "test message",
	}

	if err.Error() != "TEST_CODE: test message" {
		t.Errorf("Error() = %q", err.Error())
	}

	if err.Unwrap() != nil {
		t.Error("Unwrap should return nil when no inner error")
	}
}

func TestBreakerError_Is(t *testing.T) {
	errOpen := &BreakerError{Code: "ERR_OPEN_STATE", Message: "open"}
	errTooMany := &BreakerError{Code: "ERR_TOO_MANY_REQUESTS", Message: "too many"}
	errOther := &BreakerError{Code: "OTHER", Message: "other"}

	if !errOpen.Is(ErrOpenState) {
		t.Error("ERR_OPEN_STATE should match")
	}
	if errOpen.Is(ErrTooManyRequests) {
		t.Error("should not match ErrTooManyRequests")
	}
	if !errTooMany.Is(ErrTooManyRequests) {
		t.Error("ERR_TOO_MANY_REQUESTS should match")
	}
	if errOther.Is(ErrOpenState) {
		t.Error("should not match different code")
	}
}

func TestNewOpenError(t *testing.T) {
	err := NewOpenError(nil)
	if err != ErrOpenState {
		t.Error("NewOpenError(nil) should return ErrOpenState")
	}

	inner := errors.New("inner")
	err = NewOpenError(inner)
	if err == nil {
		t.Fatal("should not be nil")
	}
	if !errors.Is(err, inner) {
		t.Error("should unwrap to inner error")
	}
	if !errors.Is(err, ErrOpenState) {
		t.Error("should match ErrOpenState")
	}
}

func TestNewTooManyRequestsError(t *testing.T) {
	err := NewTooManyRequestsError(nil)
	if err != ErrTooManyRequests {
		t.Error("NewTooManyRequestsError(nil) should return ErrTooManyRequests")
	}

	inner := errors.New("inner")
	err = NewTooManyRequestsError(inner)
	if err == nil {
		t.Fatal("should not be nil")
	}
	if !errors.Is(err, inner) {
		t.Error("should unwrap to inner error")
	}
	if !errors.Is(err, ErrTooManyRequests) {
		t.Error("should match ErrTooManyRequests")
	}
}

func TestPanicError(t *testing.T) {
	pe := &PanicError{Value: "test panic"}
	if pe.Error() != "panic recovered: test panic" {
		t.Errorf("Error() = %q", pe.Error())
	}

	if pe.Unwrap() != nil {
		t.Error("Unwrap should return nil for non-error value")
	}
}

func TestPanicError_UnwrapError(t *testing.T) {
	inner := errors.New("inner panic")
	pe := &PanicError{Value: inner}

	if !errors.Is(pe, inner) {
		t.Error("should unwrap to inner error")
	}
}

func TestPanicError_Is(t *testing.T) {
	pe := &PanicError{Value: "test"}

	if !pe.Is(ErrOpenState) {
		t.Error("PanicError should match ErrOpenState")
	}
	if !pe.Is(ErrTooManyRequests) {
		t.Error("PanicError should match ErrTooManyRequests")
	}
}

func TestErrorTypes(t *testing.T) {
	var err error

	err = NewOpenError(nil)
	if err != ErrOpenState {
		t.Error("NewOpenError(nil) should return ErrOpenState directly")
	}

	err = NewTooManyRequestsError(nil)
	if err != ErrTooManyRequests {
		t.Error("NewTooManyRequestsError(nil) should return ErrTooManyRequests directly")
	}

	err = NewOpenError(errors.New("inner"))
	var openErr *openError
	if !errors.As(err, &openErr) {
		t.Error("should be able to cast to openError when wrapping error")
	}

	err = NewTooManyRequestsError(errors.New("inner"))
	var tooManyErr *tooManyRequestsError
	if !errors.As(err, &tooManyErr) {
		t.Error("should be able to cast to tooManyRequestsError when wrapping error")
	}
}

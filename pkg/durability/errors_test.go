package durability

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsTerminalError_default(t *testing.T) {
	if !IsTerminalError(errors.New("plain")) {
		t.Fatal("plain errors should default to terminal (no retry)")
	}
}

func TestPermanentError_isTerminal(t *testing.T) {
	err := &PermanentError{Err: errors.New("nope")}
	if !IsTerminalError(err) {
		t.Fatal("PermanentError must be terminal")
	}
	if IsRetryableError(err) {
		t.Fatal("PermanentError must not be retryable")
	}
}

func TestRetryableError_isNotTerminal(t *testing.T) {
	err := &RetryableError{Err: errors.New("transient")}
	if IsTerminalError(err) {
		t.Fatal("RetryableError must not be terminal")
	}
	if !IsRetryableError(err) {
		t.Fatal("RetryableError must be retryable")
	}
}

func TestRetryableError_unwrappedThroughFmt(t *testing.T) {
	inner := errors.New("network down")
	wrapped := fmt.Errorf("step bar: %w", &RetryableError{Err: inner})
	if IsTerminalError(wrapped) {
		t.Fatal("retryable classification must survive fmt.Errorf wrapping")
	}
	if !IsRetryableError(wrapped) {
		t.Fatal("retryable classification must survive fmt.Errorf wrapping")
	}
}

func TestPermanentError_unwrap(t *testing.T) {
	inner := errors.New("missing")
	err := &PermanentError{Err: inner}
	if !errors.Is(err, inner) {
		t.Fatal("PermanentError must unwrap to inner")
	}
	if err.Error() != "missing" {
		t.Fatalf("unexpected message: %q", err.Error())
	}
}

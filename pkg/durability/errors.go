package durability

import "errors"

// TerminalClassifier is implemented by errors that know whether they are permanent.
// RestateDurableCtx checks this interface: errors marked terminal are wrapped with
// restate.TerminalError so Restate stops retrying immediately.
type TerminalClassifier interface {
	IsTerminal() bool
}

// PermanentError marks an error as terminal for durable execution.
// Use this to wrap errors that should never be retried (e.g., resource not found,
// invalid configuration) without importing Restate.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string    { return e.Err.Error() }
func (e *PermanentError) Unwrap() error    { return e.Err }
func (e *PermanentError) IsTerminal() bool { return true }

// RetryableError marks an error as retryable for durable execution.
// By default all errors are terminal (no retry). Wrap with this to allow
// Restate to retry (e.g., transient network errors, rate limits).
type RetryableError struct{ Err error }

func (e *RetryableError) Error() string    { return e.Err.Error() }
func (e *RetryableError) Unwrap() error    { return e.Err }
func (e *RetryableError) IsTerminal() bool { return false }

// IsTerminalError checks whether err should be treated as terminal.
// Returns true unless the error (or any in its chain) is explicitly marked
// retryable via RetryableError.
func IsTerminalError(err error) bool {
	var tc TerminalClassifier
	if errors.As(err, &tc) {
		return tc.IsTerminal()
	}
	return true
}

// IsRetryableError checks whether err is explicitly marked as retryable.
func IsRetryableError(err error) bool {
	var tc TerminalClassifier
	return errors.As(err, &tc) && !tc.IsTerminal()
}

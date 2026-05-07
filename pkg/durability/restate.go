package durability

import (
	"context"

	restate "github.com/restatedev/sdk-go"
)

// RestateDurableCtx wraps a Restate Context to provide durable step execution.
// Each Run() call is journaled by Restate — on crash/restart, completed steps
// return cached results without re-execution.
// Works with any Restate context type (Context, ObjectContext, WorkflowContext).
type RestateDurableCtx struct {
	Rctx restate.Context
}

func (d *RestateDurableCtx) run(name string, fn func(ctx context.Context) (any, error), opts ...restate.RunOption) (any, error) {
	allOpts := append([]restate.RunOption{restate.WithName(name)}, opts...)
	result, err := restate.Run(d.Rctx, func(rc restate.RunContext) (any, error) {
		result, err := fn(rc)
		if err != nil && !IsRetryableError(err) {
			// Default: all errors are terminal unless explicitly wrapped
			// in RetryableError by the caller.
			return result, restate.TerminalError(err)
		}
		return result, err
	}, allOpts...)
	// On replay, Restate preserves terminal-ness (restate.IsTerminalError) but
	// strips Go error types. Re-wrap as PermanentError so callers can use
	// durability.IsTerminalError without parsing error strings.
	if err != nil && restate.IsTerminalError(err) && !IsTerminalError(err) {
		return result, &PermanentError{Err: err}
	}
	return result, err
}

func (d *RestateDurableCtx) Run(name string, fn func(ctx context.Context) (any, error)) (any, error) {
	return d.run(name, fn)
}

func (d *RestateDurableCtx) RunWithRetry(name string, policy RetryPolicy, fn func(ctx context.Context) (any, error)) (any, error) {
	var opts []restate.RunOption
	if policy.MaxAttempts > 0 {
		opts = append(opts, restate.WithMaxRetryAttempts(uint(policy.MaxAttempts)))
	}
	if policy.InitialInterval > 0 {
		opts = append(opts, restate.WithInitialRetryInterval(policy.InitialInterval))
	}
	return d.run(name, fn, opts...)
}

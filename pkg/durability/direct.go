package durability

import (
	"context"
	"log/slog"
	"time"
)

// DirectCtx executes functions inline without journaling.
// Used in tests and for library-mode embedding (e.g., governor importing flow's
// engine but not running a Restate process).
type DirectCtx struct {
	Ctx context.Context
}

func (d *DirectCtx) ctx() context.Context {
	if d.Ctx != nil {
		return d.Ctx
	}
	return context.Background()
}

func (d *DirectCtx) Run(_ string, fn func(ctx context.Context) (any, error)) (any, error) {
	return fn(d.ctx())
}

func (d *DirectCtx) RunWithRetry(name string, policy RetryPolicy, fn func(ctx context.Context) (any, error)) (any, error) {
	maxAttempts := policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	wait := policy.InitialInterval
	if wait <= 0 {
		wait = time.Second
	}

	ctx := d.ctx()
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := fn(ctx)
		if err == nil {
			if attempt > 0 {
				slog.InfoContext(ctx, "retry_succeeded",
					slog.String("step", name),
					slog.Int("attempt", attempt+1),
				)
			}
			return result, nil
		}
		lastErr = err
		if attempt < maxAttempts-1 {
			slog.WarnContext(ctx, "retry",
				slog.String("step", name),
				slog.Int("attempt", attempt+1),
				slog.Int("max", maxAttempts),
				slog.Any("error", err),
			)
			time.Sleep(wait)
		}
	}
	return nil, lastErr
}

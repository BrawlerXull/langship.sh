package durability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDirectCtx_Run_passesThrough(t *testing.T) {
	d := &DirectCtx{}
	out, err := d.Run("step", func(_ context.Context) (any, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != 42 {
		t.Fatalf("expected 42, got %v", out)
	}
}

func TestDirectCtx_RunWithRetry_succeedsOnSecondAttempt(t *testing.T) {
	d := &DirectCtx{}
	calls := 0
	out, err := d.RunWithRetry("step", RetryPolicy{MaxAttempts: 3, InitialInterval: time.Millisecond}, func(_ context.Context) (any, error) {
		calls++
		if calls < 2 {
			return nil, errors.New("flaky")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
	if out != "ok" {
		t.Fatalf("expected ok, got %v", out)
	}
}

func TestDirectCtx_RunWithRetry_givesUpAfterMaxAttempts(t *testing.T) {
	d := &DirectCtx{}
	calls := 0
	_, err := d.RunWithRetry("step", RetryPolicy{MaxAttempts: 3, InitialInterval: time.Millisecond}, func(_ context.Context) (any, error) {
		calls++
		return nil, errors.New("permanent")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDirectCtx_RunWithRetry_zeroAttemptsDefaultsToOne(t *testing.T) {
	d := &DirectCtx{}
	calls := 0
	_, err := d.RunWithRetry("step", RetryPolicy{}, func(_ context.Context) (any, error) {
		calls++
		return nil, errors.New("nope")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected single attempt, got %d", calls)
	}
}

func TestWithDurableCtx_roundTrip(t *testing.T) {
	d := &DirectCtx{}
	ctx := WithDurableCtx(context.Background(), d)
	got, ok := FromContext(ctx)
	if !ok || got != d {
		t.Fatal("DurableCtx round-trip failed")
	}
}

func TestFromContext_nilCtx(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("FromContext should be false on a bare context")
	}
}

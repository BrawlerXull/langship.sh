package durability

import (
	"context"
	"errors"
	"testing"
)

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// fakeReplayCtx returns map[string]any from Run instead of the original type,
// the way Restate behaves on journal replay. Verifies RunAs handles the
// JSON round-trip transparently.
type fakeReplayCtx struct{}

func (fakeReplayCtx) Run(_ string, fn func(ctx context.Context) (any, error)) (any, error) {
	v, err := fn(context.Background())
	if err != nil {
		return nil, err
	}
	// Simulate JSON round-trip ala Restate replay: encode then decode into map.
	// Real Restate does this via its journal serialization.
	if s, ok := v.(sample); ok {
		return map[string]any{"name": s.Name, "count": float64(s.Count)}, nil
	}
	return v, nil
}

func (fakeReplayCtx) RunWithRetry(name string, _ RetryPolicy, fn func(ctx context.Context) (any, error)) (any, error) {
	return fakeReplayCtx{}.Run(name, fn)
}

func TestRunAs_directPath(t *testing.T) {
	d := &DirectCtx{}
	got, err := RunAs[sample](d, "step", func(_ context.Context) (sample, error) {
		return sample{Name: "alice", Count: 7}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "alice" || got.Count != 7 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRunAs_replayPath(t *testing.T) {
	got, err := RunAs[sample](fakeReplayCtx{}, "step", func(_ context.Context) (sample, error) {
		return sample{Name: "bob", Count: 9}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "bob" || got.Count != 9 {
		t.Fatalf("RunAs failed to round-trip via JSON: %+v", got)
	}
}

func TestRunAs_propagatesErrors(t *testing.T) {
	d := &DirectCtx{}
	_, err := RunAs[sample](d, "step", func(_ context.Context) (sample, error) {
		return sample{}, errors.New("boom")
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom, got %v", err)
	}
}

func TestScopedDurableCtx_prefixesStepName(t *testing.T) {
	captured := ""
	rec := &recordingCtx{onRun: func(name string) { captured = name }}
	scoped := &ScopedDurableCtx{Inner: rec, Prefix: "iter:0/"}
	_, _ = scoped.Run("foo", func(_ context.Context) (any, error) { return nil, nil })
	if captured != "iter:0/foo" {
		t.Fatalf("expected prefixed step name, got %q", captured)
	}
}

type recordingCtx struct{ onRun func(string) }

func (r *recordingCtx) Run(name string, fn func(ctx context.Context) (any, error)) (any, error) {
	if r.onRun != nil {
		r.onRun(name)
	}
	return fn(context.Background())
}

func (r *recordingCtx) RunWithRetry(name string, _ RetryPolicy, fn func(ctx context.Context) (any, error)) (any, error) {
	return r.Run(name, fn)
}

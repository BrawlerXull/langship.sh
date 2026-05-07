// Package durability defines the journal abstraction the engine uses to
// turn each meaningful step (node execution, sub-workflow call, approval wait)
// into a replayable journal entry.
//
// Two implementations ship: RestateDurableCtx for production durability and
// DirectCtx for tests / library-mode embedding without a Restate process.
package durability

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// RetryPolicy configures per-step retry behavior. In Restate this maps to
// native per-Run retry options. In DirectCtx (tests) it retries in-process.
type RetryPolicy struct {
	MaxAttempts     int
	InitialInterval time.Duration
}

// DurableCtx abstracts over journaled execution.
type DurableCtx interface {
	// Run executes fn as a named journaled step. On replay (after crash/restart),
	// completed steps return cached results without re-execution.
	// The returned value must be JSON-serializable.
	Run(name string, fn func(ctx context.Context) (any, error)) (any, error)

	// RunWithRetry is like Run but applies a per-step retry policy.
	RunWithRetry(name string, policy RetryPolicy, fn func(ctx context.Context) (any, error)) (any, error)
}

type durableCtxKey struct{}

// WithDurableCtx attaches a DurableCtx to a context.Context.
func WithDurableCtx(ctx context.Context, dctx DurableCtx) context.Context {
	return context.WithValue(ctx, durableCtxKey{}, dctx)
}

// FromContext extracts a DurableCtx from the context, if present.
func FromContext(ctx context.Context) (DurableCtx, bool) {
	dctx, ok := ctx.Value(durableCtxKey{}).(DurableCtx)
	return dctx, ok && dctx != nil
}

// --- Restate context threading ---
// Separate from DurableCtx because some executors need the raw Restate context
// for features not available through the DurableCtx abstraction (e.g., Awakeables).

type restateCtxKey struct{}

// WithRestateCtx attaches a raw Restate WorkflowContext to a Go context.
// Used by the ApprovalExecutor to create Awakeables.
func WithRestateCtx(ctx context.Context, rctx any) context.Context {
	return context.WithValue(ctx, restateCtxKey{}, rctx)
}

// RestateCtxFromContext extracts the raw Restate WorkflowContext.
// Returns nil if not running inside Restate.
func RestateCtxFromContext(ctx context.Context) any {
	return ctx.Value(restateCtxKey{})
}

// ScopedDurableCtx wraps a DurableCtx and prefixes all step names.
// Used when a sub-workflow runs inside a parent durable workflow — the prefix
// avoids step name collisions between parent and child.
type ScopedDurableCtx struct {
	Inner  DurableCtx
	Prefix string
}

func (s *ScopedDurableCtx) Run(name string, fn func(ctx context.Context) (any, error)) (any, error) {
	return s.Inner.Run(s.Prefix+name, fn)
}

func (s *ScopedDurableCtx) RunWithRetry(name string, policy RetryPolicy, fn func(ctx context.Context) (any, error)) (any, error) {
	return s.Inner.RunWithRetry(s.Prefix+name, policy, fn)
}

// RunAs executes a durable step and JSON-decodes the result into a concrete type T.
// Restate's `restate.Run` returns map[string]interface{} on replay instead of the
// original struct type; RunAs handles that round-trip transparently.
func RunAs[T any](dctx DurableCtx, name string, fn func(ctx context.Context) (T, error)) (T, error) {
	raw, err := dctx.Run(name, func(ctx context.Context) (any, error) {
		return fn(ctx)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	if typed, ok := raw.(T); ok {
		return typed, nil
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("durability RunAs %q: marshal: %w", name, err)
	}
	var result T
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		var zero T
		return zero, fmt.Errorf("durability RunAs %q: unmarshal into %T: %w", name, result, err)
	}
	return result, nil
}

// RunAsWithRetry is like RunAs but applies a per-step retry policy.
func RunAsWithRetry[T any](dctx DurableCtx, name string, policy RetryPolicy, fn func(ctx context.Context) (T, error)) (T, error) {
	raw, err := dctx.RunWithRetry(name, policy, func(ctx context.Context) (any, error) {
		return fn(ctx)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	if typed, ok := raw.(T); ok {
		return typed, nil
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("durability RunAsWithRetry %q: marshal: %w", name, err)
	}
	var result T
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		var zero T
		return zero, fmt.Errorf("durability RunAsWithRetry %q: unmarshal into %T: %w", name, result, err)
	}
	return result, nil
}

// --- Approval persistence interface ---

// ApprovalCreator persists a HITL approval row. Implemented by the storage layer.
// Defined here to avoid a cycle from executors → storage.
type ApprovalCreator interface {
	CreateFromRecord(ctx context.Context, a *ApprovalRecord) error
}

// ApprovalRecord is the data needed to persist a pending approval.
type ApprovalRecord struct {
	ID          string
	ExecutionID string
	NodeName    string
	AwakeableID string
	Status      string
	InputData   map[string]any
	APIKey      string
}

type approvalCreatorKey struct{}
type executionIDKey struct{}
type apiKeyCtxKey struct{}

// WithApprovalCreator attaches an ApprovalCreator to a context.
func WithApprovalCreator(ctx context.Context, c ApprovalCreator) context.Context {
	return context.WithValue(ctx, approvalCreatorKey{}, c)
}

// ApprovalCreatorFromContext extracts the ApprovalCreator, or nil.
func ApprovalCreatorFromContext(ctx context.Context) ApprovalCreator {
	c, _ := ctx.Value(approvalCreatorKey{}).(ApprovalCreator)
	return c
}

// WithExecutionID attaches the workflow execution ID to a context.
func WithExecutionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionIDKey{}, id)
}

// ExecutionIDFromContext extracts the execution ID, or "".
func ExecutionIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(executionIDKey{}).(string)
	return s
}

// WithAPIKey attaches the API key to a context.
func WithAPIKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, apiKeyCtxKey{}, key)
}

// APIKeyFromContext extracts the API key, or "".
func APIKeyFromContext(ctx context.Context) string {
	s, _ := ctx.Value(apiKeyCtxKey{}).(string)
	return s
}

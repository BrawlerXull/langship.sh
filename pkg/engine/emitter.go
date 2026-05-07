package engine

import "context"

type emitterKey struct{}

// Emitter publishes execution events for a given execution ID.
// Implemented by execevents.MemoryBus (default) and external backends (Postgres LISTEN/NOTIFY, Redis Streams).
type Emitter interface {
	Emit(ctx context.Context, execID string, e ExecutionEvent)
}

// WithEmitter returns a new context carrying the given Emitter.
func WithEmitter(ctx context.Context, e Emitter) context.Context {
	return context.WithValue(ctx, emitterKey{}, e)
}

// EmitterFromContext returns the Emitter stored in ctx, or a no-op emitter.
func EmitterFromContext(ctx context.Context) Emitter {
	if e, ok := ctx.Value(emitterKey{}).(Emitter); ok && e != nil {
		return e
	}
	return noopEmitter{}
}

type noopEmitter struct{}

func (noopEmitter) Emit(_ context.Context, _ string, _ ExecutionEvent) {}

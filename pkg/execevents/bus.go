// Package execevents provides an in-memory pub/sub bus for workflow
// execution events. The engine emits events through engine.Emitter; the API
// server fans them out to SSE subscribers.
//
// This is single-process — fine for the current self-hosted topology.
// Multi-instance deployments will swap this for Postgres LISTEN/NOTIFY or
// Redis Streams (the engine.Emitter interface stays the same).
package execevents

import (
	"context"
	"sync"

	"github.com/lyzrai/flow/pkg/engine"
)

// MemoryBus is a single-process publish/subscribe bus keyed by execution ID.
// Subscribers receive every event published for their execution until they
// unsubscribe or the channel buffer fills (slow subscribers are dropped to
// keep the publisher non-blocking).
type MemoryBus struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscription
	// terminal stores the final event per exec so a subscriber that arrives
	// late still gets a "done" / "error" event and closes cleanly.
	terminal map[string]engine.ExecutionEvent
}

type subscription struct {
	ch     chan engine.ExecutionEvent
	closed bool
	once   sync.Once
}

// NewMemoryBus returns a fresh in-memory bus.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		subscribers: map[string][]*subscription{},
		terminal:    map[string]engine.ExecutionEvent{},
	}
}

// Emit implements engine.Emitter. Non-blocking: if a subscriber's channel is
// full we drop the event for that subscriber (publisher must not stall).
func (b *MemoryBus) Emit(_ context.Context, execID string, e engine.ExecutionEvent) {
	if execID == "" {
		return
	}
	b.mu.Lock()
	subs := append([]*subscription(nil), b.subscribers[execID]...)
	if isTerminal(e.Type) {
		b.terminal[execID] = e
	}
	b.mu.Unlock()

	for _, s := range subs {
		select {
		case s.ch <- e:
		default:
			// drop — slow subscriber
		}
	}
}

// Subscribe returns a channel that receives every event for execID. The
// caller must call the returned cancel func when done. If a terminal event
// was already published before subscribe, it is replayed once so the caller
// can shut down cleanly.
func (b *MemoryBus) Subscribe(execID string) (<-chan engine.ExecutionEvent, func()) {
	s := &subscription{ch: make(chan engine.ExecutionEvent, 32)}

	b.mu.Lock()
	b.subscribers[execID] = append(b.subscribers[execID], s)
	term, hadTerm := b.terminal[execID]
	b.mu.Unlock()

	if hadTerm {
		// non-blocking — buffer is fresh
		s.ch <- term
	}

	cancel := func() {
		s.once.Do(func() {
			b.mu.Lock()
			cur := b.subscribers[execID]
			out := cur[:0]
			for _, x := range cur {
				if x != s {
					out = append(out, x)
				}
			}
			if len(out) == 0 {
				delete(b.subscribers, execID)
			} else {
				b.subscribers[execID] = out
			}
			b.mu.Unlock()
			s.closed = true
			close(s.ch)
		})
	}
	return s.ch, cancel
}

func isTerminal(t engine.EventType) bool {
	return t == engine.EventDone
}

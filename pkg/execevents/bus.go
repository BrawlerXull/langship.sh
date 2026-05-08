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
	"time"

	"github.com/lyzrai/flow/pkg/engine"
)

// retainPerExec is the cap on per-execution event history. We keep the
// most-recent N events so a subscriber that arrives mid-run can replay
// what it missed (node_started lifecycle events especially — those fire
// fast, often before the UI has connected).
const retainPerExec = 500

// retainAfterDone is how long to hold the buffer for an execution after
// the terminal `done` event arrives. Late subscribers (e.g. a user who
// opens the run page right after success) get the full replay.
const retainAfterDone = 5 * time.Minute

// MemoryBus is a single-process publish/subscribe bus keyed by execution ID.
// Subscribers receive every event published for their execution; on
// subscribe they additionally receive a backlog replay of events emitted
// before they connected. Bounded retention per exec keeps memory in check.
type MemoryBus struct {
	mu      sync.RWMutex
	streams map[string]*execStream
}

type execStream struct {
	subs    []*subscription
	history []engine.ExecutionEvent
	done    bool
	doneAt  time.Time
}

type subscription struct {
	ch   chan engine.ExecutionEvent
	once sync.Once
}

// NewMemoryBus returns a fresh in-memory bus and starts a background
// sweeper that drops stale streams. The returned bus has no Close — the
// process owns the lifecycle.
func NewMemoryBus() *MemoryBus {
	b := &MemoryBus{streams: map[string]*execStream{}}
	go b.sweep()
	return b
}

// Emit implements engine.Emitter. Non-blocking: if a subscriber's channel
// is full the event is dropped for that subscriber (publisher must never
// stall) but stays in the per-exec history so a fresh subscriber can still
// see it.
func (b *MemoryBus) Emit(_ context.Context, execID string, e engine.ExecutionEvent) {
	if execID == "" {
		return
	}
	b.mu.Lock()
	st := b.streams[execID]
	if st == nil {
		st = &execStream{}
		b.streams[execID] = st
	}
	// Append + cap.
	st.history = append(st.history, e)
	if over := len(st.history) - retainPerExec; over > 0 {
		st.history = st.history[over:]
	}
	if e.Type == engine.EventDone {
		st.done = true
		st.doneAt = time.Now()
	}
	subs := append([]*subscription(nil), st.subs...)
	b.mu.Unlock()

	for _, s := range subs {
		select {
		case s.ch <- e:
		default:
			// slow subscriber — drop
		}
	}
}

// Subscribe registers for events for execID. On subscribe the caller
// receives every event already retained for this execution (in order),
// followed by every new event. Returns the channel and a cancel func.
func (b *MemoryBus) Subscribe(execID string) (<-chan engine.ExecutionEvent, func()) {
	// Buffer ≥ history cap so the initial replay never drops.
	s := &subscription{ch: make(chan engine.ExecutionEvent, retainPerExec+32)}

	b.mu.Lock()
	st := b.streams[execID]
	if st == nil {
		st = &execStream{}
		b.streams[execID] = st
	}
	st.subs = append(st.subs, s)
	// Snapshot history under the lock.
	backlog := append([]engine.ExecutionEvent(nil), st.history...)
	b.mu.Unlock()

	// Replay outside the lock. Buffer is sized so this never blocks.
	for _, e := range backlog {
		s.ch <- e
	}

	cancel := func() {
		s.once.Do(func() {
			b.mu.Lock()
			st := b.streams[execID]
			if st != nil {
				out := st.subs[:0]
				for _, x := range st.subs {
					if x != s {
						out = append(out, x)
					}
				}
				st.subs = out
			}
			b.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel
}

// sweep periodically GC's streams that are done + past the retention
// window AND have no live subscribers.
func (b *MemoryBus) sweep() {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		b.mu.Lock()
		for id, st := range b.streams {
			if st.done && len(st.subs) == 0 && now.Sub(st.doneAt) > retainAfterDone {
				delete(b.streams, id)
			}
		}
		b.mu.Unlock()
	}
}

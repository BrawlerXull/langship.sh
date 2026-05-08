package api

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/logstore"
)

// logArchiver subscribes to the in-memory event bus for a single execution,
// buffers `node_log` lines per node, and flushes each node's buffer to the
// log store when the node finishes (node_completed / node_error). On the
// terminal `done` event it flushes any leftovers and stops.
//
// One archiver per active execution. They drop themselves from the
// archive map when finished.
type logArchiver struct {
	execID string
	store  logstore.Store
	bus    EventSubscriber

	// in-memory buffer per node — the live SSE listener (the UI) reads from
	// the bus directly; this struct is purely for archive-on-finish.
	mu      sync.Mutex
	buffers map[string]*bytes.Buffer
}

// startLogArchiver spins up a goroutine that drains the per-exec event bus
// into MinIO. Safe to call once per execution; idempotent if the bus is nil.
func startLogArchiver(parentCtx context.Context, store logstore.Store, bus EventSubscriber, execID string) {
	if store == nil || bus == nil || execID == "" {
		return
	}
	ar := &logArchiver{
		execID:  execID,
		store:   store,
		bus:     bus,
		buffers: map[string]*bytes.Buffer{},
	}
	go ar.run(parentCtx)
}

func (a *logArchiver) run(parentCtx context.Context) {
	ch, cancel := a.bus.Subscribe(a.execID)
	defer cancel()

	// Detach from the request context that triggered the run; once submitted
	// we want to keep archiving even if the caller disconnects. Cap with a
	// per-run deadline so a stuck workflow doesn't leak this goroutine
	// forever.
	ctx, cancelCtx := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancelCtx()

	for {
		select {
		case <-ctx.Done():
			a.flushAll(ctx)
			return
		case <-parentCtx.Done():
			// Process is shutting down.
			a.flushAll(context.Background())
			return
		case ev, ok := <-ch:
			if !ok {
				a.flushAll(ctx)
				return
			}
			a.handle(ctx, ev)
			if ev.Type == engine.EventDone {
				a.flushAll(ctx)
				return
			}
		}
	}
}

func (a *logArchiver) handle(ctx context.Context, ev engine.ExecutionEvent) {
	switch ev.Type {
	case engine.EventNodeLog:
		if ev.Node == "" || ev.Content == "" {
			return
		}
		a.mu.Lock()
		buf := a.buffers[ev.Node]
		if buf == nil {
			buf = &bytes.Buffer{}
			a.buffers[ev.Node] = buf
		}
		// One line per Log event; force a trailing newline so the archived
		// file is line-oriented and easy to tail.
		buf.WriteString(strings.TrimRight(ev.Content, "\r\n"))
		buf.WriteByte('\n')
		a.mu.Unlock()

	case engine.EventNodeCompleted, engine.EventNodeError:
		if ev.Node == "" {
			return
		}
		a.flushNode(ctx, ev.Node)
	}
}

func (a *logArchiver) flushNode(ctx context.Context, node string) {
	a.mu.Lock()
	buf := a.buffers[node]
	if buf == nil || buf.Len() == 0 {
		a.mu.Unlock()
		return
	}
	data := append([]byte(nil), buf.Bytes()...)
	delete(a.buffers, node)
	a.mu.Unlock()

	putCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := a.store.Put(putCtx, a.execID, node, data); err != nil {
		slog.WarnContext(ctx, "log_archive_failed",
			slog.String("execution_id", a.execID),
			slog.String("node", node),
			slog.Any("error", err),
		)
	}
}

func (a *logArchiver) flushAll(ctx context.Context) {
	a.mu.Lock()
	nodes := make([]string, 0, len(a.buffers))
	for n := range a.buffers {
		nodes = append(nodes, n)
	}
	a.mu.Unlock()
	for _, n := range nodes {
		a.flushNode(ctx, n)
	}
}

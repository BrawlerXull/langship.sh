package api

import (
	"encoding/json"
	"sync"
	"time"
)

// RunCreatedEvent is the payload broadcast on /api/runs/stream whenever a
// new run is dispatched (manual API call or GitHub webhook).
type RunCreatedEvent struct {
	Type         string    `json:"type"`         // "run_created"
	ExecutionID  string    `json:"executionId"`
	PipelineID   string    `json:"pipelineId,omitempty"`
	PipelineName string    `json:"pipelineName,omitempty"`
	AgentID      string    `json:"agentId,omitempty"`
	Source       string    `json:"source,omitempty"` // "manual" | "github_push"
	StartedAt    time.Time `json:"startedAt"`
}

// runsBus is a tiny broadcast bus for cross-execution UI events. The SSE
// stream handler subscribes; dispatchers publish. All in-memory; one bus
// per process.
type runsBus struct {
	mu          sync.RWMutex
	subscribers map[chan RunCreatedEvent]struct{}
}

func newRunsBus() *runsBus {
	return &runsBus{subscribers: map[chan RunCreatedEvent]struct{}{}}
}

// Publish fans an event out to every active subscriber. Non-blocking — slow
// subscribers drop the event so dispatchers never stall.
func (b *runsBus) Publish(ev RunCreatedEvent) {
	b.mu.RLock()
	subs := make([]chan RunCreatedEvent, 0, len(b.subscribers))
	for c := range b.subscribers {
		subs = append(subs, c)
	}
	b.mu.RUnlock()
	for _, c := range subs {
		select {
		case c <- ev:
		default:
		}
	}
}

// Subscribe registers a buffered channel and returns it + an unsubscribe
// func that closes the channel exactly once.
func (b *runsBus) Subscribe() (<-chan RunCreatedEvent, func()) {
	ch := make(chan RunCreatedEvent, 16)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// marshal is a small helper so handlers don't import encoding/json just for
// the event payload format.
func (e RunCreatedEvent) marshal() ([]byte, error) { return json.Marshal(e) }

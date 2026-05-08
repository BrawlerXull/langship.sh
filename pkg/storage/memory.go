package storage

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// Memory is an in-memory implementation of all stores. Useful for tests and
// for short-lived processes where Mongo isn't available. Not safe across
// process restarts. Pass via storage.NewMemory().
type Memory struct {
	mu        sync.Mutex
	pipelines map[string]Pipeline
	runs      map[string]Run
	agents    map[string]Agent
}

// NewMemory returns a fresh Memory with empty collections.
func NewMemory() *Memory {
	return &Memory{
		pipelines: map[string]Pipeline{},
		runs:      map[string]Run{},
		agents:    map[string]Agent{},
	}
}

// --- pipelines ------------------------------------------------------------

type memoryPipelines struct{ m *Memory }

func (s *memoryPipelines) Create(_ context.Context, p *Pipeline) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.pipelines[p.ID] = *p
	return nil
}

func (s *memoryPipelines) Get(_ context.Context, id string) (*Pipeline, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	p, ok := s.m.pipelines[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (s *memoryPipelines) Update(_ context.Context, p *Pipeline) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	if _, ok := s.m.pipelines[p.ID]; !ok {
		return ErrNotFound
	}
	s.m.pipelines[p.ID] = *p
	return nil
}

func (s *memoryPipelines) Delete(_ context.Context, id string) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	if _, ok := s.m.pipelines[id]; !ok {
		return ErrNotFound
	}
	delete(s.m.pipelines, id)
	return nil
}

func (s *memoryPipelines) List(_ context.Context) ([]*Pipeline, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	out := make([]*Pipeline, 0, len(s.m.pipelines))
	for _, p := range s.m.pipelines {
		p := p
		out = append(out, &p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Pipelines returns the in-memory PipelineStore.
func (m *Memory) Pipelines() PipelineStore { return &memoryPipelines{m: m} }

// --- runs ----------------------------------------------------------------

type memoryRuns struct{ m *Memory }

func (s *memoryRuns) Insert(_ context.Context, r *Run) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.runs[r.ID] = *r
	return nil
}

func (s *memoryRuns) UpdateStatus(_ context.Context, id, status string) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	r, ok := s.m.runs[id]
	if !ok {
		return ErrNotFound
	}
	r.Status = status
	s.m.runs[id] = r
	return nil
}

func (s *memoryRuns) Complete(_ context.Context, id, status string, outputs, nodeOutputs json.RawMessage, errMsg string) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	r, ok := s.m.runs[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	r.Status = status
	r.FinishedAt = &now
	r.Outputs = outputs
	r.NodeOutputs = nodeOutputs
	if errMsg != "" {
		r.Errors = []string{errMsg}
	}
	s.m.runs[id] = r
	return nil
}

func (s *memoryRuns) Get(_ context.Context, id string) (*Run, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	r, ok := s.m.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &r, nil
}

func (s *memoryRuns) ListByPipeline(_ context.Context, pipelineID string, limit int) ([]*Run, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	var out []*Run
	for _, r := range s.m.runs {
		if r.PipelineID == pipelineID {
			r := r
			out = append(out, &r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *memoryRuns) List(_ context.Context, limit int) ([]*Run, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	out := make([]*Run, 0, len(s.m.runs))
	for _, r := range s.m.runs {
		r := r
		out = append(out, &r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Runs returns the in-memory RunStore.
func (m *Memory) Runs() RunStore { return &memoryRuns{m: m} }

// --- agents --------------------------------------------------------------

type memoryAgents struct{ m *Memory }

func (s *memoryAgents) Create(_ context.Context, a *Agent) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.agents[a.ID] = *a
	return nil
}

func (s *memoryAgents) Update(_ context.Context, a *Agent) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	if _, ok := s.m.agents[a.ID]; !ok {
		return ErrNotFound
	}
	s.m.agents[a.ID] = *a
	return nil
}

func (s *memoryAgents) Get(_ context.Context, id string) (*Agent, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	a, ok := s.m.agents[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &a, nil
}

func (s *memoryAgents) Delete(_ context.Context, id string) error {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	if _, ok := s.m.agents[id]; !ok {
		return ErrNotFound
	}
	delete(s.m.agents, id)
	return nil
}

func (s *memoryAgents) List(_ context.Context) ([]*Agent, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	out := make([]*Agent, 0, len(s.m.agents))
	for _, a := range s.m.agents {
		a := a
		out = append(out, &a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Agents returns the in-memory AgentStore.
func (m *Memory) Agents() AgentStore { return &memoryAgents{m: m} }

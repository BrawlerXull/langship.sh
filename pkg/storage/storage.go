// Package storage provides persistent stores for pipelines and execution
// history. The default backend is MongoDB; the interfaces stay narrow so an
// alternative backend (SQLite, Postgres) can be slotted in later.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when a queried document does not exist. API
// handlers should map this to HTTP 404.
var ErrNotFound = errors.New("not found")

// Pipeline is the persisted shape of a pipeline (formerly "flow") that the
// API stores and returns to the UI. Definition is the n8n-format JSON.
type Pipeline struct {
	ID         string          `json:"id"          bson:"_id"`
	Name       string          `json:"name"        bson:"name"`
	Definition json.RawMessage `json:"definition"  bson:"definition"`
	NodeCount  int             `json:"nodeCount"   bson:"node_count"`
	Status     string          `json:"status"      bson:"status,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"   bson:"created_at"`
	UpdatedAt  time.Time       `json:"updatedAt"   bson:"updated_at"`
}

// PipelineStore persists pipeline definitions.
type PipelineStore interface {
	Create(ctx context.Context, p *Pipeline) error
	Get(ctx context.Context, id string) (*Pipeline, error)
	Update(ctx context.Context, p *Pipeline) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Pipeline, error)
}

// Run is a single execution attempt against a pipeline. Captured at submit
// time; status/outputs are filled in later when the orchestrator finishes.
type Run struct {
	ID          string          `json:"id"                 bson:"_id"`           // execution_id
	PipelineID  string          `json:"pipelineId"         bson:"pipeline_id"`
	PipelineName string         `json:"pipelineName"       bson:"pipeline_name"`
	Status      string          `json:"status"             bson:"status"`
	StartedAt   time.Time       `json:"startedAt"          bson:"started_at"`
	FinishedAt  *time.Time      `json:"finishedAt,omitempty" bson:"finished_at,omitempty"`
	TriggerData json.RawMessage `json:"triggerData,omitempty" bson:"trigger_data,omitempty"`
	Outputs     json.RawMessage `json:"outputs,omitempty"     bson:"outputs,omitempty"`
	NodeOutputs json.RawMessage `json:"nodeOutputs,omitempty" bson:"node_outputs,omitempty"`
	Errors      []string        `json:"errors,omitempty"      bson:"errors,omitempty"`
}

// RunStore persists execution history.
type RunStore interface {
	Insert(ctx context.Context, r *Run) error
	UpdateStatus(ctx context.Context, id, status string) error
	Complete(ctx context.Context, id, status string, outputs, nodeOutputs json.RawMessage, errMsg string) error
	Get(ctx context.Context, id string) (*Run, error)
	ListByPipeline(ctx context.Context, pipelineID string, limit int) ([]*Run, error)
	List(ctx context.Context, limit int) ([]*Run, error)
}

// AuthStatus reflects the result of the most recent PAT/repo auth probe.
type AuthStatus string

const (
	AuthUntested AuthStatus = "untested"
	AuthOK       AuthStatus = "ok"
	AuthFailed   AuthStatus = "failed"
)

// Agent is an agent repo registered with Langship. The PAT and webhook
// secret are stored server-side; the API layer scrubs them before the
// record leaves the boundary (see pkg/api/agents.go).
type Agent struct {
	ID                 string     `json:"id"                bson:"_id"`
	Name               string     `json:"name"              bson:"name"`
	RepoURL            string     `json:"repoUrl"           bson:"repo_url"`
	Ref                string     `json:"ref,omitempty"     bson:"ref,omitempty"`
	PAT                string     `json:"-"                 bson:"pat,omitempty"`
	WebhookID          int64      `json:"webhookId,omitempty"        bson:"webhook_id,omitempty"`
	WebhookSecret      string     `json:"-"                          bson:"webhook_secret,omitempty"`
	WebhookInstalledAt *time.Time `json:"webhookInstalledAt,omitempty" bson:"webhook_installed_at,omitempty"`
	AuthStatus         AuthStatus `json:"authStatus,omitempty"        bson:"auth_status,omitempty"`
	AuthCheckedAt      *time.Time `json:"authCheckedAt,omitempty"     bson:"auth_checked_at,omitempty"`
	AttachedPipelines  []string   `json:"attachedPipelines,omitempty" bson:"attached_pipelines,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"         bson:"created_at"`
	UpdatedAt          time.Time  `json:"updatedAt"         bson:"updated_at"`
}

// AgentStore persists agent registrations. Update mutates the entire
// record; callers do read-modify-write under their own consistency model.
type AgentStore interface {
	Create(ctx context.Context, a *Agent) error
	Get(ctx context.Context, id string) (*Agent, error)
	Update(ctx context.Context, a *Agent) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Agent, error)
}

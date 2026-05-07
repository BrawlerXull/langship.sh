// Package orchestrator coordinates durable workflow execution.
//
// The default implementation is RestateOrchestrator (talks to Restate Server
// over its HTTP ingress). The interface stays pluggable so an in-memory or
// Temporal-backed implementation can be slotted in for tests / alternative
// deployments.
package orchestrator

import (
	"context"
	"encoding/json"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// RequestMeta carries identity fields that survive serialization to Restate
// and back. Embedded in every request struct.
type RequestMeta struct {
	APIKey     string `json:"api_key,omitempty"`
	OrgID      string `json:"org_id,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
}

// RunRequest contains everything needed to execute a workflow.
type RunRequest struct {
	RequestMeta
	Workflow    *models.WorkflowDefinition
	TriggerData []models.Item
	Lookup      engine.ExecutorLookup
}

// PendingApproval describes a workflow paused on a HITL Approval node.
type PendingApproval struct {
	Node        string         `json:"node"`
	AwakeableID string         `json:"awakeable_id"`
	Context     map[string]any `json:"context,omitempty"`
}

// ExecutionStatus represents the current state of a workflow execution.
type ExecutionStatus struct {
	ExecutionID     string                           `json:"execution_id"`
	Status          string                           `json:"status"`
	Outputs         map[string]map[int][]models.Item `json:"outputs,omitempty"`
	NodeOutputs     map[string]map[int][]models.Item `json:"node_outputs,omitempty"`
	Errors          []string                         `json:"errors,omitempty"`
	PendingApproval *PendingApproval                 `json:"pending_approval,omitempty"`
}

// Orchestrator coordinates workflow execution. Implementations control the
// durability and scheduling model (in-memory, Restate, Temporal, etc.).
type Orchestrator interface {
	// Run executes a workflow and blocks until completion.
	Run(ctx context.Context, req *RunRequest) (executionID string, result *models.ExecutionResult, err error)

	// RunAsync starts a workflow execution and returns immediately with an
	// execution ID. Status is observed via GetExecution / event stream.
	RunAsync(ctx context.Context, req *RunRequest) (executionID string, err error)

	// GetExecution retrieves the status and result of a workflow execution.
	GetExecution(ctx context.Context, executionID string) (*ExecutionStatus, error)
}

// NodeRunner is called for each node during DAG execution. The orchestrator
// controls HOW the node runs (directly, via restate.Run, etc.).
// stepID is the node name, used as a durable step identifier.
type NodeRunner func(ctx context.Context, stepID string, executeFn func(ctx context.Context) (map[int][]models.Item, error)) (map[int][]models.Item, error)

// ExecutionCompleter persists terminal execution state to the database.
// Optional — if nil, terminal state is not persisted.
type ExecutionCompleter interface {
	Complete(ctx context.Context, id, status string, outputs, nodeOutputs json.RawMessage, errMsg string) error
}

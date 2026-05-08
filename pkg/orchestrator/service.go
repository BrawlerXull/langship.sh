package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	restate "github.com/restatedev/sdk-go"
	"github.com/restatedev/sdk-go/server"

	"github.com/lyzrai/flow/pkg/durability"
	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// WorkflowRequest is the JSON-serializable payload Restate persists for replay.
type WorkflowRequest struct {
	RequestMeta
	Workflow    *models.WorkflowDefinition `json:"workflow"`
	TriggerData []models.Item              `json:"trigger_data"`
}

// nodeStepResult round-trips node outputs through the Restate journal as JSON.
// We use string-keyed outputs because JSON can't natively encode int keys.
type nodeStepResult struct {
	Outputs map[string][]models.Item `json:"outputs"`
}

func toStepResult(outputs map[int][]models.Item) nodeStepResult {
	m := make(map[string][]models.Item, len(outputs))
	for idx, items := range outputs {
		m[strconv.Itoa(idx)] = items
	}
	return nodeStepResult{Outputs: m}
}

func fromStepResult(result nodeStepResult) map[int][]models.Item {
	m := make(map[int][]models.Item, len(result.Outputs))
	for key, items := range result.Outputs {
		idx, _ := strconv.Atoi(key)
		m[idx] = items
	}
	return m
}

// WorkflowService is the Restate workflow handler that provides durable execution.
// Each node execution is wrapped in restate.Run() for automatic journaling.
type WorkflowService struct {
	lookup          engine.ExecutorLookup
	approvalCreator durability.ApprovalCreator
	execStore       ExecutionCompleter
	emitter         engine.Emitter
}

// NewWorkflowService constructs the Restate-side handler. approvalCreator,
// execStore, and emitter are optional — pass nil if not yet wired.
func NewWorkflowService(lookup engine.ExecutorLookup, approvalCreator durability.ApprovalCreator, execStore ExecutionCompleter, emitter engine.Emitter) *WorkflowService {
	return &WorkflowService{lookup: lookup, approvalCreator: approvalCreator, execStore: execStore, emitter: emitter}
}

// ServiceName is required by the Restate SDK. It's the service identifier
// used in ingress URLs and admin registration.
func (w *WorkflowService) ServiceName() string { return "WorkflowExecutor" }

// Run is the Restate workflow handler. It walks the DAG and wraps each node
// execution in restate.Run() for durability — on crash/restart, completed steps
// replay from the journal without re-execution.
func (w *WorkflowService) Run(ctx restate.WorkflowContext, req WorkflowRequest) (*models.ExecutionResult, error) {
	dctx := &durability.RestateDurableCtx{Rctx: ctx}
	enrichedCtx := durability.WithDurableCtx(ctx, dctx)
	enrichedCtx = durability.WithRestateCtx(enrichedCtx, ctx)
	enrichedCtx = durability.WithExecutionID(enrichedCtx, restate.Key(ctx))
	enrichedCtx = durability.WithAPIKey(enrichedCtx, req.APIKey)
	if w.approvalCreator != nil {
		enrichedCtx = durability.WithApprovalCreator(enrichedCtx, w.approvalCreator)
	}
	if w.emitter != nil {
		enrichedCtx = engine.WithEmitter(enrichedCtx, w.emitter)
	}

	result, err := walkDurable(enrichedCtx, req.Workflow, req.TriggerData, w.lookup, restateNodeRunner(ctx))
	if result == nil {
		result = &models.ExecutionResult{Status: "failed"}
	}
	result.ExecutionID = restate.Key(ctx)

	// Persist terminal state if a store is wired so list queries reflect
	// completion without polling the per-execution endpoint.
	if w.execStore != nil {
		w.persistTerminal(enrichedCtx, result)
	}

	if err != nil && durability.IsTerminalError(err) {
		return result, restate.TerminalError(err)
	}
	return result, err
}

// GetPendingApproval is a shared (non-blocking) handler that returns the
// pending approval state set by the Approval node. Called by the HTTP API
// to surface HITL state.
func (w *WorkflowService) GetPendingApproval(ctx restate.WorkflowSharedContext, _ restate.Void) (*PendingApproval, error) {
	node, err := restate.Get[string](ctx, "pending_approval_node")
	if err != nil || node == "" {
		return nil, nil
	}
	awakeableID, _ := restate.Get[string](ctx, "pending_approval_id")
	approvalCtx, _ := restate.Get[map[string]any](ctx, "pending_approval_context")
	return &PendingApproval{Node: node, AwakeableID: awakeableID, Context: approvalCtx}, nil
}

// restateNodeRunner returns a NodeRunner that wraps each node execution in
// restate.Run() for durable journaling.
func restateNodeRunner(rctx restate.WorkflowContext) NodeRunner {
	return func(ctx context.Context, stepID string, executeFn func(ctx context.Context) (map[int][]models.Item, error)) (map[int][]models.Item, error) {
		result, err := restate.Run(rctx, func(runCtx restate.RunContext) (nodeStepResult, error) {
			outputs, err := executeFn(ctx)
			if err != nil {
				if durability.IsTerminalError(err) {
					return nodeStepResult{}, restate.TerminalError(err)
				}
				return nodeStepResult{}, err
			}
			return toStepResult(outputs), nil
		}, restate.WithName(stepID))
		if err != nil {
			return nil, fmt.Errorf("restate step %q failed: %w", stepID, err)
		}
		return fromStepResult(result), nil
	}
}

func (w *WorkflowService) persistTerminal(ctx context.Context, result *models.ExecutionResult) {
	var outputsJSON, nodeOutputsJSON json.RawMessage
	if result.Outputs != nil {
		outputsJSON, _ = json.Marshal(result.Outputs)
	}
	if result.NodeOutputs != nil {
		nodeOutputsJSON, _ = json.Marshal(result.NodeOutputs)
	}
	errMsg := ""
	if len(result.Errors) > 0 {
		errMsg = result.Errors[0]
	}
	if cErr := w.execStore.Complete(ctx, result.ExecutionID, result.Status, outputsJSON, nodeOutputsJSON, errMsg); cErr != nil {
		slog.WarnContext(ctx, "failed to persist workflow completion",
			slog.String("execution_id", result.ExecutionID),
			slog.Any("error", cErr),
		)
	}
}

// defaultRetryPolicy caps Restate's default infinite retries to a sensible
// limit so a persistently failing step doesn't loop forever.
var defaultRetryPolicy = restate.WithInvocationRetryPolicy(
	restate.WithMaxAttempts(10),
	restate.KillOnMaxAttempts(),
)

// NewRestateServer constructs a Restate server endpoint with the WorkflowExecutor
// handler bound. extraServices lets callers register additional Restate handlers
// (for governance, audit, etc.) without modifying this package.
func NewRestateServer(lookup engine.ExecutorLookup, approvalCreator durability.ApprovalCreator, execStore ExecutionCompleter, emitter engine.Emitter, extraServices ...any) *server.Restate {
	wfSvc := NewWorkflowService(lookup, approvalCreator, execStore, emitter)

	rs := server.NewRestate().
		Bind(restate.Reflect(wfSvc, defaultRetryPolicy))

	for _, svc := range extraServices {
		rs = rs.Bind(restate.Reflect(svc, defaultRetryPolicy))
	}
	return rs
}

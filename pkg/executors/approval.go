package executors

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	restate "github.com/restatedev/sdk-go"

	"github.com/lyzrai/flow/pkg/durability"
	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// ApprovalExecutor implements a human-in-the-loop approval node.
// It pauses the workflow using a Restate Awakeable and blocks until an external
// caller resolves it via POST /api/executions/{id}/resume.
//
// Two outputs:
//   - Output 0: approved — items flow with human-supplied data merged in
//   - Output 1: rejected — items flow with rejection_reason field
//
// The caller resolves with {"approved": true, ...data} or
// {"approved": false, "reason": "..."}.
type ApprovalExecutor struct{}

func (e *ApprovalExecutor) Execute(
	ctx context.Context,
	node models.NodeDef,
	inputs [][]models.Item,
	execCtx *engine.ExecutionContext,
) (map[int][]models.Item, error) {
	var inputItems []models.Item
	for _, input := range inputs {
		inputItems = append(inputItems, input...)
	}
	if len(inputItems) == 0 {
		inputItems = []models.Item{{}}
	}

	// Approval requires Restate for durable blocking.
	raw := durability.RestateCtxFromContext(ctx)
	rctx, ok := raw.(restate.WorkflowContext)
	if !ok {
		return nil, &durability.PermanentError{Err: fmt.Errorf(
			"approval node %q requires Restate for durable execution", node.Name,
		)}
	}

	// Create the Awakeable — the workflow sleeps here until it's resolved.
	awakeable := restate.Awakeable[map[string]any](rctx)
	awakeableID := awakeable.Id()

	// Surface pending state so GET /api/executions/:id can return it.
	restate.Set(rctx, "pending_approval_node", node.Name)
	restate.Set(rctx, "pending_approval_id", awakeableID)

	// Approval context for the reviewer UI: resolved message + reason +
	// input data.
	approvalCtx := map[string]any{}
	resolved := node.Parameters
	if execCtx != nil {
		resolved = engine.ResolveExpressions(node.Parameters, execCtx, node.Name)
	}
	if msg, ok := resolved["message"].(string); ok && msg != "" {
		approvalCtx["message"] = msg
	}
	if reason, ok := resolved["reason"].(string); ok && reason != "" {
		approvalCtx["reason"] = reason
	}
	if len(inputItems) == 1 {
		approvalCtx["inputs"] = map[string]any(inputItems[0])
	} else if len(inputItems) > 1 {
		items := make([]map[string]any, len(inputItems))
		for i, item := range inputItems {
			items[i] = map[string]any(item)
		}
		approvalCtx["inputs"] = items
	}
	if len(approvalCtx) > 0 {
		restate.Set(rctx, "pending_approval_context", approvalCtx)
	}

	// Optionally persist to a side store so other channels (Slack, email) can
	// resolve the approval too. No-op when no creator is wired in this build.
	if creator := durability.ApprovalCreatorFromContext(ctx); creator != nil {
		inputMap := make(map[string]any, len(inputItems))
		for i, item := range inputItems {
			inputMap[fmt.Sprintf("item_%d", i)] = map[string]any(item)
		}
		record := &durability.ApprovalRecord{
			ID:          uuid.New().String(),
			ExecutionID: durability.ExecutionIDFromContext(ctx),
			NodeName:    node.Name,
			AwakeableID: awakeableID,
			Status:      "pending",
			InputData:   inputMap,
			APIKey:      durability.APIKeyFromContext(ctx),
		}
		if err := creator.CreateFromRecord(ctx, record); err != nil {
			slog.WarnContext(ctx, "failed to persist approval, continuing",
				slog.String("node", node.Name),
				slog.String("error", err.Error()),
			)
		}
	}

	slog.InfoContext(ctx, "workflow_paused_for_approval",
		slog.String("node", node.Name),
		slog.String("awakeable_id", awakeableID),
	)

	// Block on the Awakeable. Durable across crashes / restarts.
	approvalData, err := awakeable.Result()
	if err != nil {
		return nil, fmt.Errorf("approval node %q: %w", node.Name, err)
	}

	// Clear pending markers now that we've resumed.
	restate.Clear(rctx, "pending_approval_node")
	restate.Clear(rctx, "pending_approval_id")
	restate.Clear(rctx, "pending_approval_context")

	slog.InfoContext(ctx, "workflow_resumed",
		slog.String("node", node.Name),
	)

	// Route based on the approved field.
	approved, _ := approvalData["approved"].(bool)

	if approved {
		var out []models.Item
		for _, item := range inputItems {
			merged := copyItem(item)
			for k, v := range approvalData {
				merged[k] = v
			}
			out = append(out, merged)
		}
		return map[int][]models.Item{0: out}, nil
	}

	reason, _ := approvalData["reason"].(string)
	var rejected []models.Item
	for _, item := range inputItems {
		r := copyItem(item)
		r["rejection_reason"] = reason
		r["approved"] = false
		rejected = append(rejected, r)
	}
	return map[int][]models.Item{1: rejected}, nil
}

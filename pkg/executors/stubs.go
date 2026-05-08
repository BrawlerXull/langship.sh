package executors

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// stubExecutor implements a no-op executor that:
//   - logs the node's parameters under a distinct slog message
//   - sleeps `sleep` to simulate work (interruptible by ctx cancel)
//   - decorates each input item with `{stub: <stage>, status: success}` so
//     downstream nodes have something to read.
//
// This is a deliberate placeholder for nodes whose real backing is still
// pending (Test, Eval, Policy, Deploy, Promote, Rollback). Swap individual
// stubs for real executors as they're built; the runtime contract stays
// the same.
type stubExecutor struct {
	stage   string        // "test" | "eval" | "policy" | ...
	logName string        // slog message name
	sleep   time.Duration // simulated work
}

func (e *stubExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	slog.InfoContext(ctx, e.logName,
		slog.String("node", node.Name),
		slog.Any("parameters", node.Parameters),
	)

	logger := engine.NodeLoggerFromContext(ctx)
	logger.Log("[stub:" + e.stage + "] starting " + node.Name)
	if len(node.Parameters) > 0 {
		for k, v := range node.Parameters {
			logger.Log("  param " + k + " = " + sprintAny(v))
		}
	}

	if e.sleep > 0 {
		t := time.NewTimer(e.sleep)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.C:
		}
	}

	var inputItems []models.Item
	for _, in := range inputs {
		inputItems = append(inputItems, in...)
	}
	if len(inputItems) == 0 {
		// Trigger nodes typically pass {} as the single seed item; preserve
		// that shape so a stub at the head of the pipeline still produces
		// something downstream can consume.
		inputItems = []models.Item{{}}
	}

	out := make([]models.Item, 0, len(inputItems))
	for _, item := range inputItems {
		ci := copyItem(item)
		ci["__stub"] = e.stage
		ci["__status"] = "success"
		ci["__node"] = node.Name
		out = append(out, ci)
	}
	logger.Log("[stub:" + e.stage + "] finished " + node.Name + " (" + strconvItoa(len(out)) + " items)")
	return map[int][]models.Item{0: out}, nil
}

// strconvItoa wraps strconv.Itoa so we don't pull strconv into stubs.go just for one call.
func strconvItoa(n int) string { return fmt.Sprintf("%d", n) }

// sprintAny formats a parameter value for log lines, capping length so a
// huge JSON blob doesn't blow up the log channel.
func sprintAny(v any) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// TestExecutor stubs the Test stage.
type TestExecutor struct{}

func (e *TestExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, ec *engine.ExecutionContext) (map[int][]models.Item, error) {
	return (&stubExecutor{stage: "test", logName: "stub_test", sleep: 500 * time.Millisecond}).Execute(ctx, node, inputs, ec)
}

// EvalExecutor stubs the Eval stage.
type EvalExecutor struct{}

func (e *EvalExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, ec *engine.ExecutionContext) (map[int][]models.Item, error) {
	return (&stubExecutor{stage: "eval", logName: "stub_eval", sleep: 750 * time.Millisecond}).Execute(ctx, node, inputs, ec)
}

// PolicyExecutor stubs the Policy stage.
type PolicyExecutor struct{}

func (e *PolicyExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, ec *engine.ExecutionContext) (map[int][]models.Item, error) {
	return (&stubExecutor{stage: "policy", logName: "stub_policy", sleep: 200 * time.Millisecond}).Execute(ctx, node, inputs, ec)
}

// DeployExecutor stubs the Deploy stage.
type DeployExecutor struct{}

func (e *DeployExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, ec *engine.ExecutionContext) (map[int][]models.Item, error) {
	return (&stubExecutor{stage: "deploy", logName: "stub_deploy", sleep: 1 * time.Second}).Execute(ctx, node, inputs, ec)
}

// PromoteExecutor stubs the Promote stage.
type PromoteExecutor struct{}

func (e *PromoteExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, ec *engine.ExecutionContext) (map[int][]models.Item, error) {
	return (&stubExecutor{stage: "promote", logName: "stub_promote", sleep: 500 * time.Millisecond}).Execute(ctx, node, inputs, ec)
}

// RollbackExecutor stubs the Rollback stage.
type RollbackExecutor struct{}

func (e *RollbackExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, ec *engine.ExecutionContext) (map[int][]models.Item, error) {
	return (&stubExecutor{stage: "rollback", logName: "stub_rollback", sleep: 500 * time.Millisecond}).Execute(ctx, node, inputs, ec)
}

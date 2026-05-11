package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lyzrai/flow/pkg/durability"
	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// walkDurable executes a workflow's DAG with each node wrapped in a NodeRunner
// (typically restate.Run for journaling). Sequential topological order — the
// reactive ready-queue with futures is a v0.2 thing.
func walkDurable(
	ctx context.Context,
	wf *models.WorkflowDefinition,
	triggerData []models.Item,
	lookup engine.ExecutorLookup,
	runNode NodeRunner,
) (*models.ExecutionResult, error) {
	dag, err := engine.BuildDAG(wf)
	if err != nil {
		wrapped := fmt.Errorf("failed to build DAG: %w", err)
		return &models.ExecutionResult{Status: "failed", Errors: []string{wrapped.Error()}}, wrapped
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		wrapped := fmt.Errorf("failed to sort DAG: %w", err)
		return &models.ExecutionResult{Status: "failed", Errors: []string{wrapped.Error()}}, wrapped
	}

	slog.InfoContext(ctx, "workflow_started_durable",
		slog.String("workflow", wf.Name),
		slog.Int("node_count", len(order)),
	)

	execCtx := engine.NewExecutionContext(wf, dag)
	execCtx.Lookup = lookup

	emitter := engine.EmitterFromContext(ctx)
	execID := durability.ExecutionIDFromContext(ctx)

	emit := func(e engine.ExecutionEvent) {
		emitter.Emit(ctx, execID, e)
	}

	var execErrors []string

	for _, nodeName := range order {
		node := dag.Nodes[nodeName]

		var inputs [][]models.Item
		if node.Type == "flow-nodes-base.trigger" && triggerData != nil {
			inputs = [][]models.Item{triggerData}
		} else {
			inputs = execCtx.GatherInputs(nodeName)
		}

		// Skip orphan non-trigger nodes (no path from a trigger).
		if node.Type != "flow-nodes-base.trigger" && len(dag.InEdges[nodeName]) == 0 {
			continue
		}

		// Skip nodes whose inputs were routed elsewhere (Switch/If false branch).
		if len(dag.InEdges[nodeName]) > 0 && allInputsEmpty(inputs) {
			continue
		}

		resolvedParams := engine.ResolveExpressions(node.Parameters, execCtx, nodeName)
		node.Parameters = resolvedParams

		executorFn, err := lookup(node.Type)
		if err != nil {
			slog.WarnContext(ctx, "node_skipped_unknown_type",
				slog.String("node", nodeName),
				slog.String("type", node.Type),
			)
			execErrors = append(execErrors, fmt.Sprintf("node %q: %v", nodeName, err))
			continue
		}

		// Capture loop-locals for the closure.
		nm, nd, in, ex := nodeName, node, inputs, executorFn

		emit(engine.ExecutionEvent{
			Type:     engine.EventNodeStarted,
			Node:     nm,
			NodeType: nd.Type,
			Status:   "running",
		})

		// Per-node logger that publishes EventNodeLog events on the same
		// emitter the SSE bus subscribes to.
		nodeLogger := &emitNodeLogger{
			emitter: emitter,
			ctx:     ctx,
			execID:  execID,
			node:    nm,
			ntype:   nd.Type,
		}

		started := time.Now()
		// Approval is special: its executor calls Restate context methods
		// (restate.Set / Clear / Awakeable) directly. Wrapping it in
		// restate.Run would make those calls happen inside a Run block,
		// which the SDK rejects with "Concurrent context use detected".
		// So Approval runs against the parent workflow context, no Run.
		var outputs map[int][]models.Item
		var runErr error
		if nd.Type == "flow-nodes-base.waitForApproval" {
			runCtx := engine.WithNodeLogger(ctx, nodeLogger)
			outputs, runErr = ex(runCtx, nd, in, execCtx)
		} else {
			outputs, runErr = runNode(ctx, "node:"+nm, func(c context.Context) (map[int][]models.Item, error) {
				c = engine.WithNodeLogger(c, nodeLogger)
				return ex(c, nd, in, execCtx)
			})
		}
		dur := time.Since(started).Milliseconds()

		if runErr != nil {
			emit(engine.ExecutionEvent{
				Type:       engine.EventNodeError,
				Node:       nm,
				NodeType:   nd.Type,
				Status:     "failed",
				Error:      runErr.Error(),
				DurationMs: dur,
			})
			execErrors = append(execErrors, fmt.Sprintf("node %q: %v", nm, runErr))
			continue
		}
		for outIdx, items := range outputs {
			execCtx.SetOutput(nm, outIdx, items)
		}

		emit(engine.ExecutionEvent{
			Type:       engine.EventNodeCompleted,
			Node:       nm,
			NodeType:   nd.Type,
			Status:     "success",
			Outputs:    nodeOutputsPreview(outputs),
			DurationMs: dur,
		})
	}

	status := "success"
	if len(execErrors) > 0 {
		status = "partial_error"
	}

	emit(engine.ExecutionEvent{
		Type:   engine.EventDone,
		Status: status,
	})

	return &models.ExecutionResult{
		Status:      status,
		Outputs:     terminalOutputs(execCtx, dag),
		NodeOutputs: execCtx.AllOutputs(),
		Errors:      execErrors,
	}, nil
}

func allInputsEmpty(inputs [][]models.Item) bool {
	for _, group := range inputs {
		if len(group) > 0 {
			return false
		}
	}
	return true
}

// nodeOutputsPreview returns a small map suitable for embedding in an event.
// We don't ship full items over the SSE stream — they can be huge — just a
// per-output-index count + the first item's keys.
func nodeOutputsPreview(out map[int][]models.Item) map[string]any {
	if len(out) == 0 {
		return nil
	}
	preview := map[string]any{}
	for idx, items := range out {
		entry := map[string]any{"count": len(items)}
		if len(items) > 0 {
			keys := make([]string, 0, len(items[0]))
			for k := range items[0] {
				keys = append(keys, k)
			}
			entry["sample_keys"] = keys
		}
		preview[fmt.Sprintf("%d", idx)] = entry
	}
	return preview
}

// emitNodeLogger is the per-node engine.NodeLogger implementation. Each
// Log call becomes an EventNodeLog event tagged with the node name + type.
type emitNodeLogger struct {
	emitter engine.Emitter
	ctx     context.Context
	execID  string
	node    string
	ntype   string
}

func (l *emitNodeLogger) Log(line string) {
	if l == nil || l.emitter == nil {
		return
	}
	l.emitter.Emit(l.ctx, l.execID, engine.ExecutionEvent{
		Type:     engine.EventNodeLog,
		Node:     l.node,
		NodeType: l.ntype,
		Content:  line,
	})
}

func terminalOutputs(ctx *engine.ExecutionContext, dag *engine.DAG) map[string]map[int][]models.Item {
	result := make(map[string]map[int][]models.Item)
	all := ctx.AllOutputs()
	for nodeName := range dag.Nodes {
		if len(dag.Adjacency[nodeName]) == 0 {
			if out, ok := all[nodeName]; ok {
				result[nodeName] = out
			}
		}
	}
	return result
}

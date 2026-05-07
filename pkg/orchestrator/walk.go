package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

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
		outputs, runErr := runNode(ctx, "node:"+nm, func(c context.Context) (map[int][]models.Item, error) {
			return ex(c, nd, in, execCtx)
		})
		if runErr != nil {
			execErrors = append(execErrors, fmt.Sprintf("node %q: %v", nm, runErr))
			continue
		}
		for outIdx, items := range outputs {
			execCtx.SetOutput(nm, outIdx, items)
		}
	}

	status := "success"
	if len(execErrors) > 0 {
		status = "partial_error"
	}

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

package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/lyzrai/flow/pkg/models"
)

// retryableNodeTypes lists node types where retry makes sense (I/O, external calls).
// Deterministic nodes (If, Set, Merge, etc.) never retry.
var retryableNodeTypes = map[string]bool{
	"flow-nodes-base.httpRequest":     true,
	"flow-nodes-base.code":            true,
	"flow-nodes-base.executeWorkflow": true,
}

// executeWithRetry wraps an executor call with optional retry logic from node.Settings.
// Only applies to retryable node types. Deterministic nodes skip retry.
func executeWithRetry(ctx context.Context, executorFn NodeExecutorFunc, node models.NodeDef, inputs [][]models.Item, execCtx *ExecutionContext) (map[int][]models.Item, error) {
	retryOnFail, _ := node.Settings["retryOnFail"].(bool)
	if !retryOnFail || !retryableNodeTypes[node.Type] {
		return executorFn(ctx, node, inputs, execCtx)
	}

	maxTries := 3
	if v, ok := node.Settings["maxTries"].(float64); ok && v > 0 {
		maxTries = int(v)
	}
	waitMs := 1000
	if v, ok := node.Settings["waitBetweenTries"].(float64); ok && v > 0 {
		waitMs = int(v)
	}

	var lastErr error
	for attempt := 0; attempt <= maxTries; attempt++ {
		outputs, err := executorFn(ctx, node, inputs, execCtx)
		if err == nil {
			if attempt > 0 {
				slog.InfoContext(ctx, "node_retry_succeeded",
					slog.String("node", node.Name),
					slog.Int("attempt", attempt+1),
				)
			}
			return outputs, nil
		}
		lastErr = err
		if attempt < maxTries {
			slog.WarnContext(ctx, "node_retry",
				slog.String("node", node.Name),
				slog.Int("attempt", attempt+1),
				slog.Int("max", maxTries+1),
				slog.Any("error", err),
			)
			time.Sleep(time.Duration(waitMs) * time.Millisecond)
		}
	}
	return nil, lastErr
}

// allInputsEmpty returns true if all input slots are empty.
func allInputsEmpty(inputs [][]models.Item) bool {
	for _, group := range inputs {
		if len(group) > 0 {
			return false
		}
	}
	return true
}

// forwardInputs stores the node's inputs as its outputs so downstream nodes
// aren't starved when an executor fails or is missing.
func forwardInputs(ctx *ExecutionContext, nodeName string, inputs [][]models.Item) {
	var all []models.Item
	for _, input := range inputs {
		all = append(all, input...)
	}
	if len(all) > 0 {
		ctx.SetOutput(nodeName, 0, all)
	}
}

func getTerminalOutputs(ctx *ExecutionContext, dag *DAG) map[string]map[int][]models.Item {
	result := make(map[string]map[int][]models.Item)
	allOutputs := ctx.AllOutputs()

	for nodeName := range dag.Nodes {
		if ctx.IsDelegated(nodeName) {
			continue
		}
		if isEffectiveTerminal(ctx, dag, nodeName) {
			if out, ok := allOutputs[nodeName]; ok {
				result[nodeName] = out
			}
		}
	}

	return result
}

func isEffectiveTerminal(ctx *ExecutionContext, dag *DAG, nodeName string) bool {
	edges := dag.Adjacency[nodeName]
	if len(edges) == 0 {
		return true
	}
	for _, edge := range edges {
		if !ctx.IsDelegated(edge.Target) {
			return false
		}
	}
	return true
}

package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lyzrai/flow/pkg/models"
)

// terminalError is unwrapped by the runner to halt execution.
// Executors and durability layers can wrap their errors with TerminalError
// to signal "do not retry, do not forward inputs, stop the workflow".
type terminalError struct{ Err error }

func (e *terminalError) Error() string { return e.Err.Error() }
func (e *terminalError) Unwrap() error { return e.Err }

// TerminalError marks an error as non-retryable.
func TerminalError(err error) error {
	if err == nil {
		return nil
	}
	return &terminalError{Err: err}
}

// IsTerminalError returns true if err (or any wrapped error) is terminal.
func IsTerminalError(err error) bool {
	var te *terminalError
	return errors.As(err, &te)
}

// RunWorkflow builds the DAG and executes nodes in topological order.
func RunWorkflow(ctx context.Context, wf *models.WorkflowDefinition, triggerData []models.Item, lookup ExecutorLookup) (*models.ExecutionResult, error) {
	dag, err := BuildDAG(wf)
	if err != nil {
		return nil, fmt.Errorf("failed to build DAG: %w", err)
	}

	order, err := dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("failed to sort DAG: %w", err)
	}

	slog.InfoContext(ctx, "workflow_started",
		slog.String("workflow", wf.Name),
		slog.Int("node_count", len(order)),
	)

	execCtx := NewExecutionContext(wf, dag)
	execCtx.Lookup = lookup

	var execErrors []string

	for _, nodeName := range order {
		if execCtx.IsDelegated(nodeName) {
			continue
		}

		node := dag.Nodes[nodeName]

		var inputs [][]models.Item
		if node.Type == "flow-nodes-base.trigger" && triggerData != nil {
			inputs = [][]models.Item{triggerData}
		} else {
			inputs = execCtx.GatherInputs(nodeName)
		}

		// Skip non-trigger root nodes (orphaned — no path from trigger).
		if node.Type != "flow-nodes-base.trigger" && len(dag.InEdges[nodeName]) == 0 {
			slog.InfoContext(ctx, "node_skipped_no_trigger_path",
				slog.String("node", nodeName),
				slog.String("type", node.Type),
			)
			continue
		}

		// Skip nodes that have upstream connections but received no items.
		if len(dag.InEdges[nodeName]) > 0 && allInputsEmpty(inputs) {
			slog.InfoContext(ctx, "node_skipped_no_input",
				slog.String("node", nodeName),
				slog.String("type", node.Type),
			)
			continue
		}

		resolvedParams := ResolveExpressions(node.Parameters, execCtx, nodeName)
		node.Parameters = resolvedParams

		executorFn, err := lookup(node.Type)
		if err != nil {
			slog.ErrorContext(ctx, "node_skipped",
				slog.String("node", nodeName),
				slog.String("type", node.Type),
				slog.String("error", err.Error()),
			)
			execErrors = append(execErrors, fmt.Sprintf("node %q: %v", nodeName, err))
			forwardInputs(execCtx, nodeName, inputs)
			continue
		}

		start := time.Now()
		outputs, err := executeWithRetry(ctx, executorFn, node, inputs, execCtx)
		elapsed := time.Since(start)

		if err != nil {
			slog.ErrorContext(ctx, "node_failed",
				slog.String("node", nodeName),
				slog.String("type", node.Type),
				slog.String("error", err.Error()),
				slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000.0),
			)
			if IsTerminalError(err) {
				return nil, fmt.Errorf("node %q: %w", nodeName, err)
			}
			execErrors = append(execErrors, fmt.Sprintf("node %q execution failed: %v", nodeName, err))
			forwardInputs(execCtx, nodeName, inputs)
			continue
		}

		slog.InfoContext(ctx, "node_executed",
			slog.String("node", nodeName),
			slog.String("type", node.Type),
			slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000.0),
		)

		for outIdx, items := range outputs {
			execCtx.SetOutput(nodeName, outIdx, items)
		}
	}

	status := "success"
	if len(execErrors) > 0 {
		status = "partial_error"
	}

	slog.InfoContext(ctx, "workflow_completed",
		slog.String("workflow", wf.Name),
		slog.String("status", status),
		slog.Int("errors", len(execErrors)),
	)

	return &models.ExecutionResult{
		Status:      status,
		Outputs:     getTerminalOutputs(execCtx, dag),
		NodeOutputs: execCtx.AllOutputs(),
		Errors:      execErrors,
	}, nil
}

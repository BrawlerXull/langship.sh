package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/lyzrai/flow/pkg/models"
)

// passThrough returns input items as-is on output 0; emits 1 item if no inputs.
func passThroughLookup() ExecutorLookup {
	return func(_ string) (NodeExecutorFunc, error) {
		return func(_ context.Context, _ models.NodeDef, inputs [][]models.Item, _ *ExecutionContext) (map[int][]models.Item, error) {
			var items []models.Item
			for _, in := range inputs {
				items = append(items, in...)
			}
			if len(items) == 0 {
				items = []models.Item{{}}
			}
			return map[int][]models.Item{0: items}, nil
		}, nil
	}
}

func twoNodeWorkflow() *models.WorkflowDefinition {
	return &models.WorkflowDefinition{
		Name: "two-node",
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T", Type: "flow-nodes-base.trigger", Parameters: map[string]any{}},
			{ID: "2", Name: "M", Type: "flow-nodes-base.set", Parameters: map[string]any{}},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "T", SourceOutputIndex: 0, TargetNode: "M", TargetInputIndex: 0},
		},
	}
}

func TestRunWorkflow_executesAllNodes(t *testing.T) {
	wf := twoNodeWorkflow()
	result, err := RunWorkflow(context.Background(), wf, []models.Item{{"hi": "there"}}, passThroughLookup())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status: %q (errors=%v)", result.Status, result.Errors)
	}
	if len(result.NodeOutputs["T"][0]) == 0 {
		t.Fatal("expected trigger to emit items")
	}
	// M is a leaf — its outputs should appear under terminal Outputs.
	if _, ok := result.Outputs["M"]; !ok {
		t.Fatalf("expected M as terminal output, got %v", result.Outputs)
	}
}

func TestRunWorkflow_recordsErrors_partial(t *testing.T) {
	wf := twoNodeWorkflow()
	calls := 0
	failingLookup := func(_ string) (NodeExecutorFunc, error) {
		return func(_ context.Context, node models.NodeDef, _ [][]models.Item, _ *ExecutionContext) (map[int][]models.Item, error) {
			calls++
			if node.Name == "M" {
				return nil, errors.New("simulated")
			}
			return map[int][]models.Item{0: {{}}}, nil
		}, nil
	}
	result, err := RunWorkflow(context.Background(), wf, nil, failingLookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "partial_error" {
		t.Fatalf("expected partial_error, got %q", result.Status)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error message recorded")
	}
}

func TestRunWorkflow_terminalErrorHaltsRun(t *testing.T) {
	wf := twoNodeWorkflow()
	failingLookup := func(_ string) (NodeExecutorFunc, error) {
		return func(_ context.Context, node models.NodeDef, _ [][]models.Item, _ *ExecutionContext) (map[int][]models.Item, error) {
			if node.Name == "M" {
				return nil, TerminalError(errors.New("fatal"))
			}
			return map[int][]models.Item{0: {{}}}, nil
		}, nil
	}
	_, err := RunWorkflow(context.Background(), wf, nil, failingLookup)
	if err == nil {
		t.Fatal("expected error from terminal failure")
	}
	if !IsTerminalError(err) {
		t.Fatalf("expected terminal classification, got %v", err)
	}
}

func TestRunWorkflow_unknownNodeTypeContinues(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Name: "unknown",
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T", Type: "flow-nodes-base.trigger", Parameters: map[string]any{}},
			{ID: "2", Name: "X", Type: "flow-nodes-base.does-not-exist", Parameters: map[string]any{}},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "T", TargetNode: "X"},
		},
	}
	lookup := func(nodeType string) (NodeExecutorFunc, error) {
		if nodeType == "flow-nodes-base.does-not-exist" {
			return nil, errors.New("not registered")
		}
		return passThroughLookup()(nodeType)
	}
	result, err := RunWorkflow(context.Background(), wf, nil, lookup)
	if err != nil {
		t.Fatalf("unknown node should not abort the run: %v", err)
	}
	if result.Status != "partial_error" {
		t.Fatalf("status: %q", result.Status)
	}
}

func TestAllInputsEmpty_helper(t *testing.T) {
	if !allInputsEmpty(nil) {
		t.Error("nil should be empty")
	}
	if !allInputsEmpty([][]models.Item{{}, {}}) {
		t.Error("empty groups should be empty")
	}
	if allInputsEmpty([][]models.Item{{{"k": 1}}}) {
		t.Error("non-empty group should not be empty")
	}
}

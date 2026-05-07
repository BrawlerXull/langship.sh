package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/lyzrai/flow/pkg/models"
)

func TestExecuteWithRetry_skipsRetryForNonRetryableNode(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ models.NodeDef, _ [][]models.Item, _ *ExecutionContext) (map[int][]models.Item, error) {
		calls++
		return nil, errors.New("boom")
	}
	node := models.NodeDef{
		Type: "flow-nodes-base.set", // not in retryableNodeTypes
		Settings: map[string]any{
			"retryOnFail":      true,
			"maxTries":         float64(3),
			"waitBetweenTries": float64(1),
		},
	}
	_, err := executeWithRetry(context.Background(), exec, node, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (retry disabled), got %d", calls)
	}
}

func TestExecuteWithRetry_retriesRetryableNode(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ models.NodeDef, _ [][]models.Item, _ *ExecutionContext) (map[int][]models.Item, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("flaky")
		}
		return map[int][]models.Item{0: {{"ok": true}}}, nil
	}
	node := models.NodeDef{
		Type: "flow-nodes-base.httpRequest",
		Settings: map[string]any{
			"retryOnFail":      true,
			"maxTries":         float64(5),
			"waitBetweenTries": float64(1),
		},
	}
	got, err := executeWithRetry(context.Background(), exec, node, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
	if got[0][0]["ok"] != true {
		t.Fatalf("unexpected output: %+v", got)
	}
}

func TestExecuteWithRetry_noRetryWhenFlagOff(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ models.NodeDef, _ [][]models.Item, _ *ExecutionContext) (map[int][]models.Item, error) {
		calls++
		return nil, errors.New("boom")
	}
	node := models.NodeDef{
		Type: "flow-nodes-base.httpRequest",
		// No Settings → retryOnFail defaults false.
	}
	_, _ = executeWithRetry(context.Background(), exec, node, nil, nil)
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestForwardInputs_storesFlattenedItemsOnOutput0(t *testing.T) {
	wf := &models.WorkflowDefinition{Nodes: []models.NodeDef{{Name: "X"}}}
	dag, _ := BuildDAG(wf)
	c := NewExecutionContext(wf, dag)

	forwardInputs(c, "X", [][]models.Item{{{"a": 1}}, {{"b": 2}}})
	got := c.GetNodeOutput("X", 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 forwarded items, got %d", len(got))
	}
}

func TestGetTerminalOutputs_excludesNodesWithDownstream(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "A"},
			{ID: "2", Name: "B"},
			{ID: "3", Name: "C"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "A", TargetNode: "B"},
			{SourceNode: "B", TargetNode: "C"},
		},
	}
	dag, _ := BuildDAG(wf)
	c := NewExecutionContext(wf, dag)
	c.SetOutput("A", 0, []models.Item{{"a": 1}})
	c.SetOutput("B", 0, []models.Item{{"b": 1}})
	c.SetOutput("C", 0, []models.Item{{"c": 1}})

	got := getTerminalOutputs(c, dag)
	if _, ok := got["C"]; !ok {
		t.Fatal("C should be terminal")
	}
	if _, ok := got["A"]; ok {
		t.Fatal("A should not be terminal")
	}
	if _, ok := got["B"]; ok {
		t.Fatal("B should not be terminal")
	}
}

func TestExecutionContext_GatherInputs(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{{ID: "1", Name: "A"}, {ID: "2", Name: "B"}},
		Connections: []models.ConnectionDef{
			{SourceNode: "A", SourceOutputIndex: 0, TargetNode: "B", TargetInputIndex: 0},
		},
	}
	dag, _ := BuildDAG(wf)
	c := NewExecutionContext(wf, dag)
	c.SetOutput("A", 0, []models.Item{{"k": "v"}})

	inputs := c.GatherInputs("B")
	if len(inputs) != 1 || len(inputs[0]) != 1 || inputs[0][0]["k"] != "v" {
		t.Fatalf("unexpected gather: %+v", inputs)
	}
}

func TestExecutionContext_DelegationFlag(t *testing.T) {
	c := &ExecutionContext{}
	if c.IsDelegated("X") {
		t.Fatal("default false")
	}
	c.MarkDelegated("X")
	if !c.IsDelegated("X") {
		t.Fatal("expected true after Mark")
	}
}

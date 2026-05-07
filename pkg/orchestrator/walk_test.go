package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

func newTestWorkflow() *models.WorkflowDefinition {
	return &models.WorkflowDefinition{
		Name: "test",
		Nodes: []models.NodeDef{
			{ID: "1", Name: "Trigger", Type: "flow-nodes-base.trigger", Parameters: map[string]any{}},
			{ID: "2", Name: "Noop", Type: "flow-nodes-base.noOp", Parameters: map[string]any{}},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "Trigger", SourceOutputIndex: 0, TargetNode: "Noop", TargetInputIndex: 0},
		},
	}
}

// fakeRunner records each step name and forwards execution.
func fakeRunner(steps *[]string) NodeRunner {
	return func(ctx context.Context, stepID string, fn func(context.Context) (map[int][]models.Item, error)) (map[int][]models.Item, error) {
		*steps = append(*steps, stepID)
		return fn(ctx)
	}
}

func passThroughLookup() engine.ExecutorLookup {
	return func(_ string) (engine.NodeExecutorFunc, error) {
		return func(_ context.Context, _ models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
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

func TestWalkDurable_runsNodesViaNodeRunner(t *testing.T) {
	wf := newTestWorkflow()
	var steps []string
	result, err := walkDurable(context.Background(), wf, []models.Item{{"in": "x"}}, passThroughLookup(), fakeRunner(&steps))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success, got %q (errors=%v)", result.Status, result.Errors)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps via NodeRunner, got %d: %v", len(steps), steps)
	}
	for _, want := range []string{"node:Trigger", "node:Noop"} {
		found := false
		for _, s := range steps {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected step %q in %v", want, steps)
		}
	}
}

func TestWalkDurable_collectsTerminalOutputs(t *testing.T) {
	wf := newTestWorkflow()
	steps := []string{}
	result, err := walkDurable(context.Background(), wf, []models.Item{{"hi": "world"}}, passThroughLookup(), fakeRunner(&steps))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Noop has no outgoing edges → it's the terminal node.
	if _, ok := result.Outputs["Noop"]; !ok {
		t.Fatalf("expected terminal output for Noop, got %v", result.Outputs)
	}
	if _, ok := result.Outputs["Trigger"]; ok {
		t.Fatalf("Trigger should not be a terminal output (has downstream edge)")
	}
}

func TestWalkDurable_recordsExecErrors(t *testing.T) {
	wf := newTestWorkflow()
	calls := int32(0)

	failingLookup := func(_ string) (engine.NodeExecutorFunc, error) {
		return func(_ context.Context, node models.NodeDef, _ [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
			n := atomic.AddInt32(&calls, 1)
			if n == 2 {
				return nil, errors.New("simulated failure")
			}
			return map[int][]models.Item{0: {{"ok": true}}}, nil
		}, nil
	}

	steps := []string{}
	result, err := walkDurable(context.Background(), wf, []models.Item{{}}, failingLookup, fakeRunner(&steps))
	if err != nil {
		t.Fatalf("walk should not return error for non-terminal failures, got %v", err)
	}
	if result.Status != "partial_error" {
		t.Fatalf("expected partial_error, got %q", result.Status)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error message recorded")
	}
}

func TestWalkDurable_failsOnCycle(t *testing.T) {
	// Two non-trigger nodes pointing at each other = cycle.
	wf := &models.WorkflowDefinition{
		Name: "cycle",
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T", Type: "flow-nodes-base.trigger"},
			{ID: "2", Name: "A", Type: "flow-nodes-base.noOp"},
			{ID: "3", Name: "B", Type: "flow-nodes-base.noOp"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "T", TargetNode: "A"},
			{SourceNode: "A", TargetNode: "B"},
			{SourceNode: "B", TargetNode: "A"},
		},
	}
	steps := []string{}
	_, err := walkDurable(context.Background(), wf, nil, passThroughLookup(), fakeRunner(&steps))
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestRequestMeta_isSerializable(t *testing.T) {
	// Smoke check: types compile and zero values work.
	r := RunRequest{RequestMeta: RequestMeta{APIKey: "k", OrgID: "o"}}
	if r.RequestMeta.APIKey != "k" || r.RequestMeta.OrgID != "o" {
		t.Fatal("RequestMeta fields lost on assignment")
	}
}

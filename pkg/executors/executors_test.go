package executors

import (
	"context"
	"errors"
	"testing"

	"github.com/lyzrai/flow/pkg/durability"
	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

func TestTrigger_emitsAtLeastOneItemWhenInputEmpty(t *testing.T) {
	out, err := (&TriggerExecutor{}).Execute(context.Background(),
		models.NodeDef{Name: "t", Type: "flow-nodes-base.trigger"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 item on output 0, got %d", len(out[0]))
	}
}

func TestTrigger_passesThroughInputItems(t *testing.T) {
	in := [][]models.Item{{{"a": 1}, {"a": 2}}}
	out, err := (&TriggerExecutor{}).Execute(context.Background(),
		models.NodeDef{Name: "t", Type: "flow-nodes-base.trigger"}, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out[0]) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out[0]))
	}
}

func TestNoOp_passesAllItemsThrough(t *testing.T) {
	in := [][]models.Item{{{"x": "a"}}, {{"x": "b"}}}
	out, err := (&NoOpExecutor{}).Execute(context.Background(), models.NodeDef{}, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out[0]) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out[0]))
	}
}

func TestSet_v3Assignments(t *testing.T) {
	node := models.NodeDef{Parameters: map[string]any{
		"assignments": map[string]any{
			"assignments": []any{
				map[string]any{"name": "greeting", "value": "hi", "type": "string"},
				map[string]any{"name": "version", "value": float64(2), "type": "number"},
			},
		},
	}}
	in := [][]models.Item{{{"existing": true}}}
	out, err := (&SetExecutor{}).Execute(context.Background(), node, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out[0]))
	}
	got := out[0][0]
	if got["greeting"] != "hi" || got["version"] != float64(2) || got["existing"] != true {
		t.Fatalf("unexpected merged item: %+v", got)
	}
}

func TestSet_v3FieldsValuesShape(t *testing.T) {
	node := models.NodeDef{Parameters: map[string]any{
		"fields": map[string]any{
			"values": []any{
				map[string]any{"name": "country", "stringValue": "IN"},
				map[string]any{"name": "rank", "numberValue": float64(7)},
			},
		},
	}}
	in := [][]models.Item{{{}}}
	out, err := (&SetExecutor{}).Execute(context.Background(), node, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out[0][0]
	if got["country"] != "IN" || got["rank"] != float64(7) {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestSet_legacyValuesShape(t *testing.T) {
	node := models.NodeDef{Parameters: map[string]any{
		"values": map[string]any{
			"string": []any{
				map[string]any{"name": "env", "value": "prod"},
			},
			"number": []any{
				map[string]any{"name": "port", "value": float64(8080)},
			},
		},
	}}
	in := [][]models.Item{{{}}}
	out, err := (&SetExecutor{}).Execute(context.Background(), node, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out[0][0]
	if got["env"] != "prod" || got["port"] != float64(8080) {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestSet_doesNotMutateInputItem(t *testing.T) {
	original := models.Item{"k": "v"}
	node := models.NodeDef{Parameters: map[string]any{
		"assignments": map[string]any{
			"assignments": []any{map[string]any{"name": "k", "value": "v2"}},
		},
	}}
	in := [][]models.Item{{original}}
	out, _ := (&SetExecutor{}).Execute(context.Background(), node, in, nil)
	if out[0][0]["k"] != "v2" {
		t.Fatalf("expected output to be overridden, got %v", out[0][0]["k"])
	}
	if original["k"] != "v" {
		t.Fatal("Set must not mutate the input item in place")
	}
}

// Approval node refuses to run without a Restate context, returning a
// PermanentError so the engine doesn't retry.
func TestApproval_requiresRestateContext(t *testing.T) {
	node := models.NodeDef{Name: "Approve", Type: "flow-nodes-base.waitForApproval"}
	in := [][]models.Item{{{}}}

	_, err := (&ApprovalExecutor{}).Execute(context.Background(), node, in, &engine.ExecutionContext{})
	if err == nil {
		t.Fatal("expected approval node to refuse running outside Restate")
	}
	var perm *durability.PermanentError
	if !errors.As(err, &perm) {
		t.Fatalf("expected PermanentError so engine doesn't retry, got %T", err)
	}
}

func TestRegistry_RegisterAll_registersExpectedTypes(t *testing.T) {
	RegisterAll()
	for _, want := range []string{
		"flow-nodes-base.trigger",
		"flow-nodes-base.noOp",
		"flow-nodes-base.set",
		"flow-nodes-base.waitForApproval",
	} {
		if _, err := Get(want); err != nil {
			t.Errorf("missing executor for %q: %v", want, err)
		}
	}
}

func TestBuildLookup_returnsFunctioningExecutor(t *testing.T) {
	RegisterAll()
	lookup := BuildLookup()
	fn, err := lookup("flow-nodes-base.trigger")
	if err != nil {
		t.Fatalf("lookup miss: %v", err)
	}
	out, err := fn(context.Background(), models.NodeDef{Type: "flow-nodes-base.trigger"}, nil, nil)
	if err != nil {
		t.Fatalf("trigger execution failed: %v", err)
	}
	if len(out[0]) != 1 {
		t.Fatalf("trigger should emit 1 item, got %d", len(out[0]))
	}
}

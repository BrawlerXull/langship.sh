package engine

import (
	"reflect"
	"testing"

	"github.com/lyzrai/flow/pkg/models"
)

// helper: build a context with a single upstream node Producer that has output 0.
func ctxWith(produced models.Item) *ExecutionContext {
	wf := &models.WorkflowDefinition{Nodes: []models.NodeDef{{Name: "Producer"}, {Name: "Consumer"}}}
	dag, _ := BuildDAG(&models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "Producer", Type: "flow-nodes-base.set"},
			{ID: "2", Name: "Consumer", Type: "flow-nodes-base.noOp"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "Producer", TargetNode: "Consumer", SourceOutputIndex: 0, TargetInputIndex: 0},
		},
	})
	c := NewExecutionContext(wf, dag)
	c.SetOutput("Producer", 0, []models.Item{produced})
	return c
}

func TestResolveString_dollarJsonField(t *testing.T) {
	c := ctxWith(models.Item{"name": "ada", "n": float64(42)})
	got := resolveString("={{ $json.name }}", c, "Consumer")
	if got != "ada" {
		t.Fatalf("got %v want ada", got)
	}
}

func TestResolveString_dollarJsonNumberPreservesType(t *testing.T) {
	c := ctxWith(models.Item{"n": float64(42)})
	got := resolveString("={{ $json.n }}", c, "Consumer")
	if got != float64(42) {
		t.Fatalf("got %T %v, want float64 42", got, got)
	}
}

func TestResolveString_dollarParenNodeReference(t *testing.T) {
	c := ctxWith(models.Item{"k": "v"})
	got := resolveString(`={{ $('Producer').json.k }}`, c, "Consumer")
	if got != "v" {
		t.Fatalf("got %v want v", got)
	}
}

func TestResolveString_dollarNodeBracketReference(t *testing.T) {
	c := ctxWith(models.Item{"k": "v"})
	got := resolveString(`={{ $node["Producer"].json.k }}`, c, "Consumer")
	if got != "v" {
		t.Fatalf("got %v want v", got)
	}
}

func TestResolveString_interpolation(t *testing.T) {
	c := ctxWith(models.Item{"name": "ada"})
	got := resolveString("hi {{ $json.name }}", c, "Consumer")
	if got != "hi ada" {
		t.Fatalf("got %q want %q", got, "hi ada")
	}
}

func TestResolveString_jsArithmetic(t *testing.T) {
	c := ctxWith(models.Item{"n": float64(7)})
	got := resolveString("={{ $json.n + 3 }}", c, "Consumer")
	// goja returns int64 for integer arithmetic.
	if got != int64(10) && got != float64(10) {
		t.Fatalf("got %T %v want 10", got, got)
	}
}

func TestResolveString_unknownField_returnsNil(t *testing.T) {
	c := ctxWith(models.Item{"k": "v"})
	got := resolveString("={{ $json.missing }}", c, "Consumer")
	if got != nil {
		t.Fatalf("got %v, want nil for missing field", got)
	}
}

func TestResolveExpressions_recursive(t *testing.T) {
	c := ctxWith(models.Item{"name": "ada", "age": float64(7)})
	in := map[string]any{
		"top": "={{ $json.name }}",
		"nested": map[string]any{
			"k": "={{ $json.age }}",
		},
		"list": []any{"={{ $json.name }}", "static"},
	}
	got := ResolveExpressions(in, c, "Consumer")
	wantNested := map[string]any{"k": float64(7)}
	if !reflect.DeepEqual(got["nested"], wantNested) {
		t.Errorf("nested: got %+v want %+v", got["nested"], wantNested)
	}
	wantList := []any{"ada", "static"}
	if !reflect.DeepEqual(got["list"], wantList) {
		t.Errorf("list: got %+v want %+v", got["list"], wantList)
	}
	if got["top"] != "ada" {
		t.Errorf("top: got %v", got["top"])
	}
}

func TestTraverseField_dotPath(t *testing.T) {
	item := models.Item{
		"user": map[string]any{"name": "ada", "addr": map[string]any{"city": "London"}},
	}
	if got := traverseField(item, "user.name"); got != "ada" {
		t.Errorf("got %v", got)
	}
	if got := traverseField(item, "user.addr.city"); got != "London" {
		t.Errorf("got %v", got)
	}
	if got := traverseField(item, "user.addr.zip"); got != nil {
		t.Errorf("expected nil for missing path, got %v", got)
	}
}

func TestResolveString_jsEvalFailureReturnsExpressionLiteral(t *testing.T) {
	c := ctxWith(models.Item{})
	got := resolveString("={{ this is not js }}", c, "Consumer")
	// Failure path returns "{{ expr }}" so the broken expression is visible at runtime.
	if got != "{{ this is not js }}" {
		t.Fatalf("got %v", got)
	}
}

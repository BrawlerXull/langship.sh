package engine

import (
	"testing"

	"github.com/lyzrai/flow/pkg/models"
)

// Loop has two outputs: 0 → body entry, 1 → continuation.
// Layout:
//
//	Trigger → Loop → (out0) Body1 → Body2
//	             └→ (out1) After
func TestGetLoopBody_topologicalAndTerminal(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T", Type: "flow-nodes-base.trigger"},
			{ID: "2", Name: "Loop", Type: "flow-nodes-base.splitInBatches"},
			{ID: "3", Name: "Body1", Type: "flow-nodes-base.set"},
			{ID: "4", Name: "Body2", Type: "flow-nodes-base.noOp"},
			{ID: "5", Name: "After", Type: "flow-nodes-base.noOp"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "T", TargetNode: "Loop"},
			{SourceNode: "Loop", SourceOutputIndex: 0, TargetNode: "Body1"},
			{SourceNode: "Body1", TargetNode: "Body2"},
			{SourceNode: "Loop", SourceOutputIndex: 1, TargetNode: "After"},
		},
	}
	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("build dag: %v", err)
	}
	body, err := GetLoopBody(dag, "Loop")
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	if len(body.Order) != 2 {
		t.Fatalf("expected 2 body nodes, got %v", body.Order)
	}
	// Body1 must come before Body2 in topo order.
	idx := map[string]int{}
	for i, n := range body.Order {
		idx[n] = i
	}
	if !(idx["Body1"] < idx["Body2"]) {
		t.Errorf("expected Body1 before Body2, got %v", body.Order)
	}
	if !body.Nodes["Body1"] || !body.Nodes["Body2"] {
		t.Errorf("body nodes set: %v", body.Nodes)
	}
	if body.Nodes["After"] {
		t.Error("After should NOT be in body (it's behind output 1)")
	}
	// Body2 has no successors inside the body → terminal.
	if len(body.TerminalNodes) != 1 || body.TerminalNodes[0] != "Body2" {
		t.Errorf("terminals: %v", body.TerminalNodes)
	}
}

func TestGetLoopBody_returnsErrorWithNoBody(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T", Type: "flow-nodes-base.trigger"},
			{ID: "2", Name: "Loop", Type: "flow-nodes-base.splitInBatches"},
			{ID: "3", Name: "After", Type: "flow-nodes-base.noOp"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "T", TargetNode: "Loop"},
			// Only output 1 wired — no body.
			{SourceNode: "Loop", SourceOutputIndex: 1, TargetNode: "After"},
		},
	}
	dag, _ := BuildDAG(wf)
	if _, err := GetLoopBody(dag, "Loop"); err == nil {
		t.Fatal("expected error when loop has no body")
	}
}

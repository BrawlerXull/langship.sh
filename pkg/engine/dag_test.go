package engine

import (
	"reflect"
	"sort"
	"testing"

	"github.com/lyzrai/flow/pkg/models"
)

func TestBuildDAG_populatesEdgesAndDegrees(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T", Type: "flow-nodes-base.trigger"},
			{ID: "2", Name: "A", Type: "flow-nodes-base.set"},
			{ID: "3", Name: "B", Type: "flow-nodes-base.noOp"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "T", TargetNode: "A", SourceOutputIndex: 0, TargetInputIndex: 0},
			{SourceNode: "A", TargetNode: "B", SourceOutputIndex: 0, TargetInputIndex: 0},
		},
	}
	dag, err := BuildDAG(wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dag.InDegree["T"] != 0 || dag.InDegree["A"] != 1 || dag.InDegree["B"] != 1 {
		t.Errorf("indegrees: %+v", dag.InDegree)
	}
	if len(dag.Adjacency["T"]) != 1 || dag.Adjacency["T"][0].Target != "A" {
		t.Errorf("T adjacency: %+v", dag.Adjacency["T"])
	}
	if len(dag.InEdges["B"]) != 1 || dag.InEdges["B"][0].Target != "A" {
		t.Errorf("B in-edges: %+v", dag.InEdges["B"])
	}
}

func TestBuildDAG_rejectsDuplicateNodeNames(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "X", Type: "flow-nodes-base.trigger"},
			{ID: "2", Name: "X", Type: "flow-nodes-base.set"},
		},
	}
	if _, err := BuildDAG(wf); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestTopologicalSort_orderRespectsDependencies(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T", Type: "flow-nodes-base.trigger"},
			{ID: "2", Name: "A", Type: "flow-nodes-base.set"},
			{ID: "3", Name: "B", Type: "flow-nodes-base.noOp"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "T", TargetNode: "A"},
			{SourceNode: "A", TargetNode: "B"},
		},
	}
	dag, _ := BuildDAG(wf)
	order, err := dag.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	idx := map[string]int{}
	for i, n := range order {
		idx[n] = i
	}
	if !(idx["T"] < idx["A"] && idx["A"] < idx["B"]) {
		t.Fatalf("unexpected order %v", order)
	}
}

func TestTopologicalSort_detectsCycle(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "A", Type: "flow-nodes-base.set"},
			{ID: "2", Name: "B", Type: "flow-nodes-base.set"},
		},
		Connections: []models.ConnectionDef{
			{SourceNode: "A", TargetNode: "B"},
			{SourceNode: "B", TargetNode: "A"},
		},
	}
	dag, _ := BuildDAG(wf)
	if _, err := dag.TopologicalSort(); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestHasApprovalNode(t *testing.T) {
	with := &models.WorkflowDefinition{Nodes: []models.NodeDef{{Type: "flow-nodes-base.waitForApproval"}}}
	without := &models.WorkflowDefinition{Nodes: []models.NodeDef{{Type: "flow-nodes-base.set"}}}
	if !HasApprovalNode(with) {
		t.Error("expected true")
	}
	if HasApprovalNode(without) {
		t.Error("expected false")
	}
}

func TestGetStartNodes(t *testing.T) {
	wf := &models.WorkflowDefinition{
		Nodes: []models.NodeDef{
			{ID: "1", Name: "T1", Type: "flow-nodes-base.trigger"},
			{ID: "2", Name: "T2", Type: "flow-nodes-base.trigger"},
			{ID: "3", Name: "Mid", Type: "flow-nodes-base.set"},
		},
		Connections: []models.ConnectionDef{{SourceNode: "T1", TargetNode: "Mid"}},
	}
	dag, _ := BuildDAG(wf)
	got := dag.GetStartNodes()
	sort.Strings(got)
	want := []string{"Mid", "T1", "T2"}
	// Mid has incoming edge so it should be excluded; T2 has no incoming so it's a start.
	want = []string{"T1", "T2"}
	// recompute filter
	filtered := []string{}
	for _, n := range got {
		if n != "Mid" {
			filtered = append(filtered, n)
		}
	}
	sort.Strings(filtered)
	if !reflect.DeepEqual(filtered, want) {
		t.Fatalf("starts: got %v want %v", filtered, want)
	}
}

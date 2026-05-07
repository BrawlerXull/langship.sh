package engine

import (
	"fmt"
	"sort"

	"github.com/lyzrai/flow/pkg/models"
)

// Edge represents a directed connection between nodes in the DAG.
type Edge struct {
	Target            string
	SourceOutputIndex int
	TargetInputIndex  int
}

// DAG is the directed acyclic graph built from a workflow's nodes and connections.
type DAG struct {
	Nodes       map[string]models.NodeDef
	Adjacency   map[string][]Edge
	InEdges     map[string][]Edge
	InDegree    map[string]int
	Connections []models.ConnectionDef
}

// BuildDAG constructs a DAG from a WorkflowDefinition.
func BuildDAG(wf *models.WorkflowDefinition) (*DAG, error) {
	dag := &DAG{
		Nodes:       make(map[string]models.NodeDef, len(wf.Nodes)),
		Adjacency:   make(map[string][]Edge),
		InEdges:     make(map[string][]Edge),
		InDegree:    make(map[string]int),
		Connections: wf.Connections,
	}

	for _, node := range wf.Nodes {
		if _, exists := dag.Nodes[node.Name]; exists {
			return nil, fmt.Errorf("duplicate node name %q", node.Name)
		}
		dag.Nodes[node.Name] = node
		dag.InDegree[node.Name] = 0
	}

	for _, conn := range wf.Connections {
		edge := Edge{
			Target:            conn.TargetNode,
			SourceOutputIndex: conn.SourceOutputIndex,
			TargetInputIndex:  conn.TargetInputIndex,
		}
		dag.Adjacency[conn.SourceNode] = append(dag.Adjacency[conn.SourceNode], edge)

		inEdge := Edge{
			Target:            conn.SourceNode,
			SourceOutputIndex: conn.SourceOutputIndex,
			TargetInputIndex:  conn.TargetInputIndex,
		}
		dag.InEdges[conn.TargetNode] = append(dag.InEdges[conn.TargetNode], inEdge)

		dag.InDegree[conn.TargetNode]++
	}

	return dag, nil
}

// TopologicalSort returns nodes in execution order using Kahn's algorithm.
// Returns an error if a cycle is detected.
func (d *DAG) TopologicalSort() ([]string, error) {
	inDegree := make(map[string]int, len(d.InDegree))
	for k, v := range d.InDegree {
		inDegree[k] = v
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	var order []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		for _, edge := range d.Adjacency[current] {
			inDegree[edge.Target]--
			if inDegree[edge.Target] == 0 {
				queue = append(queue, edge.Target)
			}
		}
	}

	if len(order) != len(d.Nodes) {
		return nil, fmt.Errorf("cycle detected in workflow graph: sorted %d of %d nodes", len(order), len(d.Nodes))
	}

	return order, nil
}

// HasApprovalNode returns true if any node in the workflow is a waitForApproval node.
// Used by the runner to gate workflows that need durable HITL.
func HasApprovalNode(wf *models.WorkflowDefinition) bool {
	for _, node := range wf.Nodes {
		if node.Type == "flow-nodes-base.waitForApproval" {
			return true
		}
	}
	return false
}

// GetStartNodes returns nodes with no incoming connections.
func (d *DAG) GetStartNodes() []string {
	var starts []string
	for name, deg := range d.InDegree {
		if deg == 0 {
			starts = append(starts, name)
		}
	}
	return starts
}

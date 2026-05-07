package engine

import (
	"github.com/lyzrai/flow/pkg/models"
)

// ExecutionContext is the data bus that carries items between nodes during execution.
type ExecutionContext struct {
	nodeOutputs map[string]map[int][]models.Item

	Workflow *models.WorkflowDefinition
	DAG      *DAG
	Lookup   ExecutorLookup

	// Events is an optional channel for streaming execution events (SSE).
	// When non-nil, the runner and executors emit events as execution progresses.
	Events chan<- ExecutionEvent

	// delegatedNodes tracks nodes claimed as delegate tools by an upstream node.
	// Reserved for governance/sub-agent extensions; unused in the base engine.
	delegatedNodes map[string]bool
}

// MarkDelegated marks a node as claimed by an upstream node.
func (c *ExecutionContext) MarkDelegated(nodeName string) {
	if c.delegatedNodes == nil {
		c.delegatedNodes = make(map[string]bool)
	}
	c.delegatedNodes[nodeName] = true
}

// IsDelegated returns true if the node has been claimed.
func (c *ExecutionContext) IsDelegated(nodeName string) bool {
	return c.delegatedNodes[nodeName]
}

// NewExecutionContext creates a new execution context for a workflow run.
func NewExecutionContext(wf *models.WorkflowDefinition, dag *DAG) *ExecutionContext {
	return &ExecutionContext{
		nodeOutputs: make(map[string]map[int][]models.Item),
		Workflow:    wf,
		DAG:         dag,
	}
}

// NewStreamingExecutionContext creates a context with an event channel for SSE streaming.
func NewStreamingExecutionContext(wf *models.WorkflowDefinition, dag *DAG, events chan<- ExecutionEvent) *ExecutionContext {
	return &ExecutionContext{
		nodeOutputs: make(map[string]map[int][]models.Item),
		Workflow:    wf,
		DAG:         dag,
		Events:      events,
	}
}

// Emit sends an event if streaming is enabled.
func (c *ExecutionContext) Emit(event ExecutionEvent) {
	if c.Events != nil {
		c.Events <- event
	}
}

// IsStreaming returns true if this context has a streaming event channel.
func (c *ExecutionContext) IsStreaming() bool {
	return c.Events != nil
}

// SetOutput stores the output items of a node at a specific output index.
func (c *ExecutionContext) SetOutput(nodeName string, outputIndex int, items []models.Item) {
	if c.nodeOutputs[nodeName] == nil {
		c.nodeOutputs[nodeName] = make(map[int][]models.Item)
	}
	c.nodeOutputs[nodeName][outputIndex] = items
}

// GetNodeOutput retrieves the output items of a specific node and output index.
func (c *ExecutionContext) GetNodeOutput(nodeName string, outputIndex int) []models.Item {
	if outputs, ok := c.nodeOutputs[nodeName]; ok {
		return outputs[outputIndex]
	}
	return nil
}

// GatherInputs collects all inputs for a given node by looking at incoming edges.
// Returns a slice where each element corresponds to an input index.
func (c *ExecutionContext) GatherInputs(nodeName string) [][]models.Item {
	inEdges := c.DAG.InEdges[nodeName]
	if len(inEdges) == 0 {
		return nil
	}

	maxIdx := 0
	for _, edge := range inEdges {
		if edge.TargetInputIndex > maxIdx {
			maxIdx = edge.TargetInputIndex
		}
	}

	inputs := make([][]models.Item, maxIdx+1)
	for _, edge := range inEdges {
		sourceItems := c.GetNodeOutput(edge.Target, edge.SourceOutputIndex)
		inputs[edge.TargetInputIndex] = append(inputs[edge.TargetInputIndex], sourceItems...)
	}

	return inputs
}

// AllOutputs returns all node outputs for building the execution result.
func (c *ExecutionContext) AllOutputs() map[string]map[int][]models.Item {
	return c.nodeOutputs
}

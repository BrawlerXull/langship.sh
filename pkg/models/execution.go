package models

// Item is a single data item flowing between nodes.
type Item = map[string]any

// NodeOutput holds the outputs of a single node, keyed by output index.
type NodeOutput = map[int][]Item

// ExecutionResult is the final result of a workflow execution.
type ExecutionResult struct {
	ExecutionID string                    `json:"execution_id"`
	Status      string                    `json:"status"`
	Outputs     map[string]map[int][]Item `json:"outputs"`
	NodeOutputs map[string]map[int][]Item `json:"node_outputs"`
	Errors      []string                  `json:"errors"`
}

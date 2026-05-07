package models

// NodeDef represents a single node in an n8n-compatible workflow.
type NodeDef struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	TypeVersion float64        `json:"typeVersion"`
	Parameters  map[string]any `json:"parameters"`
	Credentials map[string]any `json:"credentials,omitempty"`
	Settings    map[string]any `json:"settings,omitempty"`
	Position    [2]float64     `json:"position"`
}

// ConnectionDef represents a directed edge between two nodes.
type ConnectionDef struct {
	SourceNode        string
	SourceOutputIndex int
	TargetNode        string
	TargetInputIndex  int
}

// WorkflowDefinition is the parsed internal representation of a workflow.
type WorkflowDefinition struct {
	Name        string          `json:"name"`
	Nodes       []NodeDef       `json:"nodes"`
	Connections []ConnectionDef
	Settings    map[string]any `json:"settings"`
}

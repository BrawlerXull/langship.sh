package engine

// EventType identifies the kind of streaming event emitted during workflow execution.
type EventType string

const (
	EventNodeStarted   EventType = "node_started"
	EventNodeCompleted EventType = "node_completed"
	EventNodeError     EventType = "node_error"
	EventToken         EventType = "token"
	EventToolCallDelta EventType = "tool_call_delta"
	EventBusFull       EventType = "bus_full"
	EventDone          EventType = "done"
)

// ExecutionEvent is a streaming event emitted during workflow execution.
type ExecutionEvent struct {
	Type       EventType      `json:"type"`
	Node       string         `json:"node,omitempty"`
	NodeType   string         `json:"node_type,omitempty"`
	Content    string         `json:"content,omitempty"`
	Status     string         `json:"status,omitempty"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
}

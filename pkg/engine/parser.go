package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lyzrai/flow/pkg/models"
)

// n8nWorkflowJSON mirrors the raw n8n export format for unmarshalling.
type n8nWorkflowJSON struct {
	Name        string           `json:"name"`
	Nodes       []models.NodeDef `json:"nodes"`
	Connections map[string]struct {
		Main []json.RawMessage `json:"main"`
	} `json:"connections"`
	Settings map[string]any `json:"settings"`
}

type connectionTarget struct {
	Node  string `json:"node"`
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// n8nToFlow maps specific n8n node types to flow-native equivalents.
// Unmapped types are translated automatically via prefix replacement.
var n8nToFlow = map[string]string{
	// All triggers → single flow trigger type
	"n8n-nodes-base.webhook":                "flow-nodes-base.trigger",
	"n8n-nodes-base.scheduleTrigger":        "flow-nodes-base.trigger",
	"n8n-nodes-base.manualTrigger":          "flow-nodes-base.trigger",
	"n8n-nodes-base.formTrigger":            "flow-nodes-base.trigger",
	"n8n-nodes-base.activationTrigger":      "flow-nodes-base.trigger",
	"n8n-nodes-base.errorTrigger":           "flow-nodes-base.trigger",
	"n8n-nodes-base.executeWorkflowTrigger": "flow-nodes-base.trigger",
	"n8n-nodes-base.n8nTrigger":             "flow-nodes-base.trigger",
	"n8n-nodes-base.sseTrigger":             "flow-nodes-base.trigger",
	"n8n-nodes-base.workflowTrigger":        "flow-nodes-base.trigger",
	"n8n-nodes-base.localFileTrigger":       "flow-nodes-base.trigger",
	"n8n-nodes-base.emailReadImap":          "flow-nodes-base.trigger",
	"n8n-nodes-base.rssFeedReadTrigger":     "flow-nodes-base.trigger",
	"n8n-nodes-base.evaluationTrigger":      "flow-nodes-base.trigger",

	"n8n-nodes-base.start":    "flow-nodes-base.trigger",
	"n8n-nodes-base.cron":     "flow-nodes-base.trigger",
	"n8n-nodes-base.interval": "flow-nodes-base.trigger",

	"n8n-nodes-base.function":     "flow-nodes-base.code",
	"n8n-nodes-base.functionItem": "flow-nodes-base.code",

	"n8n-nodes-base.spreadsheetFile": "flow-nodes-base.convertToFile",
	"n8n-nodes-base.moveBinaryData":  "flow-nodes-base.convertToFile",
	"n8n-nodes-base.readBinaryFile":  "flow-nodes-base.readWriteFile",
	"n8n-nodes-base.readBinaryFiles": "flow-nodes-base.readWriteFile",
	"n8n-nodes-base.writeBinaryFile": "flow-nodes-base.readWriteFile",
	"n8n-nodes-base.readPdf":         "flow-nodes-base.extractFromFile",
	"n8n-nodes-base.htmlExtract":     "flow-nodes-base.html",
	"n8n-nodes-base.transform":       "flow-nodes-base.set",
	"n8n-nodes-base.noop":            "flow-nodes-base.noOp",

	"n8n-nodes-langchain.chatTrigger": "flow-nodes-base.trigger",
	"n8n-nodes-langchain.mcpTrigger":  "flow-nodes-base.trigger",
}

// TranslateNodeType converts an n8n node type string to its flow-native equivalent.
// Types already using the flow-nodes-base prefix are returned as-is.
func TranslateNodeType(n8nType string) string {
	if strings.HasPrefix(n8nType, "flow-nodes-base.") {
		return n8nType
	}

	cleaned := strings.TrimPrefix(n8nType, "@n8n/")

	if flowType, ok := n8nToFlow[cleaned]; ok {
		return flowType
	}

	if strings.HasPrefix(cleaned, "n8n-nodes-base.") {
		return strings.Replace(cleaned, "n8n-nodes-base.", "flow-nodes-base.", 1)
	}
	if strings.HasPrefix(cleaned, "n8n-nodes-langchain.") {
		return strings.Replace(cleaned, "n8n-nodes-langchain.", "flow-nodes-base.", 1)
	}

	return cleaned
}

// ParseWorkflow parses raw n8n workflow JSON bytes into a WorkflowDefinition.
func ParseWorkflow(data []byte) (*models.WorkflowDefinition, error) {
	var raw n8nWorkflowJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow JSON: %w", err)
	}

	if len(raw.Nodes) == 0 {
		return nil, fmt.Errorf("workflow has no nodes")
	}

	for i := range raw.Nodes {
		raw.Nodes[i].Type = TranslateNodeType(raw.Nodes[i].Type)
	}

	triggerCount := 0
	for _, n := range raw.Nodes {
		if n.Type == "flow-nodes-base.trigger" {
			triggerCount++
		}
	}
	if triggerCount == 0 {
		return nil, fmt.Errorf("workflow must have exactly one trigger node (found 0)")
	}
	if triggerCount > 1 {
		return nil, fmt.Errorf("workflow must have exactly one trigger node (found %d)", triggerCount)
	}

	nodeNames := make(map[string]bool, len(raw.Nodes))
	for _, n := range raw.Nodes {
		nodeNames[n.Name] = true
	}

	var connections []models.ConnectionDef

	for sourceName, connData := range raw.Connections {
		if !nodeNames[sourceName] {
			continue
		}

		for outputIndex, rawTargets := range connData.Main {
			var targets []connectionTarget
			if err := json.Unmarshal(rawTargets, &targets); err != nil {
				continue
			}

			for _, t := range targets {
				if t.Node == "" {
					continue
				}
				if !nodeNames[t.Node] {
					continue
				}
				connections = append(connections, models.ConnectionDef{
					SourceNode:        sourceName,
					SourceOutputIndex: outputIndex,
					TargetNode:        t.Node,
					TargetInputIndex:  t.Index,
				})
			}
		}
	}

	return &models.WorkflowDefinition{
		Name:        raw.Name,
		Nodes:       raw.Nodes,
		Connections: connections,
		Settings:    raw.Settings,
	}, nil
}

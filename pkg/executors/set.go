package executors

import (
	"context"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// SetExecutor adds, modifies, or removes fields on each input item.
// Supports n8n v1/v2 (parameters.values.{string,number,boolean}) and v3
// (parameters.assignments.assignments / parameters.fields.values) layouts.
type SetExecutor struct{}

func (e *SetExecutor) Execute(_ context.Context, node models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	var inputItems []models.Item
	for _, input := range inputs {
		inputItems = append(inputItems, input...)
	}

	assignments := getAssignments(node.Parameters)

	var result []models.Item
	for _, item := range inputItems {
		newItem := copyItem(item)
		for _, a := range assignments {
			newItem[a.name] = a.value
		}
		result = append(result, newItem)
	}

	return map[int][]models.Item{0: result}, nil
}

type assignment struct {
	name  string
	value any
}

func getAssignments(params map[string]any) []assignment {
	var result []assignment

	if assignmentsObj, ok := params["assignments"].(map[string]any); ok {
		if list, ok := assignmentsObj["assignments"].([]any); ok {
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					name, _ := m["name"].(string)
					value := m["value"]
					if name != "" {
						result = append(result, assignment{name: name, value: value})
					}
				}
			}
			return result
		}
	}

	if fieldsObj, ok := params["fields"].(map[string]any); ok {
		if list, ok := fieldsObj["values"].([]any); ok {
			for _, item := range list {
				if m, ok := item.(map[string]any); ok {
					name, _ := m["name"].(string)
					if name == "" {
						continue
					}
					for _, vKey := range []string{"stringValue", "numberValue", "booleanValue", "value"} {
						if v, exists := m[vKey]; exists {
							result = append(result, assignment{name: name, value: v})
							break
						}
					}
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}

	if values, ok := params["values"].(map[string]any); ok {
		for _, typeName := range []string{"string", "number", "boolean"} {
			if list, ok := values[typeName].([]any); ok {
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						name, _ := m["name"].(string)
						value := m["value"]
						if name != "" {
							result = append(result, assignment{name: name, value: value})
						}
					}
				}
			}
		}
	}

	return result
}

func copyItem(item models.Item) models.Item {
	cp := make(models.Item, len(item))
	for k, v := range item {
		cp[k] = v
	}
	return cp
}

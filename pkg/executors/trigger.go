package executors

import (
	"context"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// TriggerExecutor is a universal trigger node that passes through the trigger data
// injected by the runner. All n8n trigger types are mapped to this single executor.
type TriggerExecutor struct{}

func (e *TriggerExecutor) Execute(_ context.Context, _ models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	var items []models.Item
	for _, input := range inputs {
		items = append(items, input...)
	}
	if len(items) == 0 {
		items = []models.Item{{}}
	}
	return map[int][]models.Item{0: items}, nil
}

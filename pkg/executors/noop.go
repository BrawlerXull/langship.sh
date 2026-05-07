package executors

import (
	"context"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// NoOpExecutor passes input items through unchanged.
type NoOpExecutor struct{}

func (e *NoOpExecutor) Execute(_ context.Context, _ models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	var items []models.Item
	for _, input := range inputs {
		items = append(items, input...)
	}
	return map[int][]models.Item{0: items}, nil
}

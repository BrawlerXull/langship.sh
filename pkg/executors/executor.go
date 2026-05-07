package executors

import (
	"context"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// NodeExecutor is the interface that all node type executors implement.
type NodeExecutor interface {
	// Execute runs the node logic.
	// inputs[i] holds the items received on input port i.
	// Returns map[outputIndex]items. Single-output nodes return {0: items}.
	// Branching nodes (If, Switch) route items to multiple output indices.
	Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, execCtx *engine.ExecutionContext) (map[int][]models.Item, error)
}

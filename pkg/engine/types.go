package engine

import (
	"context"

	"github.com/lyzrai/flow/pkg/models"
)

// NodeExecutorFunc is the signature for executing a single node.
// Each registered node type provides one of these.
type NodeExecutorFunc func(ctx context.Context, node models.NodeDef, inputs [][]models.Item, execCtx *ExecutionContext) (map[int][]models.Item, error)

// ExecutorLookup resolves a node type string to a node executor function.
// The runner uses this to dispatch each node to its registered handler.
type ExecutorLookup func(nodeType string) (NodeExecutorFunc, error)

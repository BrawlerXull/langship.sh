package executors

import (
	"context"
	"fmt"
	"sync"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

var (
	registry   = map[string]NodeExecutor{}
	registryMu sync.RWMutex
)

// Register adds a node executor for a given flow node type.
func Register(nodeType string, executor NodeExecutor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[nodeType] = executor
}

// Get returns the executor for a flow-native node type.
func Get(nodeType string) (NodeExecutor, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if e, ok := registry[nodeType]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("executor not implemented for node type %q", nodeType)
}

// RegistryDeps holds optional dependencies for executors that need external access.
type RegistryDeps struct {
	WorkflowLoader WorkflowLoaderFunc
}

// WorkflowLoaderFunc loads a workflow definition by ID from storage.
type WorkflowLoaderFunc func(ctx context.Context, id string) (*models.WorkflowDefinition, error)

// RegisterAll registers the v0.1 primitive executors.
func RegisterAll(deps ...RegistryDeps) {
	// Control flow / data primitives
	Register("flow-nodes-base.trigger", &TriggerExecutor{})
	Register("flow-nodes-base.noOp", &NoOpExecutor{})
	Register("flow-nodes-base.set", &SetExecutor{})

	// Human-in-the-loop
	Register("flow-nodes-base.waitForApproval", &ApprovalExecutor{})
}

// BuildLookup creates an ExecutorLookup from the registered executors.
func BuildLookup() engine.ExecutorLookup {
	return func(nodeType string) (engine.NodeExecutorFunc, error) {
		exec, err := Get(nodeType)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, node models.NodeDef, inputs [][]models.Item, execCtx *engine.ExecutionContext) (map[int][]models.Item, error) {
			return exec.Execute(ctx, node, inputs, execCtx)
		}, nil
	}
}

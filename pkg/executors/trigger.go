package executors

import (
	"context"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
)

// TriggerExecutor is a universal trigger node that passes through the trigger data
// injected by the runner. All n8n trigger types are mapped to this single executor.
//
// In addition, it stamps `fromBranch` and `toBranch` from the node's parameters
// onto every output item (without overwriting incoming values from a webhook
// payload). Downstream nodes — Build (for clone) and Promote (for the PR) —
// read those keys, so the Trigger node is the single place to change the
// branch pair for a pipeline.
type TriggerExecutor struct{}

func (e *TriggerExecutor) Execute(_ context.Context, node models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	var items []models.Item
	for _, input := range inputs {
		items = append(items, input...)
	}
	if len(items) == 0 {
		items = []models.Item{{}}
	}

	fromBranch := strParam(node.Parameters, "fromBranch", "")
	toBranch := strParam(node.Parameters, "toBranch", "")

	for i, it := range items {
		if it == nil {
			it = models.Item{}
		}
		if fromBranch != "" {
			if _, ok := it["fromBranch"]; !ok {
				it["fromBranch"] = fromBranch
			}
		}
		if toBranch != "" {
			if _, ok := it["toBranch"]; !ok {
				it["toBranch"] = toBranch
			}
		}
		items[i] = it
	}
	return map[int][]models.Item{0: items}, nil
}

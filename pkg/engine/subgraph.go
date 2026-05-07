package engine

import (
	"fmt"
	"sort"
)

// LoopBody holds the body node names in topological order for inline execution.
type LoopBody struct {
	Order         []string
	Nodes         map[string]bool
	TerminalNodes []string
}

// GetLoopBody identifies the loop body nodes and returns them in topological
// order for inline execution within the parent ExecutionContext.
func GetLoopBody(dag *DAG, loopNodeName string) (*LoopBody, error) {
	var entryNodes []string
	for _, edge := range dag.Adjacency[loopNodeName] {
		if edge.SourceOutputIndex == 0 {
			entryNodes = append(entryNodes, edge.Target)
		}
	}
	if len(entryNodes) == 0 {
		return nil, fmt.Errorf("loop node %q has no body (no output 0 connections)", loopNodeName)
	}

	continuationNodes := make(map[string]bool)
	for _, edge := range dag.Adjacency[loopNodeName] {
		if edge.SourceOutputIndex == 1 {
			continuationNodes[edge.Target] = true
		}
	}

	bodyNodes := make(map[string]bool)
	queue := make([]string, len(entryNodes))
	copy(queue, entryNodes)
	for _, n := range entryNodes {
		bodyNodes[n] = true
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range dag.Adjacency[current] {
			next := edge.Target
			if next == loopNodeName || continuationNodes[next] || bodyNodes[next] {
				continue
			}
			bodyNodes[next] = true
			queue = append(queue, next)
		}
	}

	inDegree := make(map[string]int, len(bodyNodes))
	for name := range bodyNodes {
		inDegree[name] = 0
	}
	for name := range bodyNodes {
		for _, edge := range dag.Adjacency[name] {
			if bodyNodes[edge.Target] {
				inDegree[edge.Target]++
			}
		}
	}

	var topoQueue []string
	for name, deg := range inDegree {
		if deg == 0 {
			topoQueue = append(topoQueue, name)
		}
	}
	sort.Strings(topoQueue)

	var order []string
	for len(topoQueue) > 0 {
		current := topoQueue[0]
		topoQueue = topoQueue[1:]
		order = append(order, current)
		for _, edge := range dag.Adjacency[current] {
			if bodyNodes[edge.Target] {
				inDegree[edge.Target]--
				if inDegree[edge.Target] == 0 {
					topoQueue = append(topoQueue, edge.Target)
				}
			}
		}
	}

	var terminalNodes []string
	for name := range bodyNodes {
		isTerminal := true
		for _, edge := range dag.Adjacency[name] {
			if bodyNodes[edge.Target] {
				isTerminal = false
				break
			}
		}
		if isTerminal {
			terminalNodes = append(terminalNodes, name)
		}
	}
	sort.Strings(terminalNodes)

	return &LoopBody{
		Order:         order,
		Nodes:         bodyNodes,
		TerminalNodes: terminalNodes,
	}, nil
}

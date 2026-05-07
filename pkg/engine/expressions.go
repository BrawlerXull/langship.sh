package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/dop251/goja"

	"github.com/lyzrai/flow/pkg/models"
)

var (
	exprPattern = regexp.MustCompile(`\{\{\s*(.*?)\s*\}\}`)

	jsRefDollarParen = regexp.MustCompile(`\$\(['"](.*?)['"]\)(?:\.item)?\.json(?:\.([a-zA-Z0-9_.]+))?`)
	jsRefDollarNode  = regexp.MustCompile(`\$node\[['"]([^'"]+)['"]\](?:\.json(?:\.([a-zA-Z0-9_.]+))?)?`)
	jsRefDollarJSON  = regexp.MustCompile(`\$json(?:\.([a-zA-Z0-9_.]+))?`)

	pureRefDollarParen = regexp.MustCompile(`^\$\(['"](.*?)['"]\)(?:\.item)?\.json(?:\.[a-zA-Z0-9_.]+)?$`)
	pureRefDollarNode  = regexp.MustCompile(`^\$node\[['"].*?['"]\](?:\.json(?:\.[a-zA-Z0-9_.]+)?)?$`)
	pureRefDollarJSON  = regexp.MustCompile(`^\$json(?:\.[a-zA-Z0-9_.]+)?$`)
)

// ResolveExpressions recursively walks a parameters map and resolves any
// {{ ... }} expressions found in string values.
func ResolveExpressions(params map[string]any, ctx *ExecutionContext, currentNode string) map[string]any {
	result := make(map[string]any, len(params))
	for k, v := range params {
		result[k] = resolveValue(v, ctx, currentNode)
	}
	return result
}

func resolveValue(v any, ctx *ExecutionContext, currentNode string) any {
	switch val := v.(type) {
	case string:
		return resolveString(val, ctx, currentNode)
	case map[string]any:
		resolved := make(map[string]any, len(val))
		for k, inner := range val {
			resolved[k] = resolveValue(inner, ctx, currentNode)
		}
		return resolved
	case []any:
		resolved := make([]any, len(val))
		for i, inner := range val {
			resolved[i] = resolveValue(inner, ctx, currentNode)
		}
		return resolved
	default:
		return v
	}
}

func resolveString(s string, ctx *ExecutionContext, currentNode string) any {
	s = strings.TrimPrefix(s, "=")

	if matches := exprPattern.FindAllStringIndex(s, -1); len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(s) {
		inner := exprPattern.FindStringSubmatch(s)[1]
		return evaluateExpression(inner, ctx, currentNode)
	}

	return exprPattern.ReplaceAllStringFunc(s, func(match string) string {
		inner := exprPattern.FindStringSubmatch(match)[1]
		result := evaluateExpression(inner, ctx, currentNode)
		return fmt.Sprintf("%v", result)
	})
}

// evaluateExpression evaluates a single expression against the execution context.
//
// Supported forms:
//
//	$json.field.path              → current node's first input item
//	$json                         → entire first input item
//	$input.item.json.field        → same as $json.field
//	$node["Name"].json.field      → output of a named node
//	$('Name').json.field          → n8n shorthand for $node["Name"]
func evaluateExpression(expr string, ctx *ExecutionContext, currentNode string) any {
	expr = strings.TrimSpace(expr)

	if strings.HasPrefix(expr, "$json") {
		if pureRefDollarJSON.MatchString(expr) {
			if expr == "$json" {
				return getCurrentInputItem(ctx, currentNode)
			}
			fieldPath := expr[len("$json."):]
			return resolveFromInput(ctx, currentNode, fieldPath)
		}
		return evalJSExpression(expr, ctx, currentNode)
	}

	if strings.HasPrefix(expr, "$input.item.json.") {
		fieldPath := expr[len("$input.item.json."):]
		return resolveFromInput(ctx, currentNode, fieldPath)
	}
	if expr == "$input.item.json" {
		return getCurrentInputItem(ctx, currentNode)
	}

	if strings.HasPrefix(expr, "$(") {
		if pureRefDollarParen.MatchString(expr) {
			return resolveDollarParenReference(expr, ctx)
		}
		return evalJSExpression(expr, ctx, currentNode)
	}

	if strings.HasPrefix(expr, "$node[") {
		if pureRefDollarNode.MatchString(expr) {
			return resolveNodeReference(expr, ctx)
		}
		return evalJSExpression(expr, ctx, currentNode)
	}

	return evalJSExpression(expr, ctx, currentNode)
}

func evalJSExpression(expr string, ctx *ExecutionContext, currentNode string) any {
	var currentItem models.Item
	if inputs := ctx.GatherInputs(currentNode); len(inputs) > 0 && len(inputs[0]) > 0 {
		currentItem = inputs[0][0]
	}

	substituted := jsRefDollarParen.ReplaceAllStringFunc(expr, func(match string) string {
		m := jsRefDollarParen.FindStringSubmatch(match)
		nodeName, fieldPath := m[1], m[2]
		items := ctx.GetNodeOutput(nodeName, 0)
		if len(items) == 0 {
			return "null"
		}
		var v any
		if fieldPath == "" {
			v = items[0]
		} else {
			v = traverseField(items[0], fieldPath)
		}
		return toJSONLiteral(v)
	})

	substituted = jsRefDollarNode.ReplaceAllStringFunc(substituted, func(match string) string {
		m := jsRefDollarNode.FindStringSubmatch(match)
		nodeName, fieldPath := m[1], m[2]
		items := ctx.GetNodeOutput(nodeName, 0)
		if len(items) == 0 {
			return "null"
		}
		var v any
		if fieldPath == "" {
			v = items[0]
		} else {
			v = traverseField(items[0], fieldPath)
		}
		return toJSONLiteral(v)
	})

	substituted = jsRefDollarJSON.ReplaceAllStringFunc(substituted, func(match string) string {
		m := jsRefDollarJSON.FindStringSubmatch(match)
		fieldPath := m[1]
		if currentItem == nil {
			return "null"
		}
		var v any
		if fieldPath == "" {
			v = currentItem
		} else {
			v = traverseField(currentItem, fieldPath)
		}
		return toJSONLiteral(v)
	})

	vm := goja.New()
	val, err := vm.RunString(substituted)
	if err != nil {
		return "{{ " + expr + " }}"
	}
	return val.Export()
}

func toJSONLiteral(v any) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
	}
	return string(b)
}

func getCurrentInputItem(ctx *ExecutionContext, currentNode string) any {
	inputs := ctx.GatherInputs(currentNode)
	if len(inputs) > 0 && len(inputs[0]) > 0 {
		return inputs[0][0]
	}
	return nil
}

func resolveFromInput(ctx *ExecutionContext, currentNode string, fieldPath string) any {
	inputs := ctx.GatherInputs(currentNode)
	if len(inputs) == 0 || len(inputs[0]) == 0 {
		return nil
	}
	item := inputs[0][0]
	return traverseField(item, fieldPath)
}

func resolveDollarParenReference(expr string, ctx *ExecutionContext) any {
	nameStart := strings.Index(expr, "(")
	if nameStart == -1 {
		return nil
	}

	rest := expr[nameStart+1:]
	if len(rest) == 0 {
		return nil
	}
	quote := rest[0:1]
	if quote != "'" && quote != `"` {
		return nil
	}

	endQuote := strings.Index(rest[1:], quote)
	if endQuote == -1 {
		return nil
	}
	nodeName := rest[1 : 1+endQuote]

	items := ctx.GetNodeOutput(nodeName, 0)
	if len(items) == 0 {
		return nil
	}

	closeParen := strings.Index(expr, ")")
	if closeParen == -1 || closeParen >= len(expr)-1 {
		return items[0]
	}

	after := expr[closeParen+1:]
	after = strings.TrimPrefix(after, ".item")

	if !strings.HasPrefix(after, ".json") {
		return items[0]
	}
	after = after[len(".json"):]

	if after == "" {
		return items[0]
	}
	if strings.HasPrefix(after, ".") {
		fieldPath := after[1:]
		return traverseField(items[0], fieldPath)
	}

	return items[0]
}

func resolveNodeReference(expr string, ctx *ExecutionContext) any {
	start := strings.Index(expr, `"`)
	if start == -1 {
		start = strings.Index(expr, `'`)
	}
	if start == -1 {
		return nil
	}

	quote := expr[start : start+1]
	end := strings.Index(expr[start+1:], quote)
	if end == -1 {
		return nil
	}
	nodeName := expr[start+1 : start+1+end]

	items := ctx.GetNodeOutput(nodeName, 0)
	if len(items) == 0 {
		return nil
	}

	rest := expr[start+1+end:]
	jsonIdx := strings.Index(rest, ".json")
	if jsonIdx == -1 {
		return items[0]
	}

	after := rest[jsonIdx+len(".json"):]
	if after == "" || after == "." {
		return items[0]
	}
	if strings.HasPrefix(after, ".") {
		fieldPath := after[1:]
		return traverseField(items[0], fieldPath)
	}

	return items[0]
}

func traverseField(item models.Item, fieldPath string) any {
	if fieldPath == "" {
		return item
	}

	parts := strings.Split(fieldPath, ".")
	var current any = item

	for _, part := range parts {
		switch m := current.(type) {
		case map[string]any:
			current = m[part]
		default:
			return nil
		}
	}

	return current
}

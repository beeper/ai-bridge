package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/calc"
	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

// Calculator is the calculator tool definition.
var Calculator = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.CalculatorName,
		Description: toolspec.CalculatorDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Calculator"},
		InputSchema: toolspec.CalculatorSchema(),
	},
	Type:    ToolTypeBuiltin,
	Group:   GroupCalc,
	Execute: executeCalculator,
}

// executeCalculator evaluates a simple arithmetic expression.
func executeCalculator(ctx context.Context, args map[string]any) (*Result, error) {
	expr, err := ReadString(args, "expression", true)
	if err != nil {
		return ErrorResult("calculator", err.Error()), nil
	}

	result, err := calc.EvalExpression(expr)
	if err != nil {
		return ErrorResult("calculator", fmt.Sprintf("calculation error: %v", err)), nil
	}

	return JSONResult(map[string]any{
		"expression": expr,
		"result":     result,
		"formatted":  fmt.Sprintf("%.6g", result),
	}), nil
}

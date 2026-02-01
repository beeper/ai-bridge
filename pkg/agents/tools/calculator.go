package tools

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Calculator is the calculator tool definition.
var Calculator = &Tool{
	Tool: mcp.Tool{
		Name:        "calculator",
		Description: "Perform basic arithmetic calculations. Supports addition, subtraction, multiplication, division, and modulo operations.",
		Annotations: &mcp.ToolAnnotations{Title: "Calculator"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "A mathematical expression to evaluate, e.g. '2 + 3 * 4' or '100 / 5'",
				},
			},
			"required": []string{"expression"},
		},
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

	result, err := evalExpression(expr)
	if err != nil {
		return ErrorResult("calculator", fmt.Sprintf("calculation error: %v", err)), nil
	}

	return JSONResult(map[string]any{
		"expression": expr,
		"result":     result,
		"formatted":  fmt.Sprintf("%.6g", result),
	}), nil
}

// evalExpression evaluates a simple arithmetic expression.
// Supports: +, -, *, /, %, and parentheses.
func evalExpression(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	// Simple recursive descent parser for basic arithmetic
	pos := 0
	return parseExpression(expr, &pos)
}

func parseExpression(expr string, pos *int) (float64, error) {
	result, err := parseTerm(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) {
		op := expr[*pos]
		if op != '+' && op != '-' {
			break
		}
		*pos++
		right, err := parseTerm(expr, pos)
		if err != nil {
			return 0, err
		}
		if op == '+' {
			result += right
		} else {
			result -= right
		}
	}
	return result, nil
}

func parseTerm(expr string, pos *int) (float64, error) {
	result, err := parseFactor(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) {
		op := expr[*pos]
		if op != '*' && op != '/' && op != '%' {
			break
		}
		*pos++
		right, err := parseFactor(expr, pos)
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			result *= right
		case '/':
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			result /= right
		case '%':
			if right == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			result = math.Mod(result, right)
		}
	}
	return result, nil
}

func parseFactor(expr string, pos *int) (float64, error) {
	if *pos >= len(expr) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	// Handle parentheses
	if expr[*pos] == '(' {
		*pos++
		result, err := parseExpression(expr, pos)
		if err != nil {
			return 0, err
		}
		if *pos >= len(expr) || expr[*pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		*pos++
		return result, nil
	}

	// Handle negative numbers
	negative := false
	if expr[*pos] == '-' {
		negative = true
		*pos++
	}

	// Parse number
	start := *pos
	for *pos < len(expr) && (isDigit(expr[*pos]) || expr[*pos] == '.') {
		*pos++
	}

	if start == *pos {
		return 0, fmt.Errorf("expected number at position %d", start)
	}

	num, err := strconv.ParseFloat(expr[start:*pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", expr[start:*pos])
	}

	if negative {
		num = -num
	}
	return num, nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

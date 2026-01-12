package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ToolDefinition defines a tool that can be used by the AI
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(ctx context.Context, args map[string]any) (string, error)
}

// BuiltinTools returns the list of available builtin tools
func BuiltinTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "calculator",
			Description: "Perform basic arithmetic calculations. Supports addition, subtraction, multiplication, division, and modulo operations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{
						"type":        "string",
						"description": "A mathematical expression to evaluate, e.g. '2 + 3 * 4' or '100 / 5'",
					},
				},
				"required": []string{"expression"},
			},
			Execute: executeCalculator,
		},
		{
			Name:        "web_search",
			Description: "Search the web for information. Returns a summary of search results.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query",
					},
				},
				"required": []string{"query"},
			},
			Execute: executeWebSearch,
		},
	}
}

// executeCalculator evaluates a simple arithmetic expression
func executeCalculator(ctx context.Context, args map[string]any) (string, error) {
	expr, ok := args["expression"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'expression' argument")
	}

	result, err := evalExpression(expr)
	if err != nil {
		return "", fmt.Errorf("calculation error: %w", err)
	}

	return fmt.Sprintf("%.6g", result), nil
}

// evalExpression evaluates a simple arithmetic expression
// Supports: +, -, *, /, %, and parentheses
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

// executeWebSearch performs a web search (placeholder implementation)
func executeWebSearch(ctx context.Context, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'query' argument")
	}

	// Use DuckDuckGo instant answer API (no API key required)
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web search failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Abstract     string `json:"Abstract"`
		AbstractText string `json:"AbstractText"`
		Answer       string `json:"Answer"`
		AnswerType   string `json:"AnswerType"`
		Definition   string `json:"Definition"`
		Heading      string `json:"Heading"`
		RelatedTopics []struct {
			Text string `json:"Text"`
		} `json:"RelatedTopics"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse search results: %w", err)
	}

	// Build response from available data
	var response strings.Builder
	response.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	if result.Answer != "" {
		response.WriteString(fmt.Sprintf("Answer: %s\n", result.Answer))
	}
	if result.AbstractText != "" {
		response.WriteString(fmt.Sprintf("Summary: %s\n", result.AbstractText))
	}
	if result.Definition != "" {
		response.WriteString(fmt.Sprintf("Definition: %s\n", result.Definition))
	}

	// Add related topics if no direct answer
	if result.Answer == "" && result.AbstractText == "" && len(result.RelatedTopics) > 0 {
		response.WriteString("Related information:\n")
		for i, topic := range result.RelatedTopics {
			if i >= 3 || topic.Text == "" {
				break
			}
			response.WriteString(fmt.Sprintf("- %s\n", topic.Text))
		}
	}

	if response.Len() == len(fmt.Sprintf("Search results for: %s\n\n", query)) {
		return fmt.Sprintf("No direct results found for '%s'. Try rephrasing your query.", query), nil
	}

	return response.String(), nil
}

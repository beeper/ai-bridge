package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
)

// ToolDefinition defines a tool that can be used by the AI
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(ctx context.Context, args map[string]any) (string, error)
}

// BridgeToolContext provides bridge-specific context for tool execution
type BridgeToolContext struct {
	Client *AIClient
	Portal *bridgev2.Portal
	Meta   *PortalMetadata
}

// bridgeToolContextKey is the context key for BridgeToolContext
type bridgeToolContextKey struct{}

// WithBridgeToolContext adds bridge context to a context
func WithBridgeToolContext(ctx context.Context, btc *BridgeToolContext) context.Context {
	return context.WithValue(ctx, bridgeToolContextKey{}, btc)
}

// GetBridgeToolContext retrieves bridge context from a context
func GetBridgeToolContext(ctx context.Context) *BridgeToolContext {
	if v := ctx.Value(bridgeToolContextKey{}); v != nil {
		return v.(*BridgeToolContext)
	}
	return nil
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
		{
			Name:        ToolNameSetChatInfo,
			Description: "Patch the current chat's title and/or description (omit fields to keep them unchanged).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "Optional. The new title for the chat",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional. The new description/topic for the chat (empty string clears it)",
					},
				},
				"minProperties":        1,
				"additionalProperties": false,
			},
			Execute: executeSetChatInfo,
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("web search failed: status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Abstract      string `json:"Abstract"`
		AbstractText  string `json:"AbstractText"`
		Answer        string `json:"Answer"`
		AnswerType    string `json:"AnswerType"`
		Definition    string `json:"Definition"`
		Heading       string `json:"Heading"`
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
		count := 0
		for _, topic := range result.RelatedTopics {
			if topic.Text == "" {
				continue
			}
			response.WriteString(fmt.Sprintf("- %s\n", topic.Text))
			count++
			if count >= 3 {
				break
			}
		}
	}

	if response.Len() == len(fmt.Sprintf("Search results for: %s\n\n", query)) {
		return fmt.Sprintf("No direct results found for '%s'. Try rephrasing your query.", query), nil
	}

	return response.String(), nil
}

// executeSetChatInfo patches the room title and/or description using bridge context.
func executeSetChatInfo(ctx context.Context, args map[string]any) (string, error) {
	rawTitle, hasTitle := args["title"]
	rawDesc, hasDesc := args["description"]
	if !hasTitle && !hasDesc {
		return "", fmt.Errorf("missing 'title' or 'description' argument")
	}

	var title string
	if hasTitle {
		if s, ok := rawTitle.(string); ok {
			title = strings.TrimSpace(s)
		} else {
			return "", fmt.Errorf("invalid 'title' argument")
		}
		if title == "" {
			return "", fmt.Errorf("title cannot be empty")
		}
	}

	var description string
	if hasDesc {
		if s, ok := rawDesc.(string); ok {
			description = strings.TrimSpace(s)
		} else {
			return "", fmt.Errorf("invalid 'description' argument")
		}
	}

	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("bridge context not available")
	}
	if btc.Portal == nil {
		return "", fmt.Errorf("portal not available")
	}

	var updates []string
	if hasTitle {
		if err := btc.Client.setRoomName(ctx, btc.Portal, title); err != nil {
			return "", fmt.Errorf("failed to set room title: %w", err)
		}
		updates = append(updates, fmt.Sprintf("title=%s", title))
	}
	if hasDesc {
		if err := btc.Client.setRoomTopic(ctx, btc.Portal, description); err != nil {
			return "", fmt.Errorf("failed to set room description: %w", err)
		}
		if description == "" {
			updates = append(updates, "description=cleared")
		} else {
			updates = append(updates, fmt.Sprintf("description=%s", description))
		}
	}

	return fmt.Sprintf("Chat info updated: %s", strings.Join(updates, ", ")), nil
}

// GetBuiltinTool returns a builtin tool by name, or nil if not found
func GetBuiltinTool(name string) *ToolDefinition {
	for _, tool := range BuiltinTools() {
		if tool.Name == name {
			return &tool
		}
	}
	return nil
}

// GetEnabledBuiltinTools returns the list of enabled builtin tools based on config
func GetEnabledBuiltinTools(isToolEnabled func(string) bool) []ToolDefinition {
	var enabled []ToolDefinition
	for _, tool := range BuiltinTools() {
		if isToolEnabled(tool.Name) {
			enabled = append(enabled, tool)
		}
	}
	return enabled
}

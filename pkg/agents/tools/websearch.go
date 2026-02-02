package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
	"github.com/beeper/ai-bridge/pkg/shared/websearch"
)

// WebSearch is the web search tool definition.
var WebSearch = &Tool{
	Tool: mcp.Tool{
		Name:        toolspec.WebSearchName,
		Description: toolspec.WebSearchDescription,
		Annotations: &mcp.ToolAnnotations{Title: "Web Search"},
		InputSchema: toolspec.WebSearchSchema(),
	},
	Type:    ToolTypeBuiltin,
	Group:   GroupSearch,
	Execute: executeWebSearch,
}

// executeWebSearch performs a web search using DuckDuckGo.
func executeWebSearch(ctx context.Context, args map[string]any) (*Result, error) {
	query, err := ReadString(args, "query", true)
	if err != nil {
		return ErrorResult("web_search", err.Error()), nil
	}

	response, err := websearch.DuckDuckGoSearch(ctx, query)
	if err != nil {
		return ErrorResult("web_search", fmt.Sprintf("search failed: %v", err)), nil
	}

	// Build text summary for the model
	var text strings.Builder
	text.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	if response.Answer != "" {
		text.WriteString(fmt.Sprintf("Answer: %s\n", response.Answer))
	}
	if response.Summary != "" {
		text.WriteString(fmt.Sprintf("Summary: %s\n", response.Summary))
	}
	if response.Definition != "" {
		text.WriteString(fmt.Sprintf("Definition: %s\n", response.Definition))
	}

	if len(response.Results) > 0 {
		text.WriteString("\nRelated information:\n")
		for _, result := range response.Results {
			text.WriteString(fmt.Sprintf("- %s\n", result.Snippet))
		}
	}

	if response.NoResults {
		text.WriteString(fmt.Sprintf("No direct results found for '%s'. Try rephrasing your query.", query))
	}

	return &Result{
		Status:  ResultSuccess,
		Content: []ContentBlock{{Type: "text", Text: text.String()}},
		Details: toMap(response),
	}, nil
}

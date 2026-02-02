package tools

import (
	"context"
	"fmt"
	"time"

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

	count, ignoredOptions := websearch.ParseCountAndIgnoredOptions(args)

	start := time.Now()
	response, err := websearch.DuckDuckGoSearch(ctx, query)
	if err != nil {
		return ErrorResult("web_search", fmt.Sprintf("search failed: %v", err)), nil
	}
	tookMs := time.Since(start).Milliseconds()

	payload := websearch.BuildPayload(query, count, tookMs, response, ignoredOptions)
	return JSONResult(payload), nil
}

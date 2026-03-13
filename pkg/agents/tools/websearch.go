package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/beeper/agentremote/pkg/search"
	"github.com/beeper/agentremote/pkg/shared/toolspec"
	"github.com/beeper/agentremote/pkg/shared/websearch"
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

// executeWebSearch performs a web search using the configured providers.
func executeWebSearch(ctx context.Context, args map[string]any) (*Result, error) {
	req, err := websearch.RequestFromArgs(args)
	if err != nil {
		return ErrorResult("web_search", err.Error()), nil
	}

	cfg := search.ApplyEnvDefaults(nil)
	resp, err := search.Search(ctx, req, cfg)
	if err != nil {
		return ErrorResult("web_search", fmt.Sprintf("search failed: %v", err)), nil
	}

	return JSONResult(websearch.PayloadFromResponse(resp)), nil
}

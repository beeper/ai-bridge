package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
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

	count := 5
	if rawCount, ok := args["count"]; ok {
		switch v := rawCount.(type) {
		case float64:
			count = int(v)
		case int:
			count = v
		case int64:
			count = int(v)
		}
	}
	if count < 1 {
		count = 1
	} else if count > 10 {
		count = 10
	}

	var ignoredOptions []string
	if v, _ := args["country"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "country")
	}
	if v, _ := args["search_lang"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "search_lang")
	}
	if v, _ := args["ui_lang"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "ui_lang")
	}
	if v, _ := args["freshness"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "freshness")
	}

	start := time.Now()
	response, err := websearch.DuckDuckGoSearch(ctx, query)
	if err != nil {
		return ErrorResult("web_search", fmt.Sprintf("search failed: %v", err)), nil
	}
	tookMs := time.Since(start).Milliseconds()

	type webSearchResult struct {
		Title       string `json:"title,omitempty"`
		URL         string `json:"url,omitempty"`
		Description string `json:"description,omitempty"`
		Published   string `json:"published,omitempty"`
		SiteName    string `json:"siteName,omitempty"`
	}
	type webSearchPayload struct {
		Query      string            `json:"query"`
		Provider   string            `json:"provider"`
		Count      int               `json:"count"`
		TookMs     int64             `json:"tookMs"`
		Results    []webSearchResult `json:"results,omitempty"`
		Answer     string            `json:"answer,omitempty"`
		Summary    string            `json:"summary,omitempty"`
		Definition string            `json:"definition,omitempty"`
		Warning    string            `json:"warning,omitempty"`
		NoResults  bool              `json:"noResults,omitempty"`
	}

	limit := count
	if limit > len(response.Results) {
		limit = len(response.Results)
	}
	mapped := make([]webSearchResult, 0, limit)
	for _, result := range response.Results[:limit] {
		title := strings.TrimSpace(result.Title)
		description := strings.TrimSpace(result.Snippet)
		if title == "" && description != "" {
			title = description
			description = ""
		}
		if title == "" {
			continue
		}
		mapped = append(mapped, webSearchResult{
			Title:       title,
			URL:         result.URL,
			Description: description,
			SiteName:    resolveSiteName(result.URL),
		})
	}

	payload := webSearchPayload{
		Query:    query,
		Provider: "duckduckgo",
		Count:    len(mapped),
		TookMs:   tookMs,
		Results:  mapped,
	}
	if response.Answer != "" {
		payload.Answer = response.Answer
	}
	if response.Summary != "" {
		payload.Summary = response.Summary
	}
	if response.Definition != "" {
		payload.Definition = response.Definition
	}
	if response.NoResults {
		payload.NoResults = true
	}
	if len(ignoredOptions) > 0 {
		payload.Warning = fmt.Sprintf("Unsupported options ignored: %s", strings.Join(ignoredOptions, ", "))
	}

	return JSONResult(payload), nil
}

func resolveSiteName(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

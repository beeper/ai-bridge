package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WebSearch is the web search tool definition.
var WebSearch = &Tool{
	Tool: mcp.Tool{
		Name:        "web_search",
		Description: "Search the web for information. Returns a summary of search results.",
		Annotations: &mcp.ToolAnnotations{Title: "Web Search"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
				},
			},
			"required": []string{"query"},
		},
	},
	Type:    ToolTypeBuiltin,
	Group:   GroupSearch,
	Execute: executeWebSearch,
}

// SearchResult represents a single search result.
type SearchResult struct {
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// SearchResponse represents the search response.
type SearchResponse struct {
	Query      string         `json:"query"`
	Answer     string         `json:"answer,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	Definition string         `json:"definition,omitempty"`
	Results    []SearchResult `json:"results,omitempty"`
	NoResults  bool           `json:"no_results,omitempty"`
}

// executeWebSearch performs a web search using DuckDuckGo.
func executeWebSearch(ctx context.Context, args map[string]any) (*Result, error) {
	query, err := ReadString(args, "query", true)
	if err != nil {
		return ErrorResult("web_search", err.Error()), nil
	}

	response, err := performDuckDuckGoSearch(ctx, query)
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

// performDuckDuckGoSearch performs a search using DuckDuckGo instant answer API.
func performDuckDuckGoSearch(ctx context.Context, query string) (*SearchResponse, error) {
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var ddgResult struct {
		Abstract      string `json:"Abstract"`
		AbstractText  string `json:"AbstractText"`
		Answer        string `json:"Answer"`
		AnswerType    string `json:"AnswerType"`
		Definition    string `json:"Definition"`
		Heading       string `json:"Heading"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ddgResult); err != nil {
		return nil, fmt.Errorf("failed to parse results: %w", err)
	}

	response := &SearchResponse{
		Query:      query,
		Answer:     ddgResult.Answer,
		Summary:    ddgResult.AbstractText,
		Definition: ddgResult.Definition,
	}

	// Convert related topics to results
	for i, topic := range ddgResult.RelatedTopics {
		if topic.Text == "" {
			continue
		}
		response.Results = append(response.Results, SearchResult{
			Snippet: topic.Text,
			URL:     topic.FirstURL,
		})
		if i >= 2 { // Limit to 3 results
			break
		}
	}

	// Check if we got any meaningful results
	if response.Answer == "" && response.Summary == "" && response.Definition == "" && len(response.Results) == 0 {
		response.NoResults = true
	}

	return response, nil
}

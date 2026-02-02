package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

// PayloadResult represents a normalized search result for tool output.
type PayloadResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	Published   string `json:"published,omitempty"`
	SiteName    string `json:"siteName,omitempty"`
}

// Payload represents the normalized web_search tool response payload.
type Payload struct {
	Query      string          `json:"query"`
	Provider   string          `json:"provider"`
	Count      int             `json:"count"`
	TookMs     int64           `json:"tookMs"`
	Results    []PayloadResult `json:"results,omitempty"`
	Answer     string          `json:"answer,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	Definition string          `json:"definition,omitempty"`
	Warning    string          `json:"warning,omitempty"`
	NoResults  bool            `json:"noResults,omitempty"`
}

// DuckDuckGoSearch performs a search using the DuckDuckGo instant answer API.
func DuckDuckGoSearch(ctx context.Context, query string) (*SearchResponse, error) {
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

	type ddgTopic struct {
		Text     string     `json:"Text"`
		FirstURL string     `json:"FirstURL"`
		Topics   []ddgTopic `json:"Topics"`
	}
	var ddgResult struct {
		Abstract      string     `json:"Abstract"`
		AbstractText  string     `json:"AbstractText"`
		Answer        string     `json:"Answer"`
		AnswerType    string     `json:"AnswerType"`
		Definition    string     `json:"Definition"`
		Heading       string     `json:"Heading"`
		RelatedTopics []ddgTopic `json:"RelatedTopics"`
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

	// Convert related topics to results.
	var appendTopic func(topic ddgTopic)
	appendTopic = func(topic ddgTopic) {
		if topic.Text != "" {
			title, snippet := splitTopicText(topic.Text)
			response.Results = append(response.Results, SearchResult{
				Title:   title,
				Snippet: snippet,
				URL:     topic.FirstURL,
			})
		}
		for _, child := range topic.Topics {
			appendTopic(child)
		}
	}
	for _, topic := range ddgResult.RelatedTopics {
		appendTopic(topic)
	}

	// Check if we got any meaningful results.
	if response.Answer == "" && response.Summary == "" && response.Definition == "" && len(response.Results) == 0 {
		response.NoResults = true
	}

	return response, nil
}

func splitTopicText(text string) (title string, snippet string) {
	parts := strings.SplitN(text, " - ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(text), ""
}

// ParseCountAndIgnoredOptions extracts count and unsupported option warnings from args.
func ParseCountAndIgnoredOptions(args map[string]any) (int, []string) {
	count := 5
	if args != nil {
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

	return count, ignoredOptions
}

// BuildPayload maps a search response to the web_search tool payload.
func BuildPayload(query string, count int, tookMs int64, response *SearchResponse, ignoredOptions []string) Payload {
	payload := Payload{
		Query:    query,
		Provider: "duckduckgo",
		TookMs:   tookMs,
	}
	if response == nil {
		if len(ignoredOptions) > 0 {
			payload.Warning = fmt.Sprintf("Unsupported options ignored: %s", strings.Join(ignoredOptions, ", "))
		}
		return payload
	}

	limit := count
	if limit > len(response.Results) {
		limit = len(response.Results)
	}
	mapped := make([]PayloadResult, 0, limit)
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
		mapped = append(mapped, PayloadResult{
			Title:       title,
			URL:         result.URL,
			Description: description,
			SiteName:    ResolveSiteName(result.URL),
		})
	}

	payload.Count = len(mapped)
	payload.Results = mapped
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

	return payload
}

// ResolveSiteName extracts a hostname from a URL string.
func ResolveSiteName(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

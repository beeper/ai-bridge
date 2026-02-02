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
		Text     string    `json:"Text"`
		FirstURL string    `json:"FirstURL"`
		Topics   []ddgTopic `json:"Topics"`
	}
	var ddgResult struct {
		Abstract      string    `json:"Abstract"`
		AbstractText  string    `json:"AbstractText"`
		Answer        string    `json:"Answer"`
		AnswerType    string    `json:"AnswerType"`
		Definition    string    `json:"Definition"`
		Heading       string    `json:"Heading"`
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

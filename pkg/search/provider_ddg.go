package search

import (
	"context"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/shared/websearch"
)

type ddgProvider struct{}

func newDDGProvider(cfg *Config) Provider {
	if cfg == nil {
		return nil
	}
	if !isEnabled(cfg.DDG.Enabled, true) {
		return nil
	}
	return &ddgProvider{}
}

func (p *ddgProvider) Name() string {
	return ProviderDuckDuckGo
}

func (p *ddgProvider) Search(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()
	result, err := websearch.DuckDuckGoSearch(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	ignored := []string{}
	if req.Country != "" {
		ignored = append(ignored, "country")
	}
	if req.SearchLang != "" {
		ignored = append(ignored, "search_lang")
	}
	if req.UILang != "" {
		ignored = append(ignored, "ui_lang")
	}
	if req.Freshness != "" {
		ignored = append(ignored, "freshness")
	}
	payload := websearch.BuildPayload(req.Query, req.Count, time.Since(start).Milliseconds(), result, ignored)

	results := make([]Result, 0, len(payload.Results))
	for _, entry := range payload.Results {
		results = append(results, Result{
			Title:       strings.TrimSpace(entry.Title),
			URL:         entry.URL,
			Description: strings.TrimSpace(entry.Description),
			Published:   entry.Published,
			SiteName:    entry.SiteName,
		})
	}

	return &Response{
		Query:      payload.Query,
		Provider:   payload.Provider,
		Count:      payload.Count,
		TookMs:     payload.TookMs,
		Results:    results,
		Answer:     payload.Answer,
		Summary:    payload.Summary,
		Definition: payload.Definition,
		Warning:    payload.Warning,
		NoResults:  payload.NoResults,
		Extras:     map[string]any{"legacy": true},
	}, nil
}

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type perplexityProvider struct {
	cfg PerplexityConfig
}

func newPerplexityProvider(cfg *Config) Provider {
	if cfg == nil {
		return nil
	}
	enabled := isEnabled(cfg.Perplexity.Enabled, true)
	if !enabled {
		return nil
	}
	apiKey := strings.TrimSpace(cfg.Perplexity.APIKey)
	if apiKey == "" {
		return nil
	}
	return &perplexityProvider{cfg: cfg.Perplexity}
}

func (p *perplexityProvider) Name() string {
	return ProviderPerplexity
}

func (p *perplexityProvider) Search(ctx context.Context, req Request) (*Response, error) {
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model": p.cfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": req.Query,
			},
		},
	}
	start := time.Now()
	data, _, err := postJSON(ctx, endpoint, map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", p.cfg.APIKey),
	}, payload, p.cfg.TimeoutSecs)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Citations []string `json:"citations"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	answer := ""
	if len(resp.Choices) > 0 {
		answer = strings.TrimSpace(resp.Choices[0].Message.Content)
	}
	results := make([]Result, 0, len(resp.Citations))
	for _, citation := range resp.Citations {
		results = append(results, Result{
			Title:    citation,
			URL:      citation,
			SiteName: resolveSiteName(citation),
		})
	}

	return &Response{
		Query:     req.Query,
		Provider:  ProviderPerplexity,
		Count:     len(results),
		TookMs:    time.Since(start).Milliseconds(),
		Results:   results,
		Answer:    answer,
		NoResults: len(results) == 0 && answer == "",
	}, nil
}

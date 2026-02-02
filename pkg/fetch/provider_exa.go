package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type exaProvider struct {
	cfg ExaConfig
}

func newExaProvider(cfg *Config) Provider {
	if cfg == nil {
		return nil
	}
	if !isEnabled(cfg.Exa.Enabled, true) {
		return nil
	}
	apiKey := strings.TrimSpace(cfg.Exa.APIKey)
	if apiKey == "" {
		return nil
	}
	return &exaProvider{cfg: cfg.Exa}
}

func (p *exaProvider) Name() string {
	return ProviderExa
}

func (p *exaProvider) Fetch(ctx context.Context, req Request) (*Response, error) {
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/contents"
	maxChars := req.MaxChars
	if maxChars <= 0 {
		maxChars = p.cfg.TextMaxCharacters
	}
	payload := map[string]any{
		"urls": []string{req.URL},
		"text": map[string]any{
			"maxCharacters": maxChars,
		},
	}

	start := time.Now()
	data, _, err := postJSON(ctx, endpoint, map[string]string{
		"x-api-key": p.cfg.APIKey,
		"accept":    "application/json",
	}, payload, DefaultTimeoutSecs)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []struct {
			URL         string   `json:"url"`
			Text        string   `json:"text"`
			Summary     string   `json:"summary"`
			Highlights  []string `json:"highlights"`
			Title       string   `json:"title"`
			PublishedAt string   `json:"publishedDate"`
		} `json:"results"`
		Statuses []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  any    `json:"error"`
		} `json:"statuses"`
		CostDollars map[string]any `json:"costDollars"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("exa contents returned no results")
	}
	entry := resp.Results[0]
	text := entry.Text
	if text == "" && len(entry.Highlights) > 0 {
		text = entry.Highlights[0]
	}
	if text == "" {
		text = entry.Summary
	}
	length := len(text)
	return &Response{
		URL:           req.URL,
		FinalURL:      req.URL,
		Status:        200,
		ContentType:   "text/plain",
		ExtractMode:   req.ExtractMode,
		Extractor:     "exa-contents",
		Truncated:     length >= req.MaxChars && req.MaxChars > 0,
		Length:        length,
		RawLength:     length,
		WrappedLength: length,
		FetchedAt:     time.Now().UTC().Format(time.RFC3339),
		TookMs:        time.Since(start).Milliseconds(),
		Text:          text,
		Provider:      ProviderExa,
		Extras: map[string]any{
			"costDollars": resp.CostDollars,
		},
	}, nil
}

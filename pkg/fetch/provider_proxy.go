package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type proxyProvider struct {
	cfg ProxyConfig
}

func newProxyProvider(cfg *Config) Provider {
	if cfg == nil {
		return nil
	}
	if !isEnabled(cfg.Proxy.Enabled, true) {
		return nil
	}
	if strings.TrimSpace(cfg.Proxy.BaseURL) == "" {
		return nil
	}
	return &proxyProvider{cfg: cfg.Proxy}
}

func (p *proxyProvider) Name() string {
	return ProviderProxy
}

func (p *proxyProvider) Fetch(ctx context.Context, req Request) (*Response, error) {
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + p.cfg.ContentsPath
	payload := map[string]any{
		"url":         req.URL,
		"extractMode": req.ExtractMode,
		"maxChars":    req.MaxChars,
	}
	start := time.Now()
	data, _, err := postJSON(ctx, endpoint, proxyHeaders(p.cfg), payload, p.cfg.TimeoutSecs)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("proxy response parse error: %w", err)
	}
	if resp.Provider == "" {
		resp.Provider = ProviderProxy
	}
	resp.TookMs = time.Since(start).Milliseconds()
	return &resp, nil
}

func proxyHeaders(cfg ProxyConfig) map[string]string {
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}
	return headers
}

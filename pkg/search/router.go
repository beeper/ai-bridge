package search

import (
	"context"
	"errors"
	"strings"

	"github.com/beeper/agentremote/pkg/shared/exa"
	"github.com/beeper/agentremote/pkg/shared/providerchain"
	"github.com/beeper/agentremote/pkg/shared/registry"
	"github.com/beeper/agentremote/pkg/shared/stringutil"
)

// Search executes a search using the configured provider chain.
func Search(ctx context.Context, req Request, cfg *Config) (*Response, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, errors.New("missing query")
	}
	cfg = cfg.WithDefaults()
	req = normalizeRequest(req)

	reg := registry.New[Provider]()
	if exa.Enabled(cfg.Exa.Enabled, cfg.Exa.APIKey) {
		reg.Register(&exaProvider{cfg: cfg.Exa})
	}
	order := stringutil.BuildProviderOrder(cfg.Provider, cfg.Fallbacks, DefaultFallbackOrder)

	return providerchain.RunFirst(
		order,
		reg.Get,
		func(provider Provider) (*Response, error) {
			return provider.Search(ctx, req)
		},
		func(name string, resp *Response) {
			if resp.Provider == "" {
				resp.Provider = name
			}
			if resp.Query == "" {
				resp.Query = req.Query
			}
			if resp.Count == 0 {
				resp.Count = len(resp.Results)
			}
		},
		errors.New("no search providers available"),
	)
}

func normalizeRequest(req Request) Request {
	if req.Count <= 0 {
		req.Count = DefaultSearchCount
	}
	if req.Count > MaxSearchCount {
		req.Count = MaxSearchCount
	}
	return req
}

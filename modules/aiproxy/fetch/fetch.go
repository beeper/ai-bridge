package fetch

import (
	"context"
	"slices"
	"strings"

	basefetch "github.com/beeper/ai-bridge/pkg/matrixai/fetch"
)

type Request struct {
	URL         string
	ExtractMode string
	MaxChars    int
}

type Response struct {
	URL           string
	FinalURL      string
	Status        int
	ContentType   string
	ExtractMode   string
	Extractor     string
	Truncated     bool
	Length        int
	RawLength     int
	WrappedLength int
	FetchedAt     string
	TookMs        int64
	Text          string
	Warning       string
	Cached        bool
	Provider      string
	Extras        map[string]any
}

type Config struct {
	Provider  string
	Fallbacks []string

	Exa    ExaConfig
	Direct DirectConfig
}

type ExaConfig struct {
	Enabled           *bool
	BaseURL           string
	APIKey            string
	IncludeText       bool
	TextMaxCharacters int
}

type DirectConfig struct {
	Enabled      *bool
	TimeoutSecs  int
	UserAgent    string
	Readability  bool
	MaxChars     int
	MaxRedirects int
	CacheTtlSecs int
}

const (
	ProviderExa        = basefetch.ProviderExa
	ProviderDirect     = basefetch.ProviderDirect
	DefaultTimeoutSecs = basefetch.DefaultTimeoutSecs
	DefaultMaxChars    = basefetch.DefaultMaxChars
)

var DefaultFallbackOrder = basefetch.DefaultFallbackOrder

func Fetch(ctx context.Context, req Request, cfg *Config) (*Response, error) {
	resp, err := basefetch.Fetch(ctx, toBaseRequest(req), toBaseConfig(cfg))
	if err != nil || resp == nil {
		return nil, err
	}
	converted := fromBaseResponse(*resp)
	return &converted, nil
}

func ApplyEnvDefaults(cfg *Config) *Config {
	return fromBaseConfig(basefetch.ApplyEnvDefaults(toBaseConfig(cfg)))
}

func (c *Config) WithDefaults() *Config {
	if c == nil {
		c = &Config{}
	}
	if strings.TrimSpace(c.Provider) == "" {
		c.Provider = ProviderExa
	}
	if len(c.Fallbacks) == 0 {
		c.Fallbacks = slices.Clone(DefaultFallbackOrder)
	}
	c.Exa = c.Exa.withDefaults()
	c.Direct = c.Direct.withDefaults()
	return c
}

func (c ExaConfig) withDefaults() ExaConfig {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.exa.ai"
	}
	if c.TextMaxCharacters <= 0 {
		c.TextMaxCharacters = 5_000
	}
	return c
}

func (c DirectConfig) withDefaults() DirectConfig {
	if c.TimeoutSecs <= 0 {
		c.TimeoutSecs = DefaultTimeoutSecs
	}
	if c.UserAgent == "" {
		c.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
	}
	if c.MaxChars <= 0 {
		c.MaxChars = DefaultMaxChars
	}
	if c.MaxRedirects <= 0 {
		c.MaxRedirects = 3
	}
	return c
}

func toBaseRequest(req Request) basefetch.Request {
	return basefetch.Request{
		URL:         req.URL,
		ExtractMode: req.ExtractMode,
		MaxChars:    req.MaxChars,
	}
}

func toBaseConfig(cfg *Config) *basefetch.Config {
	if cfg == nil {
		return nil
	}
	return &basefetch.Config{
		Provider:  cfg.Provider,
		Fallbacks: cfg.Fallbacks,
		Exa: basefetch.ExaConfig{
			Enabled:           cfg.Exa.Enabled,
			BaseURL:           cfg.Exa.BaseURL,
			APIKey:            cfg.Exa.APIKey,
			IncludeText:       cfg.Exa.IncludeText,
			TextMaxCharacters: cfg.Exa.TextMaxCharacters,
		},
		Direct: basefetch.DirectConfig{
			Enabled:      cfg.Direct.Enabled,
			TimeoutSecs:  cfg.Direct.TimeoutSecs,
			UserAgent:    cfg.Direct.UserAgent,
			Readability:  cfg.Direct.Readability,
			MaxChars:     cfg.Direct.MaxChars,
			MaxRedirects: cfg.Direct.MaxRedirects,
			CacheTtlSecs: cfg.Direct.CacheTtlSecs,
		},
	}
}

func fromBaseConfig(cfg *basefetch.Config) *Config {
	if cfg == nil {
		return nil
	}
	return &Config{
		Provider:  cfg.Provider,
		Fallbacks: cfg.Fallbacks,
		Exa: ExaConfig{
			Enabled:           cfg.Exa.Enabled,
			BaseURL:           cfg.Exa.BaseURL,
			APIKey:            cfg.Exa.APIKey,
			IncludeText:       cfg.Exa.IncludeText,
			TextMaxCharacters: cfg.Exa.TextMaxCharacters,
		},
		Direct: DirectConfig{
			Enabled:      cfg.Direct.Enabled,
			TimeoutSecs:  cfg.Direct.TimeoutSecs,
			UserAgent:    cfg.Direct.UserAgent,
			Readability:  cfg.Direct.Readability,
			MaxChars:     cfg.Direct.MaxChars,
			MaxRedirects: cfg.Direct.MaxRedirects,
			CacheTtlSecs: cfg.Direct.CacheTtlSecs,
		},
	}
}

func fromBaseResponse(resp basefetch.Response) Response {
	return Response{
		URL:           resp.URL,
		FinalURL:      resp.FinalURL,
		Status:        resp.Status,
		ContentType:   resp.ContentType,
		ExtractMode:   resp.ExtractMode,
		Extractor:     resp.Extractor,
		Truncated:     resp.Truncated,
		Length:        resp.Length,
		RawLength:     resp.RawLength,
		WrappedLength: resp.WrappedLength,
		FetchedAt:     resp.FetchedAt,
		TookMs:        resp.TookMs,
		Text:          resp.Text,
		Warning:       resp.Warning,
		Cached:        resp.Cached,
		Provider:      resp.Provider,
		Extras:        resp.Extras,
	}
}

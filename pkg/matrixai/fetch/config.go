package fetch

import (
	"slices"
	"strings"
)

const (
	ProviderExa        = "exa"
	DefaultTimeoutSecs = 30
	DefaultMaxChars    = 50_000
)

var DefaultFallbackOrder = []string{
	ProviderExa,
}

// Config controls fetch provider selection and credentials.
type Config struct {
	Provider  string   `yaml:"provider"`
	Fallbacks []string `yaml:"fallbacks"`

	Exa ExaConfig `yaml:"exa"`
}

type ExaConfig struct {
	Enabled           *bool  `yaml:"enabled"`
	BaseURL           string `yaml:"base_url"`
	APIKey            string `yaml:"api_key"`
	IncludeText       bool   `yaml:"include_text"`
	TextMaxCharacters int    `yaml:"text_max_chars"`
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

func isEnabled(flag *bool, fallback bool) bool {
	if flag == nil {
		return fallback
	}
	return *flag
}

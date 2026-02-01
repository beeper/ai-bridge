package connector

import (
	_ "embed"
	"time"

	"go.mau.fi/util/configupgrade"
)

//go:embed example-config.yaml
var exampleNetworkConfig string

// Config represents the connector-specific configuration that is nested under
// the `network:` block in the main bridge config.
type Config struct {
	Beeper    BeeperConfig    `yaml:"beeper"`
	Providers ProvidersConfig `yaml:"providers"`
	Bridge    BridgeConfig    `yaml:"bridge"`

	// Global settings
	DefaultSystemPrompt string        `yaml:"default_system_prompt"`
	ModelCacheDuration  time.Duration `yaml:"model_cache_duration"`

	// Context pruning configuration (OpenClaw-style)
	Pruning *PruningConfig `yaml:"pruning"`
}

// BeeperConfig contains Beeper AI proxy credentials for automatic login.
// If both BaseURL and Token are set, users don't need to manually log in.
type BeeperConfig struct {
	BaseURL string `yaml:"base_url"` // Beeper AI proxy endpoint
	Token   string `yaml:"token"`    // Beeper Matrix access token
}

// ProviderConfig holds settings for a specific AI provider.
type ProviderConfig struct {
	DefaultModel     string `yaml:"default_model"`
	DefaultPDFEngine string `yaml:"default_pdf_engine"` // pdf-text, mistral-ocr (default), native
}

// ProvidersConfig contains per-provider configuration.
type ProvidersConfig struct {
	Beeper     ProviderConfig `yaml:"beeper"`
	OpenAI     ProviderConfig `yaml:"openai"`
	OpenRouter ProviderConfig `yaml:"openrouter"`
}

// BridgeConfig tweaks Matrix-side behaviour for the AI bridge.
type BridgeConfig struct {
	CommandPrefix string `yaml:"command_prefix"`
}

func upgradeConfig(helper configupgrade.Helper) {
	// Beeper credentials for auto-login
	helper.Copy(configupgrade.Str, "beeper", "base_url")
	helper.Copy(configupgrade.Str, "beeper", "token")

	// Per-provider default models
	helper.Copy(configupgrade.Str, "providers", "beeper", "default_model")
	helper.Copy(configupgrade.Str, "providers", "beeper", "default_pdf_engine")
	helper.Copy(configupgrade.Str, "providers", "openai", "default_model")
	helper.Copy(configupgrade.Str, "providers", "openrouter", "default_model")
	helper.Copy(configupgrade.Str, "providers", "openrouter", "default_pdf_engine")

	// Global settings
	helper.Copy(configupgrade.Str, "default_system_prompt")
	helper.Copy(configupgrade.Str, "model_cache_duration")

	// Bridge-specific configuration
	helper.Copy(configupgrade.Str, "bridge", "command_prefix")

	// Context pruning configuration
	helper.Copy(configupgrade.Bool, "pruning", "enabled")
	helper.Copy(configupgrade.Float, "pruning", "soft_trim_ratio")
	helper.Copy(configupgrade.Float, "pruning", "hard_clear_ratio")
	helper.Copy(configupgrade.Int, "pruning", "keep_last_assistants")
	helper.Copy(configupgrade.Int, "pruning", "min_prunable_chars")
	helper.Copy(configupgrade.Int, "pruning", "soft_trim_max_chars")
	helper.Copy(configupgrade.Int, "pruning", "soft_trim_head_chars")
	helper.Copy(configupgrade.Int, "pruning", "soft_trim_tail_chars")
	helper.Copy(configupgrade.Bool, "pruning", "hard_clear_enabled")
	helper.Copy(configupgrade.Str, "pruning", "hard_clear_placeholder")
}

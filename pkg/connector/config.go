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
	OpenAI OpenAIConfig `yaml:"openai"`
	Bridge BridgeConfig `yaml:"bridge"`
}

// OpenAIConfig controls how the connector talks to the OpenAI API.
// Per-user credentials (API key, base_url, org_id, project_id) are provided during login, not in config.
type OpenAIConfig struct {
	// Bridge-wide defaults for new rooms (can be overridden per-room via room state)
	DefaultTemperature  float64       `yaml:"default_temperature"`
	MaxContextMessages  int           `yaml:"max_context_messages"`
	MaxCompletionTokens int           `yaml:"max_completion_tokens"`
	SystemPrompt        string        `yaml:"system_prompt"`
	RequestTimeout      time.Duration `yaml:"request_timeout"`

	// Streaming configuration
	EnableStreaming     bool `yaml:"enable_streaming"`
	EditDebounceMs      int  `yaml:"edit_debounce_ms"`      // Debounce time for edits (default: 200ms)
	TransientDebounceMs int  `yaml:"transient_debounce_ms"` // Debounce for transient events (default: 50ms)
}

// BridgeConfig tweaks Matrix-side behaviour for the GPT bridge.
type BridgeConfig struct {
	CommandPrefix       string `yaml:"command_prefix"`
	TypingNotifications bool   `yaml:"typing_notifications"`
	MentionAssistant    bool   `yaml:"mention_assistant"`
}

func upgradeConfig(helper configupgrade.Helper) {
	// Bridge-wide defaults (kept from config)
	helper.Copy(configupgrade.Float, "openai", "default_temperature")
	helper.Copy(configupgrade.Int, "openai", "max_context_messages")
	helper.Copy(configupgrade.Int, "openai", "max_completion_tokens")
	helper.Copy(configupgrade.Str, "openai", "system_prompt")
	helper.Copy(configupgrade.Str, "openai", "request_timeout")

	// Streaming configuration
	helper.Copy(configupgrade.Bool, "openai", "enable_streaming")
	helper.Copy(configupgrade.Int, "openai", "edit_debounce_ms")
	helper.Copy(configupgrade.Int, "openai", "transient_debounce_ms")

	// Bridge-specific configuration
	helper.Copy(configupgrade.Str, "bridge", "command_prefix")
	helper.Copy(configupgrade.Bool, "bridge", "typing_notifications")
	helper.Copy(configupgrade.Bool, "bridge", "mention_assistant")

	// Note: api_key, organization_id, project_id, base_url, and default_model
	// are now per-user (via login flow) and per-room (via room state), not in config
}

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
type OpenAIConfig struct {
	APIKey             string        `yaml:"api_key"`
	OrganizationID     string        `yaml:"organization_id"`
	ProjectID          string        `yaml:"project_id"`
	BaseURL            string        `yaml:"base_url"`
	DefaultModel       string        `yaml:"default_model"`
	DefaultTemperature float64       `yaml:"default_temperature"`
	MaxContextMessages int           `yaml:"max_context_messages"`
	MaxCompletionTokens int          `yaml:"max_completion_tokens"`
	SystemPrompt       string        `yaml:"system_prompt"`
	RequestTimeout     time.Duration `yaml:"request_timeout"`
}

// BridgeConfig tweaks Matrix-side behaviour for the GPT bridge.
type BridgeConfig struct {
	CommandPrefix       string `yaml:"command_prefix"`
	TypingNotifications bool   `yaml:"typing_notifications"`
	MentionAssistant    bool   `yaml:"mention_assistant"`
}

func upgradeConfig(helper configupgrade.Helper) {
	helper.Copy(configupgrade.Str, "openai", "api_key")
	helper.Copy(configupgrade.Str, "openai", "organization_id")
	helper.Copy(configupgrade.Str, "openai", "project_id")
	helper.Copy(configupgrade.Str, "openai", "base_url")
	helper.Copy(configupgrade.Str, "openai", "default_model")
	helper.Copy(configupgrade.Float, "openai", "default_temperature")
	helper.Copy(configupgrade.Int, "openai", "max_context_messages")
	helper.Copy(configupgrade.Int, "openai", "max_completion_tokens")
	helper.Copy(configupgrade.Str, "openai", "system_prompt")
	helper.Copy(configupgrade.Str, "bridge", "command_prefix")
	helper.Copy(configupgrade.Bool, "bridge", "typing_notifications")
	helper.Copy(configupgrade.Bool, "bridge", "mention_assistant")
}

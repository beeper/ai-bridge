package codex

import (
	"time"

	"go.mau.fi/util/configupgrade"
)

const (
	ProviderCodex = "codex"
)

type Config struct {
	Bridge             BridgeConfig   `yaml:"bridge"`
	Codex              *CodexConfig   `yaml:"codex"`
	Owners             []string       `yaml:"owners"`
	ModelCacheDuration time.Duration  `yaml:"model_cache_duration"`
	LinkPreviews       *PreviewConfig `yaml:"link_previews"`
}

type BridgeConfig struct {
	CommandPrefix string `yaml:"command_prefix"`
}

// PreviewConfig is intentionally minimal for the codex bridge split.
type PreviewConfig struct {
	Enabled *bool `yaml:"enabled"`
}

// CodexConfig configures the Codex app-server integration.
type CodexConfig struct {
	Enabled       *bool            `yaml:"enabled"`
	Command       string           `yaml:"command"`
	HomeBaseDir   string           `yaml:"home_base_dir"`
	DefaultModel  string           `yaml:"default_model"`
	NetworkAccess *bool            `yaml:"network_access"`
	ClientInfo    *CodexClientInfo `yaml:"client_info"`
}

type CodexClientInfo struct {
	Name    string `yaml:"name"`
	Title   string `yaml:"title"`
	Version string `yaml:"version"`
}

const exampleNetworkConfig = `
bridge:
  command_prefix: "!ai"
codex:
  enabled: true
  command: "codex"
  default_model: "gpt-5.1-codex"
  network_access: true
  client_info:
    name: "ai_bridge_matrix"
    title: "AI Bridge (Matrix)"
    version: "0.1.0"
`

func upgradeConfig(helper configupgrade.Helper) {
	_ = helper
}


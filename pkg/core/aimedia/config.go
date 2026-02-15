package aimedia

// ScopeMatch defines match criteria for media understanding scope rules.
type ScopeMatch struct {
	Channel   string `yaml:"channel" json:"channel,omitempty"`
	ChatType  string `yaml:"chatType" json:"chatType,omitempty"`
	KeyPrefix string `yaml:"keyPrefix" json:"keyPrefix,omitempty"`
}

// ScopeRule defines a single allow/deny rule.
type ScopeRule struct {
	Action string      `yaml:"action" json:"action,omitempty"`
	Match  *ScopeMatch `yaml:"match" json:"match,omitempty"`
}

// ScopeConfig controls allow/deny gating for media understanding.
type ScopeConfig struct {
	Default string      `yaml:"default" json:"default,omitempty"`
	Rules   []ScopeRule `yaml:"rules" json:"rules,omitempty"`
}

// AttachmentsConfig controls how media attachments are selected.
type AttachmentsConfig struct {
	Mode           string `yaml:"mode" json:"mode,omitempty"`
	MaxAttachments int    `yaml:"maxAttachments" json:"maxAttachments,omitempty"`
	Prefer         string `yaml:"prefer" json:"prefer,omitempty"`
}

// DeepgramConfig is a deprecated Deepgram settings alias.
type DeepgramConfig struct {
	DetectLanguage *bool `yaml:"detectLanguage" json:"detectLanguage,omitempty"`
	Punctuate      *bool `yaml:"punctuate" json:"punctuate,omitempty"`
	SmartFormat    *bool `yaml:"smartFormat" json:"smartFormat,omitempty"`
}

// ModelConfig defines a single media understanding model entry.
type ModelConfig struct {
	Provider         string                    `yaml:"provider" json:"provider,omitempty"`
	Model            string                    `yaml:"model" json:"model,omitempty"`
	Capabilities     []string                  `yaml:"capabilities" json:"capabilities,omitempty"`
	Type             string                    `yaml:"type" json:"type,omitempty"`
	Command          string                    `yaml:"command" json:"command,omitempty"`
	Args             []string                  `yaml:"args" json:"args,omitempty"`
	Prompt           string                    `yaml:"prompt" json:"prompt,omitempty"`
	MaxChars         int                       `yaml:"maxChars" json:"maxChars,omitempty"`
	MaxBytes         int                       `yaml:"maxBytes" json:"maxBytes,omitempty"`
	TimeoutSeconds   int                       `yaml:"timeoutSeconds" json:"timeoutSeconds,omitempty"`
	Language         string                    `yaml:"language" json:"language,omitempty"`
	ProviderOptions  map[string]map[string]any `yaml:"providerOptions" json:"providerOptions,omitempty"`
	Deepgram         *DeepgramConfig           `yaml:"deepgram" json:"deepgram,omitempty"`
	BaseURL          string                    `yaml:"baseUrl" json:"baseUrl,omitempty"`
	Headers          map[string]string         `yaml:"headers" json:"headers,omitempty"`
	Profile          string                    `yaml:"profile" json:"profile,omitempty"`
	PreferredProfile string                    `yaml:"preferredProfile" json:"preferredProfile,omitempty"`
}

// CapabilityConfig defines defaults for media understanding of a single capability.
type CapabilityConfig struct {
	Enabled         *bool                     `yaml:"enabled" json:"enabled,omitempty"`
	Scope           *ScopeConfig              `yaml:"scope" json:"scope,omitempty"`
	MaxBytes        int                       `yaml:"maxBytes" json:"maxBytes,omitempty"`
	MaxChars        int                       `yaml:"maxChars" json:"maxChars,omitempty"`
	Prompt          string                    `yaml:"prompt" json:"prompt,omitempty"`
	TimeoutSeconds  int                       `yaml:"timeoutSeconds" json:"timeoutSeconds,omitempty"`
	Language        string                    `yaml:"language" json:"language,omitempty"`
	ProviderOptions map[string]map[string]any `yaml:"providerOptions" json:"providerOptions,omitempty"`
	Deepgram        *DeepgramConfig           `yaml:"deepgram" json:"deepgram,omitempty"`
	BaseURL         string                    `yaml:"baseUrl" json:"baseUrl,omitempty"`
	Headers         map[string]string         `yaml:"headers" json:"headers,omitempty"`
	Attachments     *AttachmentsConfig        `yaml:"attachments" json:"attachments,omitempty"`
	Models          []ModelConfig             `yaml:"models" json:"models,omitempty"`
}

// ToolsConfig configures media understanding/transcription.
type ToolsConfig struct {
	Models      []ModelConfig     `yaml:"models" json:"models,omitempty"`
	Concurrency int               `yaml:"concurrency" json:"concurrency,omitempty"`
	Image       *CapabilityConfig `yaml:"image" json:"image,omitempty"`
	Audio       *CapabilityConfig `yaml:"audio" json:"audio,omitempty"`
	Video       *CapabilityConfig `yaml:"video" json:"video,omitempty"`
}

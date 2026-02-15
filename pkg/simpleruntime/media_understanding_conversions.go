package connector

import "github.com/beeper/ai-bridge/pkg/core/aimedia"

func toCoreScopeMatch(match *MediaUnderstandingScopeMatch) *aimedia.ScopeMatch {
	if match == nil {
		return nil
	}
	return &aimedia.ScopeMatch{
		Channel:   match.Channel,
		ChatType:  match.ChatType,
		KeyPrefix: match.KeyPrefix,
	}
}

func toCoreScopeRule(rule MediaUnderstandingScopeRule) aimedia.ScopeRule {
	return aimedia.ScopeRule{
		Action: rule.Action,
		Match:  toCoreScopeMatch(rule.Match),
	}
}

func toCoreScopeConfig(scope *MediaUnderstandingScopeConfig) *aimedia.ScopeConfig {
	if scope == nil {
		return nil
	}
	rules := make([]aimedia.ScopeRule, 0, len(scope.Rules))
	for _, rule := range scope.Rules {
		rules = append(rules, toCoreScopeRule(rule))
	}
	return &aimedia.ScopeConfig{
		Default: scope.Default,
		Rules:   rules,
	}
}

func toCoreAttachmentsConfig(cfg *MediaUnderstandingAttachmentsConfig) *aimedia.AttachmentsConfig {
	if cfg == nil {
		return nil
	}
	return &aimedia.AttachmentsConfig{
		Mode:           cfg.Mode,
		MaxAttachments: cfg.MaxAttachments,
		Prefer:         cfg.Prefer,
	}
}

func toCoreDeepgramConfig(cfg *MediaUnderstandingDeepgramConfig) *aimedia.DeepgramConfig {
	if cfg == nil {
		return nil
	}
	return &aimedia.DeepgramConfig{
		DetectLanguage: cfg.DetectLanguage,
		Punctuate:      cfg.Punctuate,
		SmartFormat:    cfg.SmartFormat,
	}
}

func toCoreMediaModelConfig(cfg MediaUnderstandingModelConfig) aimedia.ModelConfig {
	return aimedia.ModelConfig{
		Provider:         cfg.Provider,
		Model:            cfg.Model,
		Capabilities:     cfg.Capabilities,
		Type:             cfg.Type,
		Command:          cfg.Command,
		Args:             cfg.Args,
		Prompt:           cfg.Prompt,
		MaxChars:         cfg.MaxChars,
		MaxBytes:         cfg.MaxBytes,
		TimeoutSeconds:   cfg.TimeoutSeconds,
		Language:         cfg.Language,
		ProviderOptions:  cfg.ProviderOptions,
		Deepgram:         toCoreDeepgramConfig(cfg.Deepgram),
		BaseURL:          cfg.BaseURL,
		Headers:          cfg.Headers,
		Profile:          cfg.Profile,
		PreferredProfile: cfg.PreferredProfile,
	}
}

func fromCoreMediaModelConfig(cfg aimedia.ModelConfig) MediaUnderstandingModelConfig {
	return MediaUnderstandingModelConfig{
		Provider:         cfg.Provider,
		Model:            cfg.Model,
		Capabilities:     cfg.Capabilities,
		Type:             cfg.Type,
		Command:          cfg.Command,
		Args:             cfg.Args,
		Prompt:           cfg.Prompt,
		MaxChars:         cfg.MaxChars,
		MaxBytes:         cfg.MaxBytes,
		TimeoutSeconds:   cfg.TimeoutSeconds,
		Language:         cfg.Language,
		ProviderOptions:  cfg.ProviderOptions,
		Deepgram:         fromCoreDeepgramConfig(cfg.Deepgram),
		BaseURL:          cfg.BaseURL,
		Headers:          cfg.Headers,
		Profile:          cfg.Profile,
		PreferredProfile: cfg.PreferredProfile,
	}
}

func fromCoreDeepgramConfig(cfg *aimedia.DeepgramConfig) *MediaUnderstandingDeepgramConfig {
	if cfg == nil {
		return nil
	}
	return &MediaUnderstandingDeepgramConfig{
		DetectLanguage: cfg.DetectLanguage,
		Punctuate:      cfg.Punctuate,
		SmartFormat:    cfg.SmartFormat,
	}
}

func toCoreMediaConfig(cfg *MediaUnderstandingConfig) *aimedia.CapabilityConfig {
	if cfg == nil {
		return nil
	}
	models := make([]aimedia.ModelConfig, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		models = append(models, toCoreMediaModelConfig(model))
	}
	return &aimedia.CapabilityConfig{
		Enabled:         cfg.Enabled,
		Scope:           toCoreScopeConfig(cfg.Scope),
		MaxBytes:        cfg.MaxBytes,
		MaxChars:        cfg.MaxChars,
		Prompt:          cfg.Prompt,
		TimeoutSeconds:  cfg.TimeoutSeconds,
		Language:        cfg.Language,
		ProviderOptions: cfg.ProviderOptions,
		Deepgram:        toCoreDeepgramConfig(cfg.Deepgram),
		BaseURL:         cfg.BaseURL,
		Headers:         cfg.Headers,
		Attachments:     toCoreAttachmentsConfig(cfg.Attachments),
		Models:          models,
	}
}

func toCoreToolsConfig(cfg *MediaToolsConfig) *aimedia.ToolsConfig {
	if cfg == nil {
		return nil
	}
	models := make([]aimedia.ModelConfig, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		models = append(models, toCoreMediaModelConfig(model))
	}
	return &aimedia.ToolsConfig{
		Models:      models,
		Concurrency: cfg.Concurrency,
		Image:       toCoreMediaConfig(cfg.Image),
		Audio:       toCoreMediaConfig(cfg.Audio),
		Video:       toCoreMediaConfig(cfg.Video),
	}
}

func toCoreMediaCapability(value MediaUnderstandingCapability) aimedia.MediaUnderstandingCapability {
	return aimedia.MediaUnderstandingCapability(value)
}

func fromCoreMediaOutputs(values []aimedia.MediaUnderstandingOutput) []MediaUnderstandingOutput {
	if len(values) == 0 {
		return nil
	}
	out := make([]MediaUnderstandingOutput, 0, len(values))
	for _, value := range values {
		out = append(out, MediaUnderstandingOutput{
			Kind:            MediaUnderstandingKind(value.Kind),
			AttachmentIndex: value.AttachmentIndex,
			Text:            value.Text,
			Provider:        value.Provider,
			Model:           value.Model,
		})
	}
	return out
}

func toCoreMediaOutputs(values []MediaUnderstandingOutput) []aimedia.MediaUnderstandingOutput {
	if len(values) == 0 {
		return nil
	}
	out := make([]aimedia.MediaUnderstandingOutput, 0, len(values))
	for _, value := range values {
		out = append(out, aimedia.MediaUnderstandingOutput{
			Kind:            aimedia.MediaUnderstandingKind(value.Kind),
			AttachmentIndex: value.AttachmentIndex,
			Text:            value.Text,
			Provider:        value.Provider,
			Model:           value.Model,
		})
	}
	return out
}

package connector

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/event"
)

const (
	defaultImageUnderstandingPrompt = "Describe the image."
	defaultAudioUnderstandingPrompt = "Transcribe the audio."
	imageUnderstandingMaxTokens     = 1024
)

func (oc *AIClient) canUseMediaUnderstanding(meta *PortalMetadata) bool {
	if meta == nil || meta.IsRawMode {
		return false
	}
	return hasAssignedAgent(meta)
}

// resolveImageUnderstandingModel returns a vision-capable model from the agent's model chain.
func (oc *AIClient) resolveImageUnderstandingModel(ctx context.Context, meta *PortalMetadata) string {
	if !oc.canUseMediaUnderstanding(meta) {
		return ""
	}

	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}
	if agentID == "" {
		return ""
	}

	store := NewAgentStoreAdapter(oc)
	agent, err := store.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		return ""
	}

	var candidates []string
	if strings.TrimSpace(agent.Model.Primary) != "" {
		candidates = append(candidates, agent.Model.Primary)
	}
	for _, fb := range agent.Model.Fallbacks {
		if strings.TrimSpace(fb) == "" {
			continue
		}
		candidates = append(candidates, fb)
	}

	for _, candidate := range candidates {
		resolved := ResolveAlias(candidate)
		if resolved == "" {
			continue
		}
		caps := getModelCapabilities(resolved, oc.findModelInfo(resolved))
		if caps.SupportsVision {
			return resolved
		}
	}

	return ""
}

// resolveVisionModelForImage returns the model to use for image analysis.
// The second return value is true when a fallback model (not the effective model) is used.
func (oc *AIClient) resolveVisionModelForImage(ctx context.Context, meta *PortalMetadata) (string, bool) {
	modelID := oc.effectiveModel(meta)
	caps := getModelCapabilities(modelID, oc.findModelInfo(modelID))
	if caps.SupportsVision {
		return modelID, false
	}

	if !oc.canUseMediaUnderstanding(meta) {
		return "", false
	}

	fallback := oc.resolveImageUnderstandingModel(ctx, meta)
	if fallback == "" {
		return "", false
	}
	return fallback, true
}

// resolveAudioUnderstandingModel returns an audio-capable model from the agent's model chain.
func (oc *AIClient) resolveAudioUnderstandingModel(ctx context.Context, meta *PortalMetadata) string {
	if !oc.canUseMediaUnderstanding(meta) {
		return ""
	}

	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}
	if agentID == "" {
		return ""
	}

	store := NewAgentStoreAdapter(oc)
	agent, err := store.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		return ""
	}

	var candidates []string
	if strings.TrimSpace(agent.Model.Primary) != "" {
		candidates = append(candidates, agent.Model.Primary)
	}
	for _, fb := range agent.Model.Fallbacks {
		if strings.TrimSpace(fb) == "" {
			continue
		}
		candidates = append(candidates, fb)
	}

	for _, candidate := range candidates {
		resolved := ResolveAlias(candidate)
		if resolved == "" {
			continue
		}
		caps := getModelCapabilities(resolved, oc.findModelInfo(resolved))
		if caps.SupportsAudio {
			return resolved
		}
	}

	loginMeta := loginMetadata(oc.UserLogin)
	provider := loginMeta.Provider

	// Prefer cached/provider-listed models first.
	if modelID := oc.pickAudioModelFromList(loginMeta.ModelCache, provider); modelID != "" {
		return modelID
	}
	models, err := oc.listAvailableModels(ctx, false)
	if err == nil {
		if modelID := pickAudioModelFromList(models, provider); modelID != "" {
			return modelID
		}
	}

	// Fallback to manifest-only lookup.
	if modelID := pickAudioModelFromManifest(provider); modelID != "" {
		return modelID
	}

	return ""
}

func (oc *AIClient) pickAudioModelFromList(cache *ModelCache, provider string) string {
	if cache == nil || len(cache.Models) == 0 {
		return ""
	}
	return pickAudioModelFromList(cache.Models, provider)
}

func pickAudioModelFromList(models []ModelInfo, provider string) string {
	for _, info := range models {
		if !info.SupportsAudio {
			continue
		}
		if !providerMatches(info, provider) {
			continue
		}
		return info.ID
	}
	return ""
}

func pickAudioModelFromManifest(provider string) string {
	for _, info := range ModelManifest.Models {
		if !info.SupportsAudio {
			continue
		}
		if !providerMatches(info, provider) {
			continue
		}
		return info.ID
	}
	return ""
}

func providerMatches(info ModelInfo, provider string) bool {
	switch provider {
	case ProviderOpenRouter, ProviderBeeper:
		if info.Provider != "" {
			return info.Provider == "openrouter"
		}
		return strings.HasPrefix(info.ID, "openrouter/")
	case ProviderOpenAI:
		if info.Provider != "" {
			return info.Provider == "openai"
		}
		return strings.HasPrefix(info.ID, "openai/")
	default:
		return true
	}
}

// resolveAudioModelForInput returns the model to use for audio analysis.
// The second return value is true when a fallback model (not the effective model) is used.
func (oc *AIClient) resolveAudioModelForInput(ctx context.Context, meta *PortalMetadata) (string, bool) {
	modelID := oc.effectiveModel(meta)
	caps := getModelCapabilities(modelID, oc.findModelInfo(modelID))
	if caps.SupportsAudio {
		return modelID, false
	}

	if !oc.canUseMediaUnderstanding(meta) {
		return "", false
	}

	fallback := oc.resolveAudioUnderstandingModel(ctx, meta)
	if fallback == "" {
		return "", false
	}
	return fallback, true
}

func (oc *AIClient) analyzeImageWithModel(
	ctx context.Context,
	modelID string,
	imageURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	prompt string,
) (string, error) {
	if strings.TrimSpace(modelID) == "" {
		return "", fmt.Errorf("missing model for image analysis")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultImageUnderstandingPrompt
	}

	b64Data, actualMimeType, err := oc.downloadMediaBase64(ctx, imageURL, encryptedFile, 20, mimeType)
	if err != nil {
		return "", err
	}
	if actualMimeType == "" {
		actualMimeType = "image/jpeg"
	}

	dataURL := buildDataURL(actualMimeType, b64Data)

	messages := []UnifiedMessage{
		{
			Role: RoleUser,
			Content: []ContentPart{
				{
					Type:     ContentTypeImage,
					ImageURL: dataURL,
					MimeType: actualMimeType,
				},
				{
					Type: ContentTypeText,
					Text: prompt,
				},
			},
		},
	}

	resp, err := oc.provider.Generate(ctx, GenerateParams{
		Model:               oc.modelIDForAPI(modelID),
		Messages:            messages,
		MaxCompletionTokens: imageUnderstandingMaxTokens,
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Content), nil
}

func (oc *AIClient) analyzeAudioWithModel(
	ctx context.Context,
	modelID string,
	audioURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	prompt string,
) (string, error) {
	if strings.TrimSpace(modelID) == "" {
		return "", fmt.Errorf("missing model for audio analysis")
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultAudioUnderstandingPrompt
	}

	b64Data, actualMimeType, err := oc.downloadMediaBase64(ctx, audioURL, encryptedFile, 25, mimeType)
	if err != nil {
		return "", err
	}
	format := getAudioFormat(actualMimeType)
	if format == "" {
		format = "mp3"
	}

	messages := []UnifiedMessage{
		{
			Role: RoleUser,
			Content: []ContentPart{
				{
					Type:        ContentTypeAudio,
					AudioB64:    b64Data,
					AudioFormat: format,
				},
				{
					Type: ContentTypeText,
					Text: prompt,
				},
			},
		},
	}

	resp, err := oc.provider.Generate(ctx, GenerateParams{
		Model:               oc.modelIDForAPI(modelID),
		Messages:            messages,
		MaxCompletionTokens: imageUnderstandingMaxTokens,
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(resp.Content), nil
}

func buildImageUnderstandingPrompt(caption string, hasUserCaption bool) string {
	if hasUserCaption {
		caption = strings.TrimSpace(caption)
		if caption != "" {
			return caption
		}
	}
	return defaultImageUnderstandingPrompt
}

func buildAudioUnderstandingPrompt(caption string, hasUserCaption bool) string {
	if hasUserCaption {
		caption = strings.TrimSpace(caption)
		if caption != "" {
			return caption
		}
	}
	return defaultAudioUnderstandingPrompt
}

func buildImageUnderstandingMessage(caption string, hasUserCaption bool, description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return ""
	}

	if hasUserCaption {
		caption = strings.TrimSpace(caption)
		if caption != "" {
			return fmt.Sprintf("%s [image]\nImage description: %s", caption, description)
		}
	}
	return fmt.Sprintf("[image]\nImage description: %s", description)
}

func buildAudioUnderstandingMessage(caption string, hasUserCaption bool, transcript string) string {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return ""
	}

	if hasUserCaption {
		caption = strings.TrimSpace(caption)
		if caption != "" {
			return fmt.Sprintf("%s [audio]\nAudio transcription: %s", caption, transcript)
		}
	}
	return fmt.Sprintf("[audio]\nAudio transcription: %s", transcript)
}

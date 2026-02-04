package connector

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

type mediaUnderstandingResult struct {
	Outputs    []MediaUnderstandingOutput
	Body       string
	Transcript string
}

func mediaCapabilityForMessage(msgType event.MessageType) (MediaUnderstandingCapability, bool) {
	switch msgType {
	case event.MsgImage:
		return MediaCapabilityImage, true
	case event.MsgAudio:
		return MediaCapabilityAudio, true
	case event.MsgVideo:
		return MediaCapabilityVideo, true
	default:
		return "", false
	}
}

func (oc *AIClient) applyMediaUnderstandingForAttachment(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	capability MediaUnderstandingCapability,
	mediaURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	rawCaption string,
	hasUserCaption bool,
) (*mediaUnderstandingResult, error) {
	toolsCfg := oc.connector.Config.Tools.Media
	var capCfg *MediaUnderstandingConfig
	if toolsCfg != nil {
		switch capability {
		case MediaCapabilityImage:
			capCfg = toolsCfg.Image
		case MediaCapabilityAudio:
			capCfg = toolsCfg.Audio
		case MediaCapabilityVideo:
			capCfg = toolsCfg.Video
		}
	}

	if capCfg != nil && capCfg.Enabled != nil && !*capCfg.Enabled {
		return nil, nil
	}

	if capCfg != nil && capCfg.Scope != nil {
		if oc.mediaUnderstandingScopeDecision(ctx, portal, capCfg.Scope) == scopeDeny {
			return nil, nil
		}
	}

	// Skip image understanding when the primary model supports vision.
	if capability == MediaCapabilityImage {
		caps := oc.getModelCapabilitiesForMeta(meta)
		if caps.SupportsVision {
			return nil, nil
		}
	}

	entries := resolveMediaEntries(toolsCfg, capCfg, capability)
	if len(entries) == 0 && capability == MediaCapabilityAudio {
		if auto := oc.resolveAutoAudioEntry(capCfg); auto != nil {
			entries = append(entries, *auto)
		}
	}
	if len(entries) == 0 {
		return nil, nil
	}

	var lastErr error
	for _, entry := range entries {
		output, err := oc.runMediaUnderstandingEntry(ctx, capability, mediaURL, mimeType, encryptedFile, entry, capCfg)
		if err != nil {
			lastErr = err
			continue
		}
		if output == nil || strings.TrimSpace(output.Text) == "" {
			continue
		}
		bodyBase := ""
		if hasUserCaption {
			bodyBase = rawCaption
		}
		combined := formatMediaUnderstandingBody(bodyBase, []MediaUnderstandingOutput{*output})
		result := &mediaUnderstandingResult{
			Outputs: []MediaUnderstandingOutput{*output},
			Body:    combined,
		}
		if output.Kind == MediaKindAudioTranscription {
			result.Transcript = formatAudioTranscripts([]MediaUnderstandingOutput{*output})
		}
		return result, nil
	}

	return nil, lastErr
}

func (oc *AIClient) resolveAutoAudioEntry(cfg *MediaUnderstandingConfig) *MediaUnderstandingModelConfig {
	headers := map[string]string{}
	if cfg != nil && cfg.Headers != nil {
		for key, value := range cfg.Headers {
			headers[key] = value
		}
	}

	if key := oc.resolveMediaProviderAPIKey("openai"); key != "" || hasProviderAuthHeader("openai", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "openai",
			Model:    defaultAudioModelsByProvider["openai"],
		}
	}
	if key := oc.resolveMediaProviderAPIKey("groq"); key != "" || hasProviderAuthHeader("groq", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "groq",
			Model:    defaultAudioModelsByProvider["groq"],
		}
	}
	if key := oc.resolveMediaProviderAPIKey("deepgram"); key != "" || hasProviderAuthHeader("deepgram", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "deepgram",
			Model:    defaultAudioModelsByProvider["deepgram"],
		}
	}
	if key := oc.resolveMediaProviderAPIKey("google"); key != "" || hasProviderAuthHeader("google", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "google",
			Model:    defaultGoogleAudioModel,
		}
	}

	return nil
}

func (oc *AIClient) runMediaUnderstandingEntry(
	ctx context.Context,
	capability MediaUnderstandingCapability,
	mediaURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	entry MediaUnderstandingModelConfig,
	capCfg *MediaUnderstandingConfig,
) (*MediaUnderstandingOutput, error) {
	entryType := strings.TrimSpace(entry.Type)
	if entryType == "" {
		if strings.TrimSpace(entry.Command) != "" {
			entryType = "cli"
		} else {
			entryType = "provider"
		}
	}

	maxChars := resolveMediaMaxChars(capability, entry, capCfg)
	maxBytes := resolveMediaMaxBytes(capability, entry, capCfg)
	prompt := resolveMediaPrompt(capability, entry.Prompt, maxChars)
	timeout := resolveMediaTimeoutSeconds(entry.TimeoutSeconds, defaultTimeoutSecondsByCapability[capability])

	switch entryType {
	case "cli":
		data, actualMime, err := oc.downloadMediaBytes(ctx, mediaURL, encryptedFile, maxBytes, mimeType)
		if err != nil {
			return nil, err
		}
		fileName := resolveMediaFileName("", string(capability), mediaURL)
		tempDir, err := os.MkdirTemp("", "ai-bridge-media-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tempDir)
		mediaPath := filepath.Join(tempDir, fileName)
		if err := os.WriteFile(mediaPath, data, 0600); err != nil {
			return nil, err
		}
		if actualMime != "" {
			mimeType = actualMime
		}
		output, err := runMediaCLI(ctx, entry.Command, entry.Args, prompt, maxChars, mediaPath)
		if err != nil {
			return nil, err
		}
		return buildMediaOutput(capability, output, "cli", entry.Model), nil

	default:
		providerID := normalizeMediaProviderID(entry.Provider)
		if providerID == "" && capability != MediaCapabilityImage {
			return nil, fmt.Errorf("missing provider for %s understanding", capability)
		}

		switch capability {
		case MediaCapabilityImage:
			return oc.describeImageWithEntry(ctx, entry, mediaURL, mimeType, encryptedFile, maxBytes, maxChars, prompt)
		case MediaCapabilityAudio:
			return oc.transcribeAudioWithEntry(ctx, entry, capCfg, mediaURL, mimeType, encryptedFile, maxBytes, maxChars, prompt, timeout)
		case MediaCapabilityVideo:
			return oc.describeVideoWithEntry(ctx, entry, capCfg, mediaURL, mimeType, encryptedFile, maxBytes, maxChars, prompt, timeout)
		}
	}
	return nil, fmt.Errorf("unsupported media capability %s", capability)
}

func (oc *AIClient) describeImageWithEntry(
	ctx context.Context,
	entry MediaUnderstandingModelConfig,
	mediaURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	maxBytes int,
	maxChars int,
	prompt string,
) (*MediaUnderstandingOutput, error) {
	modelID := strings.TrimSpace(entry.Model)
	if modelID == "" {
		return nil, fmt.Errorf("image understanding requires model id")
	}
	if entry.Provider != "" {
		currentProvider := normalizeMediaProviderID(loginMetadata(oc.UserLogin).Provider)
		entryProvider := normalizeMediaProviderID(entry.Provider)
		if entryProvider != "" && currentProvider != "" && entryProvider != currentProvider {
			return nil, fmt.Errorf("image provider %s not available for current login provider", entryProvider)
		}
	}

	b64Data, actualMime, err := oc.downloadMediaBase64Bytes(ctx, mediaURL, encryptedFile, maxBytes, mimeType)
	if err != nil {
		return nil, err
	}
	if actualMime == "" {
		actualMime = mimeType
	}
	if actualMime == "" {
		actualMime = "image/jpeg"
	}
	dataURL := buildDataURL(actualMime, b64Data)

	messages := []UnifiedMessage{
		{
			Role: RoleUser,
			Content: []ContentPart{
				{
					Type:     ContentTypeImage,
					ImageURL: dataURL,
					MimeType: actualMime,
				},
				{
					Type: ContentTypeText,
					Text: prompt,
				},
			},
		},
	}
	modelIDForAPI := oc.modelIDForAPI(ResolveAlias(modelID))
	resp, err := oc.provider.Generate(ctx, GenerateParams{
		Model:               modelIDForAPI,
		Messages:            messages,
		MaxCompletionTokens: defaultImageUnderstandingLimit,
	})
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(resp.Content)
	if maxChars > 0 && len(text) > maxChars {
		text = text[:maxChars]
	}
	return buildMediaOutput(MediaCapabilityImage, text, entry.Provider, modelID), nil
}

func (oc *AIClient) transcribeAudioWithEntry(
	ctx context.Context,
	entry MediaUnderstandingModelConfig,
	capCfg *MediaUnderstandingConfig,
	mediaURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	maxBytes int,
	maxChars int,
	prompt string,
	timeout time.Duration,
) (*MediaUnderstandingOutput, error) {
	providerID := normalizeMediaProviderID(entry.Provider)
	if providerID == "" {
		return nil, fmt.Errorf("missing audio provider")
	}
	data, actualMime, err := oc.downloadMediaBytes(ctx, mediaURL, encryptedFile, maxBytes, mimeType)
	if err != nil {
		return nil, err
	}
	if actualMime == "" {
		actualMime = mimeType
	}
	fileName := resolveMediaFileName("", string(MediaCapabilityAudio), mediaURL)

	headers := mergeMediaHeaders(capCfg, entry)
	apiKey := oc.resolveMediaProviderAPIKey(providerID)
	if apiKey == "" && !hasProviderAuthHeader(providerID, headers) {
		return nil, fmt.Errorf("missing API key for %s audio transcription", providerID)
	}

	request := mediaAudioRequest{
		Provider: providerID,
		APIKey:   apiKey,
		BaseURL:  resolveMediaBaseURL(capCfg, entry),
		Headers:  headers,
		Model:    strings.TrimSpace(entry.Model),
		Language: resolveMediaLanguage(entry, capCfg),
		Prompt:   prompt,
		MimeType: actualMime,
		FileName: fileName,
		Data:     data,
		Timeout:  timeout,
	}

	var text string
	switch providerID {
	case "openai", "groq":
		text, err = transcribeOpenAICompatibleAudio(ctx, request)
	case "deepgram":
		query := resolveProviderQuery("deepgram", capCfg, entry)
		text, err = transcribeDeepgramAudio(ctx, request, query)
	case "google":
		text, err = transcribeGeminiAudio(ctx, request)
	default:
		err = fmt.Errorf("unsupported audio provider: %s", providerID)
	}
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if maxChars > 0 && len(text) > maxChars {
		text = text[:maxChars]
	}
	return buildMediaOutput(MediaCapabilityAudio, text, providerID, entry.Model), nil
}

func (oc *AIClient) describeVideoWithEntry(
	ctx context.Context,
	entry MediaUnderstandingModelConfig,
	capCfg *MediaUnderstandingConfig,
	mediaURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	maxBytes int,
	maxChars int,
	prompt string,
	timeout time.Duration,
) (*MediaUnderstandingOutput, error) {
	providerID := normalizeMediaProviderID(entry.Provider)
	if providerID == "" {
		return nil, fmt.Errorf("missing video provider")
	}
	if providerID != "google" {
		return nil, fmt.Errorf("unsupported video provider: %s", providerID)
	}

	data, actualMime, err := oc.downloadMediaBytes(ctx, mediaURL, encryptedFile, maxBytes, mimeType)
	if err != nil {
		return nil, err
	}
	if actualMime == "" {
		actualMime = mimeType
	}
	if estimateBase64Size(len(data)) > defaultVideoMaxBase64Bytes {
		return nil, fmt.Errorf("video payload exceeds base64 limit")
	}

	headers := mergeMediaHeaders(capCfg, entry)
	apiKey := oc.resolveMediaProviderAPIKey(providerID)
	if apiKey == "" && !hasProviderAuthHeader(providerID, headers) {
		return nil, fmt.Errorf("missing API key for %s video description", providerID)
	}

	request := mediaVideoRequest{
		APIKey:   apiKey,
		BaseURL:  resolveMediaBaseURL(capCfg, entry),
		Headers:  headers,
		Model:    strings.TrimSpace(entry.Model),
		Prompt:   prompt,
		MimeType: actualMime,
		Data:     data,
		Timeout:  timeout,
	}
	text, err := describeGeminiVideo(ctx, request)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if maxChars > 0 && len(text) > maxChars {
		text = text[:maxChars]
	}
	return buildMediaOutput(MediaCapabilityVideo, text, providerID, entry.Model), nil
}

func resolveMediaBaseURL(cfg *MediaUnderstandingConfig, entry MediaUnderstandingModelConfig) string {
	if strings.TrimSpace(entry.BaseURL) != "" {
		return entry.BaseURL
	}
	if cfg != nil && strings.TrimSpace(cfg.BaseURL) != "" {
		return cfg.BaseURL
	}
	return ""
}

func mergeMediaHeaders(cfg *MediaUnderstandingConfig, entry MediaUnderstandingModelConfig) map[string]string {
	merged := map[string]string{}
	if cfg != nil {
		for key, value := range cfg.Headers {
			merged[key] = value
		}
	}
	for key, value := range entry.Headers {
		merged[key] = value
	}
	return merged
}

func hasProviderAuthHeader(providerID string, headers map[string]string) bool {
	for key := range headers {
		switch strings.ToLower(key) {
		case "authorization":
			if providerID == "openai" || providerID == "groq" || providerID == "deepgram" {
				return true
			}
		case "x-goog-api-key":
			if providerID == "google" {
				return true
			}
		}
	}
	return false
}

func (oc *AIClient) resolveMediaProviderAPIKey(providerID string) string {
	switch providerID {
	case "openai":
		if oc.connector != nil {
			if key := strings.TrimSpace(oc.connector.resolveOpenAIAPIKey(loginMetadata(oc.UserLogin))); key != "" {
				return key
			}
		}
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	case "groq":
		return strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	case "deepgram":
		return strings.TrimSpace(os.Getenv("DEEPGRAM_API_KEY"))
	case "google":
		if key := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); key != "" {
			return key
		}
		return strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	default:
		return ""
	}
}

func buildMediaOutput(capability MediaUnderstandingCapability, text string, provider string, model string) *MediaUnderstandingOutput {
	kind := MediaKindImageDescription
	switch capability {
	case MediaCapabilityAudio:
		kind = MediaKindAudioTranscription
	case MediaCapabilityVideo:
		kind = MediaKindVideoDescription
	}
	return &MediaUnderstandingOutput{
		Kind:            kind,
		AttachmentIndex: 0,
		Text:            strings.TrimSpace(text),
		Provider:        strings.TrimSpace(provider),
		Model:           strings.TrimSpace(model),
	}
}

func estimateBase64Size(size int) int {
	if size <= 0 {
		return 0
	}
	return ((size + 2) / 3) * 4
}

func (oc *AIClient) downloadMediaBase64Bytes(
	ctx context.Context,
	mediaURL string,
	encryptedFile *event.EncryptedFileInfo,
	maxBytes int,
	fallbackMime string,
) (string, string, error) {
	data, mimeType, err := oc.downloadMediaBytes(ctx, mediaURL, encryptedFile, maxBytes, fallbackMime)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(data), mimeType, nil
}

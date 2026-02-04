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
	Decisions  []MediaUnderstandingDecision
	Body       string
	Transcript string
	FileBlocks []string
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

func (oc *AIClient) applyMediaUnderstandingForAttachments(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	capability MediaUnderstandingCapability,
	attachments []mediaAttachment,
	rawCaption string,
	hasUserCaption bool,
) (*mediaUnderstandingResult, error) {
	result := &mediaUnderstandingResult{}
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
		result.Decisions = []MediaUnderstandingDecision{{
			Capability: capability,
			Outcome:    "disabled",
		}}
		return result, nil
	}

	selected := selectMediaAttachments(attachments, capCfg.Attachments)
	if len(selected) == 0 {
		result.Decisions = []MediaUnderstandingDecision{{
			Capability: capability,
			Outcome:    "no-attachment",
		}}
		return result, nil
	}

	if capCfg != nil && capCfg.Scope != nil {
		if oc.mediaUnderstandingScopeDecision(ctx, portal, capCfg.Scope) == scopeDeny {
			attachmentDecisions := make([]MediaUnderstandingAttachmentDecision, 0, len(selected))
			for _, attachment := range selected {
				attachmentDecisions = append(attachmentDecisions, MediaUnderstandingAttachmentDecision{
					AttachmentIndex: attachment.Index,
					Attempts:        nil,
				})
			}
			result.Decisions = []MediaUnderstandingDecision{{
				Capability:  capability,
				Outcome:     "scope-deny",
				Attachments: attachmentDecisions,
			}}
			return result, nil
		}
	}

	// Skip image understanding when the primary model supports vision.
	if capability == MediaCapabilityImage {
		caps := oc.getModelCapabilitiesForMeta(meta)
		if caps.SupportsVision {
			attachmentDecisions := make([]MediaUnderstandingAttachmentDecision, 0, len(selected))
			for _, attachment := range selected {
				attempt := MediaUnderstandingModelDecision{
					Type:     "provider",
					Provider: normalizeMediaProviderID(loginMetadata(oc.UserLogin).Provider),
					Model:    oc.effectiveModel(meta),
					Outcome:  "skipped",
					Reason:   "primary model supports vision",
				}
				attachmentDecisions = append(attachmentDecisions, MediaUnderstandingAttachmentDecision{
					AttachmentIndex: attachment.Index,
					Attempts:        []MediaUnderstandingModelDecision{attempt},
					Chosen:          &attempt,
				})
			}
			result.Decisions = []MediaUnderstandingDecision{{
				Capability:  capability,
				Outcome:     "skipped",
				Attachments: attachmentDecisions,
			}}
			return result, nil
		}
	}

	entries := resolveMediaEntries(toolsCfg, capCfg, capability)
	if len(entries) == 0 {
		if auto := oc.resolveAutoMediaEntry(capability, capCfg); auto != nil {
			entries = append(entries, *auto)
		}
	}
	if len(entries) == 0 {
		attachmentDecisions := make([]MediaUnderstandingAttachmentDecision, 0, len(selected))
		for _, attachment := range selected {
			attachmentDecisions = append(attachmentDecisions, MediaUnderstandingAttachmentDecision{
				AttachmentIndex: attachment.Index,
				Attempts:        nil,
			})
		}
		result.Decisions = []MediaUnderstandingDecision{{
			Capability:  capability,
			Outcome:     "skipped",
			Attachments: attachmentDecisions,
		}}
		return result, nil
	}

	var outputs []MediaUnderstandingOutput
	var lastErr error
	attachmentDecisions := make([]MediaUnderstandingAttachmentDecision, 0, len(selected))
	for _, attachment := range selected {
		output, attempts, err := oc.runMediaUnderstandingEntries(ctx, capability, attachment, entries, capCfg)
		if err != nil {
			lastErr = err
		}
		decision := MediaUnderstandingAttachmentDecision{
			AttachmentIndex: attachment.Index,
			Attempts:        attempts,
		}
		for i := range attempts {
			if attempts[i].Outcome == "success" {
				decision.Chosen = &attempts[i]
				break
			}
		}
		if output != nil {
			outputs = append(outputs, *output)
		}
		attachmentDecisions = append(attachmentDecisions, decision)
	}

	result.Outputs = outputs
	decisionOutcome := "skipped"
	if len(outputs) > 0 {
		decisionOutcome = "success"
	}
	result.Decisions = []MediaUnderstandingDecision{{
		Capability:  capability,
		Outcome:     decisionOutcome,
		Attachments: attachmentDecisions,
	}}
	oc.log.Debug().
		Str("capability", string(capability)).
		Str("outcome", decisionOutcome).
		Int("attachments", len(selected)).
		Int("outputs", len(outputs)).
		Msg("Media understanding decision")

	bodyBase := ""
	if hasUserCaption {
		bodyBase = rawCaption
	}
	combined := formatMediaUnderstandingBody(bodyBase, outputs)
	if len(outputs) > 0 {
		audioOutputs := filterMediaOutputs(outputs, MediaKindAudioTranscription)
		if len(audioOutputs) > 0 {
			result.Transcript = formatAudioTranscripts(audioOutputs)
		}
	}

	fileBlocks := oc.extractMediaFileBlocks(ctx, selected, outputs)
	if len(fileBlocks) > 0 {
		result.FileBlocks = fileBlocks
		if combined == "" {
			combined = strings.Join(fileBlocks, "\n\n")
		} else {
			combined = strings.TrimSpace(combined + "\n\n" + strings.Join(fileBlocks, "\n\n"))
		}
	}
	result.Body = combined
	if len(outputs) == 0 && lastErr != nil {
		return result, lastErr
	}
	return result, nil
}

func (oc *AIClient) resolveAutoAudioEntry(cfg *MediaUnderstandingConfig) *MediaUnderstandingModelConfig {
	headers := map[string]string{}
	if cfg != nil && cfg.Headers != nil {
		for key, value := range cfg.Headers {
			headers[key] = value
		}
	}

	if key := oc.resolveMediaProviderAPIKey("openai", "", ""); key != "" || hasProviderAuthHeader("openai", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "openai",
			Model:    defaultAudioModelsByProvider["openai"],
		}
	}
	if key := oc.resolveMediaProviderAPIKey("groq", "", ""); key != "" || hasProviderAuthHeader("groq", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "groq",
			Model:    defaultAudioModelsByProvider["groq"],
		}
	}
	if key := oc.resolveMediaProviderAPIKey("deepgram", "", ""); key != "" || hasProviderAuthHeader("deepgram", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "deepgram",
			Model:    defaultAudioModelsByProvider["deepgram"],
		}
	}
	if key := oc.resolveMediaProviderAPIKey("google", "", ""); key != "" || hasProviderAuthHeader("google", headers) {
		return &MediaUnderstandingModelConfig{
			Provider: "google",
			Model:    defaultGoogleAudioModel,
		}
	}

	return nil
}

func (oc *AIClient) resolveAutoMediaEntry(
	capability MediaUnderstandingCapability,
	cfg *MediaUnderstandingConfig,
) *MediaUnderstandingModelConfig {
	switch capability {
	case MediaCapabilityAudio:
		return oc.resolveAutoAudioEntry(cfg)
	case MediaCapabilityImage, MediaCapabilityVideo:
		if oc == nil || oc.UserLogin == nil {
			return nil
		}
		provider := normalizeMediaProviderID(loginMetadata(oc.UserLogin).Provider)
		if provider != "openrouter" {
			return nil
		}
		model := defaultGoogleVideoModel
		if capability == MediaCapabilityImage {
			model = defaultGoogleImageModel
		}
		return &MediaUnderstandingModelConfig{
			Provider: "openrouter",
			Model:    model,
		}
	default:
		return nil
	}
}

func (oc *AIClient) runMediaUnderstandingEntries(
	ctx context.Context,
	capability MediaUnderstandingCapability,
	attachment mediaAttachment,
	entries []MediaUnderstandingModelConfig,
	capCfg *MediaUnderstandingConfig,
) (*MediaUnderstandingOutput, []MediaUnderstandingModelDecision, error) {
	attempts := make([]MediaUnderstandingModelDecision, 0, len(entries))
	var lastErr error
	for _, entry := range entries {
		entryType := strings.TrimSpace(entry.Type)
		if entryType == "" {
			if strings.TrimSpace(entry.Command) != "" {
				entryType = "cli"
			} else {
				entryType = "provider"
			}
		}
		provider := strings.TrimSpace(entry.Provider)
		model := strings.TrimSpace(entry.Model)
		if entryType == "cli" {
			provider = strings.TrimSpace(entry.Command)
			if provider == "" {
				provider = "cli"
			}
			if model == "" {
				model = provider
			}
		} else {
			provider = normalizeMediaProviderID(provider)
		}
		output, err := oc.runMediaUnderstandingEntry(ctx, capability, attachment, entry, capCfg)
		if err != nil {
			lastErr = err
			attempts = append(attempts, MediaUnderstandingModelDecision{
				Type:     entryType,
				Provider: provider,
				Model:    model,
				Outcome:  "failed",
				Reason:   err.Error(),
			})
			continue
		}
		if output == nil || strings.TrimSpace(output.Text) == "" {
			attempts = append(attempts, MediaUnderstandingModelDecision{
				Type:     entryType,
				Provider: provider,
				Model:    model,
				Outcome:  "skipped",
				Reason:   "empty output",
			})
			continue
		}
		attempts = append(attempts, MediaUnderstandingModelDecision{
			Type:     entryType,
			Provider: provider,
			Model:    model,
			Outcome:  "success",
		})
		return output, attempts, nil
	}
	return nil, attempts, lastErr
}

func filterMediaOutputs(outputs []MediaUnderstandingOutput, kind MediaUnderstandingKind) []MediaUnderstandingOutput {
	filtered := make([]MediaUnderstandingOutput, 0, len(outputs))
	for _, output := range outputs {
		if output.Kind == kind {
			filtered = append(filtered, output)
		}
	}
	return filtered
}

func (oc *AIClient) extractMediaFileBlocks(
	ctx context.Context,
	attachments []mediaAttachment,
	outputs []MediaUnderstandingOutput,
) []string {
	if len(attachments) == 0 {
		return nil
	}
	skip := map[int]bool{}
	for _, output := range outputs {
		if output.Kind == MediaKindAudioTranscription {
			skip[output.AttachmentIndex] = true
		}
	}
	blocks := []string{}
	for _, attachment := range attachments {
		if skip[attachment.Index] {
			continue
		}
		mimeType := normalizeMimeType(attachment.MimeType)
		if mimeType == "" || !isTextFileMime(mimeType) {
			continue
		}
		content, truncated, err := oc.downloadTextFile(ctx, attachment.URL, attachment.EncryptedFile, mimeType)
		if err != nil {
			oc.log.Warn().Err(err).
				Int("attachment_index", attachment.Index).
				Msg("Failed to extract text file block for media understanding")
			continue
		}
		block := buildTextFileMessage("", false, attachment.FileName, mimeType, content, truncated)
		if strings.TrimSpace(block) == "" {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func (oc *AIClient) runMediaUnderstandingEntry(
	ctx context.Context,
	capability MediaUnderstandingCapability,
	attachment mediaAttachment,
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
	prompt := resolveMediaPrompt(capability, entry.Prompt, capCfg, maxChars)
	timeout := resolveMediaTimeoutSeconds(entry.TimeoutSeconds, capCfg, defaultTimeoutSecondsByCapability[capability])

	switch entryType {
	case "cli":
		data, actualMime, err := oc.downloadMediaBytes(ctx, attachment.URL, attachment.EncryptedFile, maxBytes, attachment.MimeType)
		if err != nil {
			return nil, err
		}
		fileName := resolveMediaFileName(attachment.FileName, string(capability), attachment.URL)
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
			attachment.MimeType = actualMime
		}
		output, err := runMediaCLI(ctx, entry.Command, entry.Args, prompt, maxChars, mediaPath)
		if err != nil {
			return nil, err
		}
		return buildMediaOutput(capability, output, "cli", entry.Model, attachment.Index), nil

	default:
		providerID := normalizeMediaProviderID(entry.Provider)
		if providerID == "" && capability != MediaCapabilityImage {
			return nil, fmt.Errorf("missing provider for %s understanding", capability)
		}

		switch capability {
		case MediaCapabilityImage:
			return oc.describeImageWithEntry(ctx, entry, attachment.URL, attachment.MimeType, attachment.EncryptedFile, maxBytes, maxChars, prompt, attachment.Index)
		case MediaCapabilityAudio:
			return oc.transcribeAudioWithEntry(ctx, entry, capCfg, attachment.URL, attachment.MimeType, attachment.EncryptedFile, attachment.FileName, maxBytes, maxChars, prompt, timeout, attachment.Index)
		case MediaCapabilityVideo:
			return oc.describeVideoWithEntry(ctx, entry, capCfg, attachment.URL, attachment.MimeType, attachment.EncryptedFile, maxBytes, maxChars, prompt, timeout, attachment.Index)
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
	attachmentIndex int,
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
					Type: ContentTypeText,
					Text: prompt,
				},
				{
					Type:     ContentTypeImage,
					ImageURL: dataURL,
					MimeType: actualMime,
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
	return buildMediaOutput(MediaCapabilityImage, text, entry.Provider, modelID, attachmentIndex), nil
}

func (oc *AIClient) transcribeAudioWithEntry(
	ctx context.Context,
	entry MediaUnderstandingModelConfig,
	capCfg *MediaUnderstandingConfig,
	mediaURL string,
	mimeType string,
	encryptedFile *event.EncryptedFileInfo,
	fileName string,
	maxBytes int,
	maxChars int,
	prompt string,
	timeout time.Duration,
	attachmentIndex int,
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
	fileName = resolveMediaFileName(fileName, string(MediaCapabilityAudio), mediaURL)

	headers := mergeMediaHeaders(capCfg, entry)
	apiKey := oc.resolveMediaProviderAPIKey(providerID, entry.Profile, entry.PreferredProfile)
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
	return buildMediaOutput(MediaCapabilityAudio, text, providerID, entry.Model, attachmentIndex), nil
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
	attachmentIndex int,
) (*MediaUnderstandingOutput, error) {
	providerID := normalizeMediaProviderID(entry.Provider)
	if providerID == "" {
		return nil, fmt.Errorf("missing video provider")
	}
	if providerID == "openrouter" {
		modelID := strings.TrimSpace(entry.Model)
		if modelID == "" {
			return nil, fmt.Errorf("video understanding requires model id")
		}
		currentProvider := normalizeMediaProviderID(loginMetadata(oc.UserLogin).Provider)
		if currentProvider != "" && currentProvider != providerID {
			return nil, fmt.Errorf("video provider %s not available for current login provider", providerID)
		}

		data, actualMime, err := oc.downloadMediaBytes(ctx, mediaURL, encryptedFile, maxBytes, mimeType)
		if err != nil {
			return nil, err
		}
		if actualMime == "" {
			actualMime = mimeType
		}
		if actualMime == "" {
			actualMime = "video/mp4"
		}
		base64Size := estimateBase64Size(len(data))
		if base64Size > defaultVideoMaxBase64Bytes {
			oc.log.Warn().
				Int("base64_bytes", base64Size).
				Int("limit_bytes", defaultVideoMaxBase64Bytes).
				Msg("OpenRouter video payload exceeds base64 limit")
			return nil, fmt.Errorf("video payload exceeds base64 limit")
		}
		videoB64 := base64.StdEncoding.EncodeToString(data)

		messages := []UnifiedMessage{
			{
				Role: RoleUser,
				Content: []ContentPart{
					{
						Type: ContentTypeText,
						Text: prompt,
					},
					{
						Type:     ContentTypeVideo,
						VideoB64: videoB64,
						MimeType: actualMime,
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
		return buildMediaOutput(MediaCapabilityVideo, text, entry.Provider, modelID, attachmentIndex), nil
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
	base64Size := estimateBase64Size(len(data))
	if base64Size > defaultVideoMaxBase64Bytes {
		oc.log.Warn().
			Int("base64_bytes", base64Size).
			Int("limit_bytes", defaultVideoMaxBase64Bytes).
			Msg("Google video payload exceeds base64 limit")
		return nil, fmt.Errorf("video payload exceeds base64 limit")
	}

	headers := mergeMediaHeaders(capCfg, entry)
	apiKey := oc.resolveMediaProviderAPIKey(providerID, entry.Profile, entry.PreferredProfile)
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
	return buildMediaOutput(MediaCapabilityVideo, text, providerID, entry.Model, attachmentIndex), nil
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

func resolveProfiledEnvKey(base string, profile string) string {
	if base == "" || strings.TrimSpace(profile) == "" {
		return ""
	}
	normalized := strings.TrimSpace(profile)
	normalized = strings.ToUpper(normalized)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, ".", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	env := base + "_" + normalized
	return strings.TrimSpace(os.Getenv(env))
}

func (oc *AIClient) resolveMediaProviderAPIKey(providerID string, profile string, preferredProfile string) string {
	switch providerID {
	case "openai":
		if key := resolveProfiledEnvKey("OPENAI_API_KEY", profile); key != "" {
			return key
		}
		if key := resolveProfiledEnvKey("OPENAI_API_KEY", preferredProfile); key != "" {
			return key
		}
		if oc.connector != nil {
			if key := strings.TrimSpace(oc.connector.resolveOpenAIAPIKey(loginMetadata(oc.UserLogin))); key != "" {
				return key
			}
		}
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	case "groq":
		if key := resolveProfiledEnvKey("GROQ_API_KEY", profile); key != "" {
			return key
		}
		if key := resolveProfiledEnvKey("GROQ_API_KEY", preferredProfile); key != "" {
			return key
		}
		return strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	case "deepgram":
		if key := resolveProfiledEnvKey("DEEPGRAM_API_KEY", profile); key != "" {
			return key
		}
		if key := resolveProfiledEnvKey("DEEPGRAM_API_KEY", preferredProfile); key != "" {
			return key
		}
		return strings.TrimSpace(os.Getenv("DEEPGRAM_API_KEY"))
	case "google":
		if key := resolveProfiledEnvKey("GEMINI_API_KEY", profile); key != "" {
			return key
		}
		if key := resolveProfiledEnvKey("GEMINI_API_KEY", preferredProfile); key != "" {
			return key
		}
		if key := resolveProfiledEnvKey("GOOGLE_API_KEY", profile); key != "" {
			return key
		}
		if key := resolveProfiledEnvKey("GOOGLE_API_KEY", preferredProfile); key != "" {
			return key
		}
		if key := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); key != "" {
			return key
		}
		return strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	default:
		return ""
	}
}

func buildMediaOutput(capability MediaUnderstandingCapability, text string, provider string, model string, attachmentIndex int) *MediaUnderstandingOutput {
	kind := MediaKindImageDescription
	switch capability {
	case MediaCapabilityAudio:
		kind = MediaKindAudioTranscription
	case MediaCapabilityVideo:
		kind = MediaKindVideoDescription
	}
	return &MediaUnderstandingOutput{
		Kind:            kind,
		AttachmentIndex: attachmentIndex,
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

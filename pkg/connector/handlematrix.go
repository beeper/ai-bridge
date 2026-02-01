package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

// HandleMatrixMessage processes incoming Matrix messages and dispatches them to the AI
func (oc *AIClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if msg.Content == nil {
		return nil, fmt.Errorf("missing message content")
	}

	portal := msg.Portal
	if portal == nil {
		return nil, fmt.Errorf("portal is nil")
	}
	meta := portalMeta(portal)

	// Handle media messages based on type
	switch msg.Content.MsgType {
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		return oc.handleMediaMessage(ctx, msg, portal, meta, msg.Content.MsgType)
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		// Continue to text handling below
	default:
		return nil, fmt.Errorf("%s messages are not supported", msg.Content.MsgType)
	}
	body := strings.TrimSpace(msg.Content.Body)
	if body == "" {
		return nil, fmt.Errorf("empty messages are not supported")
	}

	promptMessages, err := oc.buildPrompt(ctx, portal, meta, body)
	if err != nil {
		return nil, err
	}
	userMessage := &database.Message{
		ID:       networkid.MessageID(fmt.Sprintf("mx:%s", string(msg.Event.ID))),
		Room:     portal.PortalKey,
		SenderID: humanUserID(oc.UserLogin.ID),
		Metadata: &MessageMetadata{
			Role: "user",
			Body: body,
		},
		Timestamp: time.Now(),
	}

	dbMsg, isPending := oc.dispatchOrQueue(ctx, msg.Event, portal, meta, userMessage, pendingMessage{
		Event:       msg.Event,
		Portal:      portal,
		Meta:        meta,
		Type:        pendingTypeText,
		MessageBody: body,
	}, promptMessages)

	return &bridgev2.MatrixMessageResponse{
		DB:      dbMsg,
		Pending: isPending,
	}, nil
}

// HandleMatrixEdit handles edits to previously sent messages
func (oc *AIClient) HandleMatrixEdit(ctx context.Context, edit *bridgev2.MatrixEdit) error {
	if edit.Content == nil || edit.EditTarget == nil {
		return fmt.Errorf("invalid edit: missing content or target")
	}

	portal := edit.Portal
	if portal == nil {
		return fmt.Errorf("portal is nil")
	}
	meta := portalMeta(portal)

	// Get the new message body
	newBody := strings.TrimSpace(edit.Content.Body)
	if newBody == "" {
		return fmt.Errorf("empty edit body")
	}

	// Update the message metadata with the new content
	msgMeta := messageMeta(edit.EditTarget)
	if msgMeta == nil {
		msgMeta = &MessageMetadata{}
		edit.EditTarget.Metadata = msgMeta
	}
	msgMeta.Body = newBody

	// Persist the updated metadata
	if err := oc.UserLogin.Bridge.DB.Message.Update(ctx, edit.EditTarget); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to persist edited message metadata")
	}

	// Only regenerate if this was a user message
	if msgMeta.Role != "user" {
		// Just update the content, don't regenerate
		return nil
	}

	oc.log.Info().
		Str("message_id", string(edit.EditTarget.ID)).
		Str("new_body", newBody).
		Msg("User edited message, regenerating response")

	// Find the assistant response that came after this message
	// We'll delete it and regenerate
	err := oc.regenerateFromEdit(ctx, edit.Event, portal, meta, edit.EditTarget, newBody)
	if err != nil {
		oc.log.Err(err).Msg("Failed to regenerate response after edit")
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Failed to regenerate response: %v", err))
	}

	return nil
}

// regenerateFromEdit regenerates the AI response based on an edited user message
func (oc *AIClient) regenerateFromEdit(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	editedMessage *database.Message,
	newBody string,
) error {
	// Get messages in the portal to find the assistant response after the edited message
	messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 50)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	// Find the assistant response that came after the edited message
	// Messages come newest-first from GetLastNInPortal, so lower indices are newer
	var assistantResponse *database.Message

	// First find the index of the edited message
	editedIdx := -1
	for i, msg := range messages {
		if msg.ID == editedMessage.ID {
			editedIdx = i
			break
		}
	}

	if editedIdx > 0 {
		// Search toward newer messages (lower indices) for assistant response
		for i := editedIdx - 1; i >= 0; i-- {
			msgMeta := messageMeta(messages[i])
			if msgMeta != nil && msgMeta.Role == "assistant" {
				assistantResponse = messages[i]
				break
			}
		}
	}

	// Build the prompt with the edited message included
	// We need to rebuild from scratch up to the edited message
	promptMessages, err := oc.buildPromptUpToMessage(ctx, portal, meta, editedMessage.ID, newBody)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	// If we found an assistant response, we'll redact/edit it
	if assistantResponse != nil {
		// Try to redact the old response
		if assistantResponse.MXID != "" {
			intent, _ := portal.GetIntentFor(ctx, bridgev2.EventSender{IsFromMe: true}, oc.UserLogin, bridgev2.RemoteEventMessageRemove)
			if intent != nil {
				_, _ = intent.SendMessage(ctx, portal.MXID, event.EventRedaction, &event.Content{
					Parsed: &event.RedactionEventContent{
						Redacts: assistantResponse.MXID,
					},
				}, nil)
			}
		}
		// Clean up database record to prevent orphaned messages
		if err := oc.UserLogin.Bridge.DB.Message.Delete(ctx, assistantResponse.RowID); err != nil {
			oc.log.Warn().Err(err).Str("msg_id", string(assistantResponse.ID)).Msg("Failed to delete redacted message from database")
		}
	}

	oc.dispatchOrQueueWithStatus(ctx, evt, portal, meta, pendingMessage{
		Event:       evt,
		Portal:      portal,
		Meta:        meta,
		Type:        pendingTypeEditRegenerate,
		MessageBody: newBody,
		TargetMsgID: editedMessage.ID,
	}, promptMessages)

	return nil
}

// mediaConfig describes how to handle a specific media type
type mediaConfig struct {
	msgType         pendingMessageType
	capabilityCheck func(*ModelCapabilities) bool
	capabilityName  string
	defaultCaption  string
	bodySuffix      string
	defaultMimeType string
}

var mediaConfigs = map[event.MessageType]mediaConfig{
	event.MsgImage: {
		msgType:         pendingTypeImage,
		capabilityCheck: func(c *ModelCapabilities) bool { return c.SupportsVision },
		capabilityName:  "image analysis",
		defaultCaption:  "What's in this image?",
		bodySuffix:      " [image]",
	},
	event.MsgAudio: {
		msgType:         pendingTypeAudio,
		capabilityCheck: func(c *ModelCapabilities) bool { return c.SupportsAudio },
		capabilityName:  "audio input",
		defaultCaption:  "Please transcribe or analyze this audio.",
		bodySuffix:      " [audio]",
		defaultMimeType: "audio/mpeg",
	},
	event.MsgVideo: {
		msgType:         pendingTypeVideo,
		capabilityCheck: func(c *ModelCapabilities) bool { return c.SupportsVideo },
		capabilityName:  "video input",
		defaultCaption:  "Please analyze this video.",
		bodySuffix:      " [video]",
	},
}

// pdfConfig is handled separately due to special OpenRouter capability check
var pdfConfig = mediaConfig{
	msgType:         pendingTypePDF,
	capabilityCheck: func(c *ModelCapabilities) bool { return c.SupportsPDF },
	capabilityName:  "PDF analysis",
	defaultCaption:  "Please analyze this PDF document.",
	bodySuffix:      " [PDF]",
	defaultMimeType: "application/pdf",
}

// handleMediaMessage processes media messages (image, PDF, audio, video)
func (oc *AIClient) handleMediaMessage(
	ctx context.Context,
	msg *bridgev2.MatrixMessage,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	msgType event.MessageType,
) (*bridgev2.MatrixMessageResponse, error) {
	// Get config for this media type
	config, ok := mediaConfigs[msgType]
	isPDF := false

	// Handle PDF files (MsgFile with application/pdf MIME type)
	if msgType == event.MsgFile {
		mimeType := ""
		if msg.Content.Info != nil {
			mimeType = msg.Content.Info.MimeType
		}
		if mimeType == "application/pdf" {
			config = pdfConfig
			isPDF = true
			ok = true
		}
	}

	if !ok {
		return nil, fmt.Errorf("unsupported media type: %s", msgType)
	}

	// Check capability (PDF has special OpenRouter handling via file-parser plugin)
	capabilityOK := config.capabilityCheck(&meta.Capabilities)
	if isPDF && !capabilityOK && oc.isOpenRouterProvider() {
		capabilityOK = true // OpenRouter supports PDF via file-parser plugin
	}
	if !capabilityOK {
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf(
			"The current model (%s) does not support %s. Please switch to a capable model using /model.",
			oc.effectiveModel(meta), config.capabilityName,
		))
		return &bridgev2.MatrixMessageResponse{}, nil
	}

	// Get the media URL
	mediaURL := msg.Content.URL
	if mediaURL == "" && msg.Content.File != nil {
		mediaURL = msg.Content.File.URL
	}
	if mediaURL == "" {
		return nil, fmt.Errorf("%s message has no URL", msgType)
	}

	// Get MIME type
	mimeType := config.defaultMimeType
	if msg.Content.Info != nil && msg.Content.Info.MimeType != "" {
		mimeType = msg.Content.Info.MimeType
	}

	// Get caption (body is usually the filename or caption)
	caption := strings.TrimSpace(msg.Content.Body)
	if caption == "" || (msg.Content.Info != nil && caption == msg.Content.Info.MimeType) {
		caption = config.defaultCaption
	}

	// Get encrypted file info if present (for E2EE rooms)
	var encryptedFile *event.EncryptedFileInfo
	if msg.Content.File != nil {
		encryptedFile = msg.Content.File
	}

	// Build prompt with media
	promptMessages, err := oc.buildPromptWithMedia(ctx, portal, meta, caption, string(mediaURL), mimeType, encryptedFile, config.msgType)
	if err != nil {
		return nil, err
	}

	userMessage := &database.Message{
		ID:       networkid.MessageID(fmt.Sprintf("mx:%s", string(msg.Event.ID))),
		Room:     portal.PortalKey,
		SenderID: humanUserID(oc.UserLogin.ID),
		Metadata: &MessageMetadata{
			Role: "user",
			Body: caption + config.bodySuffix,
		},
		Timestamp: time.Now(),
	}

	dbMsg, isPending := oc.dispatchOrQueue(ctx, msg.Event, portal, meta, userMessage, pendingMessage{
		Event:         msg.Event,
		Portal:        portal,
		Meta:          meta,
		Type:          config.msgType,
		MessageBody:   caption,
		MediaURL:      string(mediaURL),
		MimeType:      mimeType,
		EncryptedFile: encryptedFile,
	}, promptMessages)

	return &bridgev2.MatrixMessageResponse{
		DB:      dbMsg,
		Pending: isPending,
	}, nil
}

// savePortalQuiet saves portal and logs errors without failing
func (oc *AIClient) savePortalQuiet(ctx context.Context, portal *bridgev2.Portal, action string) {
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Str("action", action).Msg("Failed to save portal")
	}
}

// handleToolsCommand handles the /tools command for per-tool management
func (oc *AIClient) handleToolsCommand(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	arg string,
) {
	runCtx := oc.backgroundContext(ctx)

	// Get provider info
	loginMeta := loginMetadata(oc.UserLogin)
	isOpenRouter := loginMeta.Provider == ProviderOpenRouter || loginMeta.Provider == ProviderBeeper

	// No args - show status
	if arg == "" {
		oc.showToolsStatus(runCtx, portal, meta, isOpenRouter)
		return
	}

	parts := strings.SplitN(arg, " ", 2)
	action := strings.ToLower(parts[0])
	var toolName string
	if len(parts) > 1 {
		toolName = strings.ToLower(parts[1])
	}

	switch action {
	case "on", "enable", "true", "1":
		if toolName == "" {
			// Enable all tools
			oc.setAllTools(runCtx, portal, meta, true, isOpenRouter)
		} else {
			// Enable specific tool
			oc.setToolEnabled(runCtx, portal, meta, toolName, true, isOpenRouter)
		}
	case "off", "disable", "false", "0":
		if toolName == "" {
			// Disable all tools
			oc.setAllTools(runCtx, portal, meta, false, isOpenRouter)
		} else {
			// Disable specific tool
			oc.setToolEnabled(runCtx, portal, meta, toolName, false, isOpenRouter)
		}
	case "list":
		oc.showToolsStatus(runCtx, portal, meta, isOpenRouter)
	default:
		oc.sendSystemNotice(runCtx, portal, "Usage:\n"+
			"• /tools - Show current tool status\n"+
			"• /tools on - Enable all tools\n"+
			"• /tools off - Disable all tools\n"+
			"• /tools on <tool> - Enable specific tool\n"+
			"• /tools off <tool> - Disable specific tool\n"+
			"• /tools list - List available tools")
	}
}

// showToolsStatus displays the current status of all tools
func (oc *AIClient) showToolsStatus(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, isOpenRouter bool) {
	var sb strings.Builder
	sb.WriteString("Tool Status:\n\n")

	supportsTools := meta.Capabilities.SupportsToolCalling

	// Builtin tools
	sb.WriteString("Builtin Tools:\n")
	for _, tool := range BuiltinTools() {
		enabled := oc.isToolEnabled(meta, tool.Name)
		status := "✗"
		if enabled {
			status = "✓"
		}
		availability := ""
		if !supportsTools {
			availability = " (model doesn't support tools)"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s: %s%s\n", status, tool.Name, tool.Description, availability))
	}

	// OpenRouter plugin
	if isOpenRouter {
		sb.WriteString("\nPlugins:\n")
		onlineEnabled := oc.isToolEnabled(meta, "online")
		status := "✗"
		if onlineEnabled {
			status = "✓"
		}
		sb.WriteString(fmt.Sprintf("  [%s] online: OpenRouter web search plugin (:online suffix)\n", status))
	}

	// Provider tools
	sb.WriteString("\nProvider Tools:\n")
	wsStatus := "✗"
	if oc.isToolEnabled(meta, ToolNameWebSearchProvider) {
		wsStatus = "✓"
	}
	sb.WriteString(fmt.Sprintf("  [%s] web_search_provider: Native provider web search\n", wsStatus))

	ciStatus := "✗"
	if oc.isToolEnabled(meta, ToolNameCodeInterpreter) {
		ciStatus = "✓"
	}
	sb.WriteString(fmt.Sprintf("  [%s] code_interpreter: Execute Python code\n", ciStatus))

	if !supportsTools {
		sb.WriteString(fmt.Sprintf("\nNote: Current model (%s) may not support tool calling.\n", oc.effectiveModel(meta)))
	}

	oc.sendSystemNotice(ctx, portal, sb.String())
}

// setAllTools enables or disables all tools
func (oc *AIClient) setAllTools(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, enabled bool, isOpenRouter bool) {
	loginMeta := loginMetadata(oc.UserLogin)
	ensureToolsConfig(meta, loginMeta.Provider)

	// Set all tools to the same state
	for name, entry := range meta.ToolsConfig.Tools {
		if entry == nil {
			continue
		}
		// Skip online plugin for non-OpenRouter
		if name == ToolNameOnline && !isOpenRouter {
			continue
		}
		entry.Enabled = &enabled
	}

	// If enabling online, disable builtin web_search (they overlap)
	if isOpenRouter && enabled {
		if wsEntry, ok := meta.ToolsConfig.Tools[ToolNameWebSearch]; ok && wsEntry != nil {
			wsEntry.Enabled = ptr.Ptr(false)
		}
	}

	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after tools change")
	}
	if err := oc.BroadcastRoomState(ctx, portal); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to broadcast room state after tools change")
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}
	oc.sendSystemNotice(ctx, portal, fmt.Sprintf("All tools %s.", status))
}

// setToolEnabled enables or disables a specific tool
func (oc *AIClient) setToolEnabled(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, toolName string, enabled bool, isOpenRouter bool) {
	loginMeta := loginMetadata(oc.UserLogin)
	ensureToolsConfig(meta, loginMeta.Provider)

	// Normalize tool name
	normalizedName := normalizeToolName(toolName)

	// Check if tool exists
	entry, ok := meta.ToolsConfig.Tools[normalizedName]
	if !ok || entry == nil {
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Unknown tool: %s. Available: calculator, web_search, online, web_search_provider, code_interpreter", toolName))
		return
	}

	// Validate online plugin for non-OpenRouter
	if normalizedName == ToolNameOnline && !isOpenRouter {
		oc.sendSystemNotice(ctx, portal, "The 'online' plugin is only available with OpenRouter.")
		return
	}

	// Apply the toggle
	entry.Enabled = &enabled

	// If enabling online, disable builtin web_search to avoid duplication
	if normalizedName == ToolNameOnline && enabled {
		if wsEntry, ok := meta.ToolsConfig.Tools[ToolNameWebSearch]; ok && wsEntry != nil {
			wsEntry.Enabled = ptr.Ptr(false)
		}
	}

	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after tool change")
	}
	if err := oc.BroadcastRoomState(ctx, portal); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to broadcast room state after tool change")
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}
	oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Tool '%s' %s.", toolName, status))
}

// handleRegenerate regenerates the last AI response
func (oc *AIClient) handleRegenerate(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) {
	runCtx := oc.backgroundContext(ctx)
	runCtx = oc.log.WithContext(runCtx)

	// Get message history
	history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(runCtx, portal.PortalKey, 10)
	if err != nil || len(history) == 0 {
		oc.sendSystemNotice(runCtx, portal, "No messages to regenerate from.")
		return
	}

	// Find the last user message
	var lastUserMessage *database.Message
	for _, msg := range history {
		msgMeta := messageMeta(msg)
		if msgMeta != nil && msgMeta.Role == "user" {
			lastUserMessage = msg
			break
		}
	}

	if lastUserMessage == nil {
		oc.sendSystemNotice(runCtx, portal, "No user message found to regenerate from.")
		return
	}

	userMeta := messageMeta(lastUserMessage)
	if userMeta == nil || userMeta.Body == "" {
		oc.sendSystemNotice(runCtx, portal, "Cannot regenerate: message content not available.")
		return
	}

	oc.sendSystemNotice(runCtx, portal, "Regenerating response...")

	// Build prompt excluding the old assistant response
	prompt, err := oc.buildPromptForRegenerate(runCtx, portal, meta, userMeta.Body)
	if err != nil {
		oc.sendSystemNotice(runCtx, portal, "Failed to regenerate: "+err.Error())
		return
	}

	oc.dispatchOrQueueWithStatus(runCtx, evt, portal, meta, pendingMessage{
		Event:       evt,
		Portal:      portal,
		Meta:        meta,
		Type:        pendingTypeRegenerate,
		MessageBody: userMeta.Body,
	}, prompt)
}

// buildPromptForRegenerate builds a prompt for regeneration, excluding the last assistant message
func (oc *AIClient) buildPromptForRegenerate(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	latestUserBody string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}

	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit+2)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}

		// Skip the most recent messages (last user and assistant) and build from older history
		skippedUser := false
		skippedAssistant := false
		for _, msg := range history {
			msgMeta := messageMeta(msg)
			// Skip commands and non-conversation messages
			if !shouldIncludeInHistory(msgMeta) {
				continue
			}

			// Skip the last user message and last assistant message
			if !skippedUser && msgMeta.Role == "user" {
				skippedUser = true
				continue
			}
			if !skippedAssistant && msgMeta.Role == "assistant" {
				skippedAssistant = true
				continue
			}

			switch msgMeta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(msgMeta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(msgMeta.Body))
			}
		}

		// Reverse to get chronological order (skip system message at index 0 if present)
		startIdx := 0
		if systemPrompt != "" && len(prompt) > 0 {
			startIdx = 1
		}
		for i, j := len(prompt)-1, startIdx; i > j; i, j = i-1, j+1 {
			prompt[i], prompt[j] = prompt[j], prompt[i]
		}
	}

	prompt = append(prompt, openai.UserMessage(latestUserBody))
	return prompt, nil
}

package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
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
	case event.MsgImage, event.MsgVideo, event.MsgAudio:
		return oc.handleMediaMessage(ctx, msg, portal, meta, msg.Content.MsgType)
	case event.MsgFile:
		// Check if it's a PDF
		mimeType := ""
		if msg.Content.Info != nil {
			mimeType = msg.Content.Info.MimeType
		}
		if mimeType == "application/pdf" {
			return oc.handlePDFMessage(ctx, msg, portal, meta)
		}
		return nil, fmt.Errorf("unsupported file type: %s", mimeType)
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		// Continue to text handling below
	default:
		return nil, fmt.Errorf("%s messages are not supported", msg.Content.MsgType)
	}
	body := strings.TrimSpace(msg.Content.Body)
	if body == "" {
		return nil, fmt.Errorf("empty messages are not supported")
	}

	// Check for commands
	if handled := oc.handleCommand(ctx, msg.Event, portal, meta, body); handled {
		// Return empty response - framework will send SUCCESS immediately
		// No DB message needed since commands aren't chat messages
		return &bridgev2.MatrixMessageResponse{}, nil
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
	if !ok {
		return nil, fmt.Errorf("unsupported media type: %s", msgType)
	}

	// Check capability
	if !config.capabilityCheck(&meta.Capabilities) {
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

// handlePDFMessage processes a PDF file message for PDF-capable models
func (oc *AIClient) handlePDFMessage(
	ctx context.Context,
	msg *bridgev2.MatrixMessage,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) (*bridgev2.MatrixMessageResponse, error) {
	// Check if model supports PDF
	// OpenRouter/Beeper: all models support PDF via file-parser plugin
	if !meta.Capabilities.SupportsPDF && !oc.isOpenRouterProvider() {
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf(
			"The current model (%s) does not support PDF analysis. "+
				"Please switch to a PDF-capable model using /model.",
			oc.effectiveModel(meta),
		))
		return &bridgev2.MatrixMessageResponse{}, nil
	}

	// Get the file URL from the message
	fileURL := msg.Content.URL
	if fileURL == "" && msg.Content.File != nil {
		fileURL = msg.Content.File.URL
	}
	if fileURL == "" {
		return nil, fmt.Errorf("PDF message has no URL")
	}

	// Get encrypted file info if present (for E2EE rooms)
	var encryptedFile *event.EncryptedFileInfo
	if msg.Content.File != nil {
		encryptedFile = msg.Content.File
	}

	// Get MIME type
	mimeType := "application/pdf"
	if msg.Content.Info != nil && msg.Content.Info.MimeType != "" {
		mimeType = msg.Content.Info.MimeType
	}

	// Get caption (body is usually the filename or caption)
	caption := strings.TrimSpace(msg.Content.Body)
	if caption == "" || (msg.Content.Info != nil && caption == msg.Content.Info.MimeType) {
		caption = "Please analyze this PDF document."
	}

	// Build prompt with PDF
	promptMessages, err := oc.buildPromptWithMedia(ctx, portal, meta, caption, string(fileURL), mimeType, encryptedFile, pendingTypePDF)
	if err != nil {
		return nil, err
	}

	userMessage := &database.Message{
		ID:       networkid.MessageID(fmt.Sprintf("mx:%s", string(msg.Event.ID))),
		Room:     portal.PortalKey,
		SenderID: humanUserID(oc.UserLogin.ID),
		Metadata: &MessageMetadata{
			Role: "user",
			Body: caption + " [PDF]",
		},
		Timestamp: time.Now(),
	}

	dbMsg, isPending := oc.dispatchOrQueue(ctx, msg.Event, portal, meta, userMessage, pendingMessage{
		Event:         msg.Event,
		Portal:        portal,
		Meta:          meta,
		Type:          pendingTypePDF,
		MessageBody:   caption,
		MediaURL:      string(fileURL),
		MimeType:      mimeType,
		EncryptedFile: encryptedFile,
	}, promptMessages)

	return &bridgev2.MatrixMessageResponse{
		DB:      dbMsg,
		Pending: isPending,
	}, nil
}

// handleCommand checks if the message is a command and handles it
// Returns true if the message was a command and was handled
func (oc *AIClient) handleCommand(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	body string,
) bool {
	// Check for slash commands
	if strings.HasPrefix(body, "/") {
		return oc.handleSlashCommand(ctx, evt, portal, meta, body)
	}

	// Check for regenerate command
	prefix := oc.connector.Config.Bridge.CommandPrefix
	if strings.HasPrefix(body, prefix+" regenerate") || body == prefix+" regenerate" ||
		body == "!regenerate" || body == "/regenerate" {
		go oc.handleRegenerate(ctx, evt, portal, meta)
		return true
	}

	return false
}

// handleSlashCommand handles slash commands like /model, /temp, /prompt
func (oc *AIClient) handleSlashCommand(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	body string,
) bool {
	parts := strings.SplitN(body, " ", 2)
	cmd := strings.ToLower(parts[0])
	var arg string
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/model":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current model: %s", oc.effectiveModel(meta)))
			return true
		}
		// Validate model
		valid, err := oc.validateModel(ctx, arg)
		if err != nil || !valid {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Invalid model: %s", arg))
			return true
		}
		// Update model
		meta.Model = arg
		meta.Capabilities = getModelCapabilities(arg, oc.findModelInfo(arg))
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after model change")
		}
		// Ensure the new model's ghost has its display name set
		oc.ensureGhostDisplayName(ctx, arg)
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Model changed to: %s", arg))
		return true

	case "/temp", "/temperature":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current temperature: %.2f", oc.effectiveTemperature(meta)))
			return true
		}
		var temp float64
		if _, err := fmt.Sscanf(arg, "%f", &temp); err != nil || temp < 0 || temp > 2 {
			oc.sendSystemNotice(ctx, portal, "Invalid temperature. Must be between 0 and 2.")
			return true
		}
		meta.Temperature = temp
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after temperature change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Temperature set to: %.2f", temp))
		return true

	case "/prompt", "/system":
		if arg == "" {
			current := oc.effectivePrompt(meta)
			if current == "" {
				current = "(none)"
			} else if len(current) > 100 {
				current = current[:100] + "..."
			}
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current system prompt: %s", current))
			return true
		}
		meta.SystemPrompt = arg
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after prompt change")
		}
		oc.sendSystemNotice(ctx, portal, "System prompt updated.")
		return true

	case "/context":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current context limit: %d messages", oc.historyLimit(meta)))
			return true
		}
		var limit int
		if _, err := fmt.Sscanf(arg, "%d", &limit); err != nil || limit < 1 || limit > 100 {
			oc.sendSystemNotice(ctx, portal, "Invalid context limit. Must be between 1 and 100.")
			return true
		}
		meta.MaxContextMessages = limit
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after context change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Context limit set to: %d messages", limit))
		return true

	case "/tokens", "/maxtokens":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current max tokens: %d", oc.effectiveMaxTokens(meta)))
			return true
		}
		var tokens int
		if _, err := fmt.Sscanf(arg, "%d", &tokens); err != nil || tokens < 1 || tokens > 16384 {
			oc.sendSystemNotice(ctx, portal, "Invalid max tokens. Must be between 1 and 16384.")
			return true
		}
		meta.MaxCompletionTokens = tokens
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after tokens change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Max tokens set to: %d", tokens))
		return true

	case "/config":
		mode := meta.ConversationMode
		if mode == "" {
			mode = "messages"
		}
		config := fmt.Sprintf(
			"Current configuration:\n"+
				"• Model: %s\n"+
				"• Temperature: %.2f\n"+
				"• Context: %d messages\n"+
				"• Max tokens: %d\n"+
				"• Vision: %v\n"+
				"• Mode: %s",
			oc.effectiveModel(meta),
			oc.effectiveTemperature(meta),
			oc.historyLimit(meta),
			oc.effectiveMaxTokens(meta),
			meta.Capabilities.SupportsVision,
			mode,
		)
		oc.sendSystemNotice(ctx, portal, config)
		return true

	case "/tools":
		go oc.handleToolsCommand(ctx, portal, meta, arg)
		return true

	case "/mode":
		mode := meta.ConversationMode
		if mode == "" {
			mode = "messages"
		}
		if arg == "" {
			modeHelp := "Conversation modes:\n" +
				"• messages - Build full message history for each request (default)\n" +
				"• responses - Use OpenAI's previous_response_id for context chaining\n\n" +
				"Current mode: " + mode
			oc.sendSystemNotice(ctx, portal, modeHelp)
			return true
		}
		newMode := strings.ToLower(arg)
		if newMode != "messages" && newMode != "responses" {
			oc.sendSystemNotice(ctx, portal, "Invalid mode. Use 'messages' or 'responses'.")
			return true
		}
		meta.ConversationMode = newMode
		if newMode == "messages" {
			meta.LastResponseID = "" // Clear when switching to messages mode
		}
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after mode change")
		}
		if err := oc.BroadcastRoomState(ctx, portal); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to broadcast room state after mode change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Conversation mode set to: %s", newMode))
		return true

	case "/help":
		help := "Available commands:\n" +
			"• /model [name] - Get or set the AI model\n" +
			"• /temp [0-2] - Get or set temperature\n" +
			"• /prompt [text] - Get or set system prompt\n" +
			"• /context [1-100] - Get or set context message limit\n" +
			"• /tokens [1-16384] - Get or set max completion tokens\n" +
			"• /tools [on|off] [tool] - Enable/disable tools (per-tool or all)\n" +
			"• /mode [messages|responses] - Set conversation context mode\n" +
			"• /new [model] - Create a new chat (uses current model if none specified)\n" +
			"• /fork [event_id] - Fork conversation to a new chat\n" +
			"• /config - Show current configuration\n" +
			"• /regenerate - Regenerate the last response\n" +
			"• /help - Show this help message"
		oc.sendSystemNotice(ctx, portal, help)
		return true

	case "/fork":
		go oc.handleFork(ctx, evt, portal, meta, arg)
		return true

	case "/new":
		go oc.handleNewChat(ctx, evt, portal, meta, arg)
		return true

	case "/regenerate":
		go oc.handleRegenerate(ctx, evt, portal, meta)
		return true
	}

	return false
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
	if meta.ToolsConfig.WebSearchProvider || meta.WebSearchEnabled {
		wsStatus = "✓"
	}
	sb.WriteString(fmt.Sprintf("  [%s] web_search_provider: Native provider web search\n", wsStatus))

	ciStatus := "✗"
	if meta.ToolsConfig.CodeInterpreter || meta.CodeInterpreterEnabled {
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
	meta.ToolsConfig.Calculator = &enabled
	meta.ToolsConfig.WebSearch = &enabled

	if isOpenRouter {
		meta.ToolsConfig.UseOpenRouterOnline = &enabled
		// If enabling online, disable builtin web_search (they overlap)
		if enabled {
			meta.ToolsConfig.WebSearch = ptrFalse()
		}
	}

	// Legacy field for backwards compatibility
	meta.ToolsEnabled = enabled

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
	validTool := true
	switch toolName {
	case "calculator", "calc":
		meta.ToolsConfig.Calculator = &enabled
	case "web_search", "websearch", "search":
		meta.ToolsConfig.WebSearch = &enabled
	case "online":
		if isOpenRouter {
			meta.ToolsConfig.UseOpenRouterOnline = &enabled
			// If enabling online, disable builtin web_search to avoid duplication
			if enabled {
				meta.ToolsConfig.WebSearch = ptrFalse()
			}
		} else {
			oc.sendSystemNotice(ctx, portal, "The 'online' plugin is only available with OpenRouter.")
			return
		}
	case "web_search_provider", "websearchprovider", "provider_search":
		meta.ToolsConfig.WebSearchProvider = enabled
	case "code_interpreter", "codeinterpreter", "interpreter":
		meta.ToolsConfig.CodeInterpreter = enabled
	default:
		validTool = false
	}

	if !validTool {
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Unknown tool: %s. Available: calculator, web_search, online, web_search_provider, code_interpreter", toolName))
		return
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

// ptrFalse returns a pointer to false
func ptrFalse() *bool {
	f := false
	return &f
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

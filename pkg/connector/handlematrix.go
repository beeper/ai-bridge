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

	// Handle image messages for vision-capable models
	if msg.Content.MsgType == event.MsgImage {
		return oc.handleImageMessage(ctx, msg, portal, meta)
	}

	switch msg.Content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
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

	// Try to acquire room SYNCHRONOUSLY - no race condition
	if oc.acquireRoom(portal.MXID) {
		// Room acquired - framework will save message and send SUCCESS
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, msg.Event, portal, meta, promptMessages)
		}()

		return &bridgev2.MatrixMessageResponse{
			DB: userMessage,
		}, nil
	}

	// Room busy - handle message saving ourselves to control status
	userMessage.MXID = msg.Event.ID
	err = oc.UserLogin.Bridge.DB.Message.Insert(ctx, userMessage)
	if err != nil {
		oc.log.Err(err).Msg("Failed to save queued message to database")
		// Continue anyway - the message will still be processed
	}

	// Queue the message for later processing (prompt built fresh when processed)
	oc.queuePendingMessage(portal.MXID, pendingMessage{
		Event:       msg.Event,
		Portal:      portal,
		Meta:        meta,
		Type:        pendingTypeText,
		MessageBody: body,
	})

	// Send PENDING status - message shows "Sending..." in client
	oc.sendPendingStatus(ctx, portal, msg.Event, "Waiting for previous response")

	// Return Pending: true so framework doesn't override our PENDING status with SUCCESS
	return &bridgev2.MatrixMessageResponse{
		Pending: true,
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
	var assistantResponse *database.Message
	foundEditedMessage := false
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.ID == editedMessage.ID {
			foundEditedMessage = true
			continue
		}
		if foundEditedMessage {
			msgMeta := messageMeta(msg)
			if msgMeta != nil && msgMeta.Role == "assistant" {
				assistantResponse = msg
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

	// Dispatch a new completion with synchronous room acquisition
	if oc.acquireRoom(portal.MXID) {
		oc.sendSuccessStatus(ctx, portal, evt)
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, evt, portal, meta, promptMessages)
		}()
	} else {
		oc.queuePendingMessage(portal.MXID, pendingMessage{
			Event:       evt,
			Portal:      portal,
			Meta:        meta,
			Type:        pendingTypeEditRegenerate,
			MessageBody: newBody,
			TargetMsgID: editedMessage.ID,
		})
		oc.sendPendingStatus(ctx, portal, evt, "Waiting for previous response")
	}

	return nil
}

// handleImageMessage processes an image message for vision-capable models
func (oc *AIClient) handleImageMessage(
	ctx context.Context,
	msg *bridgev2.MatrixMessage,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) (*bridgev2.MatrixMessageResponse, error) {
	// Check if model supports vision
	if !meta.Capabilities.SupportsVision {
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf(
			"The current model (%s) does not support image analysis. "+
				"Please switch to a vision-capable model like gpt-4o or gpt-4-turbo using /model.",
			oc.effectiveModel(meta),
		))
		return nil, nil
	}

	// Get the image URL from the message
	imageURL := msg.Content.URL
	if imageURL == "" && msg.Content.File != nil {
		imageURL = msg.Content.File.URL
	}
	if imageURL == "" {
		return nil, fmt.Errorf("image message has no URL")
	}

	// Get caption (body is usually the filename or caption)
	caption := strings.TrimSpace(msg.Content.Body)
	if caption == "" || (msg.Content.Info != nil && caption == msg.Content.Info.MimeType) {
		caption = "What's in this image?"
	}

	// Build prompt with image
	promptMessages, err := oc.buildPromptWithImage(ctx, portal, meta, caption, string(imageURL))
	if err != nil {
		return nil, err
	}

	userMessage := &database.Message{
		ID:       networkid.MessageID(fmt.Sprintf("mx:%s", string(msg.Event.ID))),
		Room:     portal.PortalKey,
		SenderID: humanUserID(oc.UserLogin.ID),
		Metadata: &MessageMetadata{
			Role: "user",
			Body: caption + " [image]",
		},
		Timestamp: time.Now(),
	}

	// Try to acquire room SYNCHRONOUSLY - no race condition
	if oc.acquireRoom(portal.MXID) {
		// Room acquired - framework will save message and send SUCCESS
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, msg.Event, portal, meta, promptMessages)
		}()

		return &bridgev2.MatrixMessageResponse{
			DB: userMessage,
		}, nil
	}

	// Room busy - handle message saving ourselves to control status
	userMessage.MXID = msg.Event.ID
	err = oc.UserLogin.Bridge.DB.Message.Insert(ctx, userMessage)
	if err != nil {
		oc.log.Err(err).Msg("Failed to save queued image message to database")
	}

	// Queue the message for later processing (prompt built fresh when processed)
	oc.queuePendingMessage(portal.MXID, pendingMessage{
		Event:       msg.Event,
		Portal:      portal,
		Meta:        meta,
		Type:        pendingTypeImage,
		MessageBody: caption,
		ImageURL:    string(imageURL),
	})

	// Send PENDING status - message shows "Sending..." in client
	oc.sendPendingStatus(ctx, portal, msg.Event, "Waiting for previous response")

	// Return Pending: true so framework doesn't override our PENDING status with SUCCESS
	return &bridgev2.MatrixMessageResponse{
		Pending: true,
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
		if arg == "" {
			status := "disabled"
			if meta.ToolsEnabled {
				status = "enabled"
			}
			toolList := ""
			for _, tool := range BuiltinTools() {
				toolList += fmt.Sprintf("  - %s: %s\n", tool.Name, tool.Description)
			}
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Tools are currently %s.\n\nAvailable tools:\n%s\nUse /tools on or /tools off to toggle.", status, toolList))
			return true
		}
		switch strings.ToLower(arg) {
		case "on", "enable", "true", "1":
			meta.ToolsEnabled = true
			if err := portal.Save(ctx); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to save portal after enabling tools")
			}
			if err := oc.BroadcastRoomState(ctx, portal); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to broadcast room state after enabling tools")
			}
			oc.sendSystemNotice(ctx, portal, "Tools enabled. The AI can now use calculator and web search.")
		case "off", "disable", "false", "0":
			meta.ToolsEnabled = false
			if err := portal.Save(ctx); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to save portal after disabling tools")
			}
			if err := oc.BroadcastRoomState(ctx, portal); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to broadcast room state after disabling tools")
			}
			oc.sendSystemNotice(ctx, portal, "Tools disabled.")
		default:
			oc.sendSystemNotice(ctx, portal, "Invalid option. Use /tools on or /tools off.")
		}
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

	case "/cost":
		// Calculate cost from conversation history
		messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 1000)
		if err != nil {
			oc.sendSystemNotice(ctx, portal, "Failed to retrieve message history for cost calculation.")
			return true
		}
		var totalPromptTokens, totalCompletionTokens int64
		var totalCost float64
		model := oc.effectiveModel(meta)
		for _, msg := range messages {
			msgMeta := messageMeta(msg)
			if msgMeta != nil && msgMeta.Role == "assistant" {
				totalPromptTokens += msgMeta.PromptTokens
				totalCompletionTokens += msgMeta.CompletionTokens
				totalCost += CalculateCost(model, msgMeta.PromptTokens, msgMeta.CompletionTokens)
			}
		}
		costMsg := fmt.Sprintf(
			"Conversation cost (%s):\n"+
				"• Input tokens: %d\n"+
				"• Output tokens: %d\n"+
				"• Estimated cost: %s",
			model,
			totalPromptTokens,
			totalCompletionTokens,
			FormatCost(totalCost),
		)
		oc.sendSystemNotice(ctx, portal, costMsg)
		return true

	case "/help":
		help := "Available commands:\n" +
			"• /model [name] - Get or set the AI model\n" +
			"• /temp [0-2] - Get or set temperature\n" +
			"• /prompt [text] - Get or set system prompt\n" +
			"• /context [1-100] - Get or set context message limit\n" +
			"• /tokens [1-16384] - Get or set max completion tokens\n" +
			"• /tools [on|off] - Enable/disable function calling tools\n" +
			"• /mode [messages|responses] - Set conversation context mode\n" +
			"• /new [model] - Create a new chat (uses current model if none specified)\n" +
			"• /fork [event_id] - Fork conversation to a new chat\n" +
			"• /config - Show current configuration\n" +
			"• /cost - Show conversation token usage and cost\n" +
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

	// Dispatch new completion with synchronous room acquisition
	if oc.acquireRoom(portal.MXID) {
		oc.sendSuccessStatus(runCtx, portal, evt)
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(runCtx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(runCtx, evt, portal, meta, prompt)
		}()
	} else {
		oc.queuePendingMessage(portal.MXID, pendingMessage{
			Event:       evt,
			Portal:      portal,
			Meta:        meta,
			Type:        pendingTypeRegenerate,
			MessageBody: userMeta.Body,
		})
		oc.sendPendingStatus(runCtx, portal, evt, "Waiting for previous response")
	}
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

		// Reverse to get chronological order
		for i, j := len(prompt)-1, 0; i > j; i, j = i-1, j+1 {
			prompt[i], prompt[j] = prompt[j], prompt[i]
		}
	}

	prompt = append(prompt, openai.UserMessage(latestUserBody))
	return prompt, nil
}

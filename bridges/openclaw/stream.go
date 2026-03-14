package openclaw

import (
	"context"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/bridges/ai/msgconv"
	"github.com/beeper/agentremote/pkg/shared/citations"
	"github.com/beeper/agentremote/pkg/shared/maputil"
	"github.com/beeper/agentremote/pkg/shared/openclawconv"
	"github.com/beeper/agentremote/pkg/shared/streamui"
	bridgesdk "github.com/beeper/agentremote/sdk"
)

func openClawStreamPartTimestamp(part map[string]any) time.Time {
	if len(part) == 0 {
		return time.Time{}
	}
	if value, ok := maputil.NumberArg(part, "timestamp"); ok && value > 0 {
		return time.UnixMilli(int64(value))
	}
	return time.Time{}
}

func applyOpenClawStreamPartTimestamp(state *openClawStreamState, partType string, ts time.Time) {
	if state == nil || ts.IsZero() {
		return
	}
	tsMillis := ts.UnixMilli()
	if state.messageTS.IsZero() || ts.Before(state.messageTS) {
		state.messageTS = ts
	}
	switch partType {
	case "start":
		if state.startedAtMs == 0 || tsMillis < state.startedAtMs {
			state.startedAtMs = tsMillis
		}
	case "text-delta", "reasoning-delta":
		if state.startedAtMs == 0 || tsMillis < state.startedAtMs {
			state.startedAtMs = tsMillis
		}
		if state.firstTokenAtMs == 0 || tsMillis < state.firstTokenAtMs {
			state.firstTokenAtMs = tsMillis
		}
	case "abort", "error", "finish":
		if state.completedAtMs == 0 || tsMillis > state.completedAtMs {
			state.completedAtMs = tsMillis
		}
	}
}

func openClawStreamMessageTimestamp(state *openClawStreamState) time.Time {
	if state == nil {
		return time.Now()
	}
	if !state.messageTS.IsZero() {
		return state.messageTS
	}
	if state.startedAtMs > 0 {
		return time.UnixMilli(state.startedAtMs)
	}
	if state.firstTokenAtMs > 0 {
		return time.UnixMilli(state.firstTokenAtMs)
	}
	if state.completedAtMs > 0 {
		return time.UnixMilli(state.completedAtMs)
	}
	return time.Now()
}

func (oc *OpenClawClient) EmitStreamPart(ctx context.Context, portal *bridgev2.Portal, turnID, agentID, sessionKey string, part map[string]any) {
	if oc == nil || portal == nil || portal.MXID == "" || strings.TrimSpace(turnID) == "" || part == nil {
		return
	}
	if oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.Bot == nil {
		return
	}
	if oc.IsStreamShuttingDown() {
		return
	}

	turnID = strings.TrimSpace(turnID)
	agentID = openclawconv.StringsTrimDefault(agentID, "gateway")
	sessionKey = strings.TrimSpace(sessionKey)

	oc.StreamMu.Lock()
	state := oc.ensureStreamStateLocked(portal, turnID, agentID, sessionKey)
	if metadata, _ := part["messageMetadata"].(map[string]any); len(metadata) > 0 {
		oc.applyStreamMessageMetadata(state, metadata)
	}
	partType := strings.TrimSpace(stringValue(part["type"]))
	partTS := openClawStreamPartTimestamp(part)
	applyOpenClawStreamPartTimestamp(state, partType, partTS)
	if state.startedAtMs == 0 && partType == "start" {
		state.startedAtMs = time.Now().UnixMilli()
	}
	switch partType {
	case "text-delta":
		if delta := stringValue(part["delta"]); delta != "" {
			state.visible.WriteString(delta)
			state.accumulated.WriteString(delta)
			if state.firstTokenAtMs == 0 {
				state.firstTokenAtMs = time.Now().UnixMilli()
			}
		}
	case "reasoning-delta":
		if delta := stringValue(part["delta"]); delta != "" {
			state.accumulated.WriteString(delta)
			if state.firstTokenAtMs == 0 {
				state.firstTokenAtMs = time.Now().UnixMilli()
			}
		}
	case "error":
		if errText := strings.TrimSpace(stringValue(part["errorText"])); errText != "" {
			state.errorText = errText
		}
	case "abort":
		state.finishReason = openclawconv.StringsTrimDefault(stringValue(part["reason"]), "aborted")
	case "finish":
		if state.completedAtMs == 0 {
			state.completedAtMs = time.Now().UnixMilli()
		}
	}
	streamui.ApplyChunk(&state.ui, part)
	stream := state.stream
	if stream == nil {
		stream = oc.newSDKStream(ctx, portal, state)
		state.stream = stream
		if stream != nil {
			state.turn = stream.Turn()
		}
	}
	oc.StreamMu.Unlock()

	if oc.IsStreamShuttingDown() {
		return
	}
	if stream == nil {
		return
	}
	switch partType {
	case "start", "message-metadata":
		if metadata, _ := part["messageMetadata"].(map[string]any); len(metadata) > 0 {
			stream.Metadata(metadata)
		}
	case "start-step":
		stream.StepStart()
	case "finish-step":
		stream.StepFinish()
	case "text-delta":
		if delta := stringValue(part["delta"]); delta != "" {
			stream.TextDelta(delta)
		}
	case "reasoning-delta":
		if delta := stringValue(part["delta"]); delta != "" {
			stream.ReasoningDelta(delta)
		}
	case "tool-input-start":
		toolName := strings.TrimSpace(stringValue(part["toolName"]))
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		providerExecuted, _ := part["providerExecuted"].(bool)
		stream.TurnStream().EnsureToolInputStart(toolCallID, nil, bridgesdk.ToolInputOptions{
			ToolName:         toolName,
			ProviderExecuted: providerExecuted,
		})
	case "tool-input-delta":
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		inputTextDelta := stringValue(part["inputTextDelta"])
		providerExecuted, _ := part["providerExecuted"].(bool)
		stream.ToolInputDelta(toolCallID, inputTextDelta, providerExecuted)
	case "tool-input-available":
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		toolName := strings.TrimSpace(stringValue(part["toolName"]))
		providerExecuted, _ := part["providerExecuted"].(bool)
		stream.ToolInput(toolCallID, toolName, part["input"], providerExecuted)
	case "tool-output-available":
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		providerExecuted, _ := part["providerExecuted"].(bool)
		stream.TurnStream().ToolOutput(toolCallID, part["output"], bridgesdk.ToolOutputOptions{ProviderExecuted: providerExecuted})
	case "tool-output-error":
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		errorText := stringValue(part["errorText"])
		providerExecuted, _ := part["providerExecuted"].(bool)
		stream.TurnStream().ToolOutputError(toolCallID, errorText, providerExecuted)
	case "tool-output-denied":
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		stream.ToolDenied(toolCallID)
	case "tool-approval-request":
		approvalID := strings.TrimSpace(stringValue(part["approvalId"]))
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		stream.Approvals().EmitRequest(approvalID, toolCallID)
	case "tool-approval-response":
		approvalID := strings.TrimSpace(stringValue(part["approvalId"]))
		toolCallID := strings.TrimSpace(stringValue(part["toolCallId"]))
		approved, _ := part["approved"].(bool)
		reason := stringValue(part["reason"])
		stream.Approvals().Respond(approvalID, toolCallID, approved, reason)
	case "file":
		stream.File(stringValue(part["url"]), stringValue(part["mediaType"]))
	case "source-document":
		stream.SourceDocument(citations.SourceDocument{
			ID:        stringValue(part["sourceId"]),
			Title:     stringValue(part["title"]),
			MediaType: stringValue(part["mediaType"]),
			Filename:  stringValue(part["filename"]),
		})
	case "source-url":
		stream.SourceURL(stringValue(part["url"]), stringValue(part["title"]))
	case "error":
		stream.Error(stringValue(part["errorText"]))
	default:
		if strings.HasPrefix(partType, "data-") {
			stream.Emitter().Emit(stream.Context(), portal, part)
		}
	}
}

func (oc *OpenClawClient) FinishStream(turnID, finishReason string) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return
	}

	oc.StreamMu.Lock()
	state := oc.streamStates[turnID]
	var stream *bridgesdk.Stream
	if state != nil {
		stream = state.stream
		if state.finishReason == "" {
			state.finishReason = strings.TrimSpace(finishReason)
		}
		if state.completedAtMs == 0 {
			state.completedAtMs = openClawStreamMessageTimestamp(state).UnixMilli()
		}
	}
	oc.StreamMu.Unlock()

	oc.StreamMu.Lock()
	delete(oc.streamStates, turnID)
	oc.StreamMu.Unlock()

	if stream == nil {
		return
	}
	switch strings.TrimSpace(state.finishReason) {
	case "abort", "aborted":
		stream.Abort(openclawconv.StringsTrimDefault(state.finishReason, "aborted"))
	case "error":
		stream.EndWithError(openclawconv.StringsTrimDefault(state.errorText, "OpenClaw stream failed"))
	default:
		reason := openclawconv.StringsTrimDefault(state.finishReason, strings.TrimSpace(finishReason))
		stream.End(openclawconv.StringsTrimDefault(reason, "stop"))
	}
}

func (oc *OpenClawClient) newSDKStream(ctx context.Context, portal *bridgev2.Portal, state *openClawStreamState) *bridgesdk.Stream {
	if oc == nil || portal == nil || state == nil || oc.connector == nil || oc.connector.sdkConfig == nil {
		return nil
	}
	profile := oc.resolveAgentProfile(ctx, state.agentID, state.sessionKey, nil, nil)
	state.agentID = openclawconv.StringsTrimDefault(profile.AgentID, state.agentID)
	state.agentID = openclawconv.StringsTrimDefault(state.agentID, "gateway")
	agent := oc.sdkAgentForProfile(profile)
	sender := oc.senderForAgent(state.agentID, false)
	conv := bridgesdk.NewConversation(ctx, oc.UserLogin, portal, sender, oc.connector.sdkConfig, oc)
	_ = conv.EnsureRoomAgent(ctx, agent)
	stream := conv.Stream(ctx)
	stream.SetAgent(agent)
	stream.SetID(state.turnID)
	stream.SetSender(sender)
	stream.SetFinalMetadataProvider(bridgesdk.FinalMetadataProviderFunc(func(_ *bridgesdk.Turn, finishReason string) any {
		if strings.TrimSpace(finishReason) != "" {
			state.finishReason = strings.TrimSpace(finishReason)
		}
		if state.completedAtMs == 0 {
			state.completedAtMs = time.Now().UnixMilli()
		}
		return oc.buildStreamDBMetadata(state)
	}))
	return stream
}

func (oc *OpenClawClient) computeVisibleDelta(turnID, text string) string {
	turnID = strings.TrimSpace(turnID)
	text = strings.TrimSpace(text)
	if turnID == "" {
		return text
	}

	oc.StreamMu.Lock()
	defer oc.StreamMu.Unlock()
	state := oc.streamStates[turnID]
	if state == nil {
		state = &openClawStreamState{turnID: turnID}
		state.ui.TurnID = turnID
		oc.streamStates[turnID] = state
	}
	if text == state.lastVisibleText {
		return ""
	}
	prev := state.lastVisibleText
	state.lastVisibleText = text
	if prev == "" {
		return text
	}
	if strings.HasPrefix(text, prev) {
		return text[len(prev):]
	}
	return text
}

func (oc *OpenClawClient) isStreamActive(turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return false
	}
	oc.StreamMu.Lock()
	defer oc.StreamMu.Unlock()
	_, ok := oc.streamStates[turnID]
	return ok
}

func (oc *OpenClawClient) ensureStreamStateLocked(portal *bridgev2.Portal, turnID, agentID, sessionKey string) *openClawStreamState {
	state := oc.streamStates[turnID]
	if state == nil {
		state = &openClawStreamState{
			portal:     portal,
			turnID:     turnID,
			agentID:    agentID,
			sessionKey: sessionKey,
			role:       "assistant",
		}
		state.ui.TurnID = turnID
		oc.streamStates[turnID] = state
	}
	if state.portal == nil {
		state.portal = portal
	}
	if state.agentID == "" {
		state.agentID = agentID
	}
	if state.sessionKey == "" {
		state.sessionKey = sessionKey
	}
	if state.ui.TurnID == "" {
		state.ui.TurnID = turnID
	}
	if state.role == "" {
		state.role = "assistant"
	}
	return state
}

func (oc *OpenClawClient) applyStreamMessageMetadata(state *openClawStreamState, metadata map[string]any) {
	if state == nil || len(metadata) == 0 {
		return
	}
	if value := maputil.StringArg(metadata, "role"); value != "" {
		state.role = value
	}
	if value := maputil.StringArg(metadata, "session_id"); value != "" {
		state.sessionID = value
	}
	if value := maputil.StringArg(metadata, "session_key"); value != "" {
		state.sessionKey = value
	}
	if value := maputil.StringArg(metadata, "completion_id"); value != "" {
		state.runID = value
	}
	if value := maputil.StringArg(metadata, "agent_id"); value != "" {
		state.agentID = value
	}
	if value := maputil.StringArg(metadata, "finish_reason"); value != "" {
		state.finishReason = value
	}
	if value := maputil.StringArg(metadata, "error_text"); value != "" {
		state.errorText = value
	}
	if timing, _ := metadata["timing"].(map[string]any); len(timing) > 0 {
		if value, ok := maputil.NumberArg(timing, "started_at"); ok {
			state.startedAtMs = int64(value)
		}
		if value, ok := maputil.NumberArg(timing, "first_token_at"); ok {
			state.firstTokenAtMs = int64(value)
		}
		if value, ok := maputil.NumberArg(timing, "completed_at"); ok {
			state.completedAtMs = int64(value)
		}
	}
	if usage, _ := metadata["usage"].(map[string]any); len(usage) > 0 {
		usage = normalizeOpenClawUsage(usage)
		if value, ok := maputil.NumberArg(usage, "prompt_tokens"); ok {
			state.promptTokens = int64(value)
		}
		if value, ok := maputil.NumberArg(usage, "completion_tokens"); ok {
			state.completionTokens = int64(value)
		}
		if value, ok := maputil.NumberArg(usage, "reasoning_tokens"); ok {
			state.reasoningTokens = int64(value)
		}
		if value, ok := maputil.NumberArg(usage, "total_tokens"); ok {
			state.totalTokens = int64(value)
		}
	}
}

func (oc *OpenClawClient) currentCanonicalUIMessage(state *openClawStreamState) map[string]any {
	if state == nil {
		return nil
	}
	uiState := &state.ui
	if state.turn != nil && state.turn.UIState() != nil {
		uiState = state.turn.UIState()
	}
	uiMessage := streamui.SnapshotCanonicalUIMessage(uiState)
	update := msgconv.BuildUIMessageMetadata(msgconv.UIMessageMetadataParams{
		TurnID:           state.turnID,
		AgentID:          state.agentID,
		FinishReason:     state.finishReason,
		CompletionID:     state.runID,
		PromptTokens:     state.promptTokens,
		CompletionTokens: state.completionTokens,
		ReasoningTokens:  state.reasoningTokens,
		TotalTokens:      state.totalTokens,
		StartedAtMs:      state.startedAtMs,
		FirstTokenAtMs:   state.firstTokenAtMs,
		CompletedAtMs:    state.completedAtMs,
		IncludeUsage:     true,
	})
	if len(uiMessage) == 0 {
		return msgconv.BuildUIMessage(msgconv.UIMessageParams{
			TurnID:   state.turnID,
			Role:     openclawconv.StringsTrimDefault(state.role, "assistant"),
			Metadata: update,
		})
	}
	metadata, _ := uiMessage["metadata"].(map[string]any)
	uiMessage["metadata"] = msgconv.MergeUIMessageMetadata(metadata, update)
	return uiMessage
}

func (oc *OpenClawClient) buildStreamDBMetadata(state *openClawStreamState) *MessageMetadata {
	if state == nil {
		return nil
	}
	body := strings.TrimSpace(state.lastVisibleText)
	if body == "" {
		body = strings.TrimSpace(state.visible.String())
	}
	if body == "" {
		body = strings.TrimSpace(state.accumulated.String())
	}
	uiMessage := oc.currentCanonicalUIMessage(state)
	return &MessageMetadata{
		BaseMessageMetadata: agentremote.BaseMessageMetadata{
			Role:               openclawconv.StringsTrimDefault(state.role, "assistant"),
			Body:               body,
			TurnID:             state.turnID,
			AgentID:            state.agentID,
			FinishReason:       state.finishReason,
			PromptTokens:       state.promptTokens,
			CompletionTokens:   state.completionTokens,
			ReasoningTokens:    state.reasoningTokens,
			CanonicalSchema:    "ai-sdk-ui-message-v1",
			CanonicalUIMessage: uiMessage,
			ThinkingContent:    agentremote.CanonicalReasoningText(agentremote.NormalizeUIParts(uiMessage["parts"])),
			ToolCalls:          agentremote.CanonicalToolCalls(agentremote.NormalizeUIParts(uiMessage["parts"]), "openclaw"),
			GeneratedFiles:     agentremote.CanonicalGeneratedFiles(agentremote.NormalizeUIParts(uiMessage["parts"])),
			StartedAtMs:        state.startedAtMs,
			CompletedAtMs:      state.completedAtMs,
		},
		SessionID:      state.sessionID,
		SessionKey:     state.sessionKey,
		RunID:          state.runID,
		ErrorText:      state.errorText,
		TotalTokens:    state.totalTokens,
		FirstTokenAtMs: state.firstTokenAtMs,
	}
}

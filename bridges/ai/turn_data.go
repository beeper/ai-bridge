package ai

import (
	"strings"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/pkg/shared/streamui"
	"github.com/beeper/agentremote/sdk"
)

func canonicalTurnData(meta *MessageMetadata) (sdk.TurnData, bool) {
	if meta == nil || meta.CanonicalTurnSchema != sdk.CanonicalTurnDataSchemaV1 || len(meta.CanonicalTurnData) == 0 {
		return sdk.TurnData{}, false
	}
	return sdk.DecodeTurnData(meta.CanonicalTurnData)
}

func promptMessagesFromTurnData(td sdk.TurnData) []PromptMessage {
	return bridgePromptMessagesFromSDK(sdk.PromptMessagesFromTurnData(td))
}

func turnDataFromUserPromptMessages(messages []PromptMessage) (sdk.TurnData, bool) {
	return sdk.TurnDataFromUserPromptMessages(sdkPromptMessagesFromBridge(messages))
}

func bridgePromptMessagesFromSDK(messages []sdk.PromptMessage) []PromptMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]PromptMessage, 0, len(messages))
	for _, msg := range messages {
		next := PromptMessage{
			Role:       PromptRole(msg.Role),
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			IsError:    msg.IsError,
		}
		next.Blocks = make([]PromptBlock, 0, len(msg.Blocks))
		for _, block := range msg.Blocks {
			next.Blocks = append(next.Blocks, PromptBlock{
				Type:              PromptBlockType(block.Type),
				Text:              block.Text,
				ImageURL:          block.ImageURL,
				MimeType:          block.MimeType,
				FileURL:           block.FileURL,
				Filename:          block.Filename,
				ToolCallID:        block.ToolCallID,
				ToolName:          block.ToolName,
				ToolCallArguments: block.ToolCallArguments,
			})
		}
		out = append(out, next)
	}
	return out
}

func sdkPromptMessagesFromBridge(messages []PromptMessage) []sdk.PromptMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]sdk.PromptMessage, 0, len(messages))
	for _, msg := range messages {
		next := sdk.PromptMessage{
			Role:       sdk.PromptRole(msg.Role),
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			IsError:    msg.IsError,
		}
		next.Blocks = make([]sdk.PromptBlock, 0, len(msg.Blocks))
		for _, block := range msg.Blocks {
			imageURL := strings.TrimSpace(block.ImageURL)
			if imageURL == "" && strings.TrimSpace(block.ImageB64) != "" {
				mimeType := block.MimeType
				if mimeType == "" {
					mimeType = "image/jpeg"
				}
				imageURL = buildDataURL(mimeType, block.ImageB64)
			}
			next.Blocks = append(next.Blocks, sdk.PromptBlock{
				Type:              sdk.PromptBlockType(block.Type),
				Text:              block.Text,
				ImageURL:          imageURL,
				MimeType:          block.MimeType,
				FileURL:           block.FileURL,
				Filename:          block.Filename,
				ToolCallID:        block.ToolCallID,
				ToolName:          block.ToolName,
				ToolCallArguments: block.ToolCallArguments,
			})
		}
		out = append(out, next)
	}
	return out
}

func turnDataFromStreamingState(state *streamingState, uiMessage map[string]any) sdk.TurnData {
	return sdk.BuildTurnDataFromUIMessage(uiMessage, sdk.TurnDataBuildOptions{
		ID:   state.turnID,
		Role: "assistant",
		Metadata: map[string]any{
			"turn_id":             state.turnID,
			"finish_reason":       state.finishReason,
			"prompt_tokens":       state.promptTokens,
			"completion_tokens":   state.completionTokens,
			"reasoning_tokens":    state.reasoningTokens,
			"response_id":         state.responseID,
			"started_at_ms":       state.startedAtMs,
			"completed_at_ms":     state.completedAtMs,
			"first_token_at_ms":   state.firstTokenAtMs,
			"network_message_id":  state.networkMessageID,
			"initial_event_id":    state.initialEventID,
			"source_event_id":     state.sourceEventID,
			"generated_file_refs": agentremote.GeneratedFileRefsFromParts(state.generatedFiles),
		},
		Text:      state.accumulated.String(),
		Reasoning: state.reasoning.String(),
		ToolCalls: state.toolCalls,
	})
}

func buildCanonicalTurnData(
	state *streamingState,
	meta *PortalMetadata,
	linkPreviews []map[string]any,
) sdk.TurnData {
	if state == nil {
		return sdk.TurnData{}
	}
	uiMessage := streamui.SnapshotCanonicalUIMessage(&state.ui)
	td := turnDataFromStreamingState(state, uiMessage)
	artifactParts := buildSourceParts(state.sourceCitations, state.sourceDocuments, nil)
	artifactParts = append(artifactParts, linkPreviews...)
	return sdk.BuildTurnDataFromUIMessage(sdk.UIMessageFromTurnData(td), sdk.TurnDataBuildOptions{
		ID:             td.ID,
		Role:           td.Role,
		Metadata:       buildTurnDataMetadata(state, meta),
		GeneratedFiles: agentremote.GeneratedFileRefsFromParts(state.generatedFiles),
		ArtifactParts:  artifactParts,
	})
}

func buildTurnDataMetadata(state *streamingState, meta *PortalMetadata) map[string]any {
	if state == nil {
		return nil
	}
	modelID := ""
	if meta != nil && meta.ResolvedTarget != nil {
		modelID = strings.TrimSpace(meta.ResolvedTarget.ModelID)
	}
	return map[string]any{
		"turn_id":           state.turnID,
		"agent_id":          state.agentID,
		"model":             modelID,
		"finish_reason":     state.finishReason,
		"prompt_tokens":     state.promptTokens,
		"completion_tokens": state.completionTokens,
		"reasoning_tokens":  state.reasoningTokens,
		"total_tokens":      state.totalTokens,
		"started_at_ms":     state.startedAtMs,
		"first_token_at_ms": state.firstTokenAtMs,
		"completed_at_ms":   state.completedAtMs,
	}
}

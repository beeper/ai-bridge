package ai

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/sdk"
)

func (oc *AIClient) buildStreamingMessageMetadata(state *streamingState, meta *PortalMetadata, uiMessage map[string]any) *MessageMetadata {
	if state == nil {
		return nil
	}
	if len(uiMessage) == 0 {
		uiMessage = oc.buildStreamUIMessage(state, meta, nil)
	}
	turnData := turnDataFromStreamingState(state, uiMessage)
	modelID := oc.effectiveModel(meta)
	return &MessageMetadata{
		BaseMessageMetadata: agentremote.BuildAssistantBaseMetadata(agentremote.AssistantMetadataParams{
			Body:                    state.accumulated.String(),
			FinishReason:            state.finishReason,
			TurnID:                  state.turn.ID(),
			AgentID:                 state.agentID,
			ToolCalls:               state.toolCalls,
			StartedAtMs:             state.startedAtMs,
			CompletedAtMs:           state.completedAtMs,
			CanonicalPromptSchema:   canonicalPromptSchemaV1,
			CanonicalPromptMessages: encodePromptMessages(assistantPromptMessagesFromState(state)),
			GeneratedFiles:          agentremote.GeneratedFileRefsFromParts(state.generatedFiles),
			ThinkingContent:         state.reasoning.String(),
			PromptTokens:            state.promptTokens,
			CompletionTokens:        state.completionTokens,
			ReasoningTokens:         state.reasoningTokens,
			CanonicalTurnSchema:     sdk.CanonicalTurnDataSchemaV1,
			CanonicalTurnData:       turnData.ToMap(),
			CanonicalSchema:         "com.beeper.ai.message",
			CanonicalUIMessage:      uiMessage,
		}),
		AssistantMessageMetadata: agentremote.AssistantMessageMetadata{
			CompletionID:       state.responseID,
			Model:              modelID,
			FirstTokenAtMs:     state.firstTokenAtMs,
			HasToolCalls:       len(state.toolCalls) > 0,
			ThinkingTokenCount: thinkingTokenCount(modelID, state.reasoning.String()),
		},
	}
}

func (oc *AIClient) noteStreamingPersistenceSideEffects(ctx context.Context, portal *bridgev2.Portal, state *streamingState, meta *PortalMetadata) {
	if state == nil {
		return
	}
	if meta != nil && portal != nil && (state.promptTokens > 0 || state.completionTokens > 0) {
		meta.SetModuleMeta("compaction_last_prompt_tokens", state.promptTokens)
		meta.SetModuleMeta("compaction_last_completion_tokens", state.completionTokens)
		meta.SetModuleMeta("compaction_last_usage_at", time.Now().UnixMilli())
		oc.savePortalQuiet(ctx, portal, "compaction usage snapshot")
	}
	oc.notifySessionMutation(ctx, portal, meta, false)
}

// saveAssistantMessage saves the completed assistant message to the database.
// When sendViaPortal was used (state.turn.NetworkMessageID() is set), the DB row already exists
// from SendConvertedMessage — this function updates the metadata with full streaming results.
// Otherwise, it falls back to inserting a new row.
func (oc *AIClient) saveAssistantMessage(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
) {
	uiMessage := oc.buildStreamUIMessage(state, meta, nil)
	fullMeta := oc.buildStreamingMessageMetadata(state, meta, uiMessage)

	agentremote.UpsertAssistantMessage(ctx, agentremote.UpsertAssistantMessageParams{
		Login:            oc.UserLogin,
		Portal:           portal,
		SenderID:         modelUserID(oc.effectiveModel(meta)),
		NetworkMessageID: state.turn.NetworkMessageID(),
		InitialEventID:   state.turn.InitialEventID(),
		Metadata:         fullMeta,
		Logger:           log,
	})
	oc.noteStreamingPersistenceSideEffects(ctx, portal, state, meta)
}

func thinkingTokenCount(model string, content string) int {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0
	}
	tkm, err := getTokenizer(model)
	if err != nil {
		return len(strings.Fields(content))
	}
	return len(tkm.Encode(content, nil, nil))
}

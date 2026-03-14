package ai

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	runtimeparse "github.com/beeper/agentremote/pkg/runtime"

	"github.com/beeper/agentremote/pkg/shared/citations"
)

func (oc *AIClient) ensureInitialStreamMessage(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	isHeartbeat bool,
	initialText string,
	errText string,
	logMessage string,
) error {
	stream := oc.writer(state, portal)
	if !state.firstToken {
		return nil
	}
	state.firstToken = false
	state.firstTokenAtMs = time.Now().UnixMilli()

	if !state.suppressSend && !isHeartbeat {
		oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
		state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state, initialText, state.turnID, state.replyTarget)
		// Some older homeserver/client combinations may accept the send but not
		// return the event ID immediately. In that case, networkMessageID is still
		// sufficient for subsequent debounced/final edits.
		if !state.hasInitialMessageTarget() {
			log.Error().Msg(logMessage)
			state.finishReason = "error"
			stream.Error(ctx, errText)
			oc.emitUIFinish(ctx, portal, state, meta)
			return errors.New(errText)
		}
	}
	return nil
}

func (oc *AIClient) handleResponseOutputTextDelta(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	typingSignals *TypingSignaler,
	isHeartbeat bool,
	delta string,
	errText string,
	logMessage string,
) error {
	_, err := oc.processStreamingTextDelta(ctx, log, portal, state, meta, typingSignals, isHeartbeat, delta, errText, logMessage)
	return err
}

func (oc *AIClient) emitVisibleTextDelta(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	typingSignals *TypingSignaler,
	isHeartbeat bool,
	delta string,
	errText string,
	logMessage string,
) error {
	stream := oc.writer(state, portal)
	if typingSignals != nil {
		typingSignals.SignalTextDelta(delta)
	}
	if delta == "" {
		return nil
	}
	state.visibleAccumulated.WriteString(delta)
	if state.firstToken && state.visibleAccumulated.Len() > 0 {
		if err := oc.ensureInitialStreamMessage(
			ctx,
			log,
			portal,
			state,
			meta,
			isHeartbeat,
			state.visibleAccumulated.String(),
			errText,
			logMessage,
		); err != nil {
			return err
		}
	}
	stream.TextDelta(ctx, delta)
	return nil
}

func (oc *AIClient) processStreamingTextDelta(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	typingSignals *TypingSignaler,
	isHeartbeat bool,
	delta string,
	errText string,
	logMessage string,
) (string, error) {
	delta = maybePrependTextSeparator(state, delta)
	state.accumulated.WriteString(delta)

	roundDelta := delta
	var parsed *runtimeparse.StreamingDirectiveResult
	if state.replyAccumulator != nil {
		parsed = state.replyAccumulator.Consume(delta, false)
	}
	if parsed == nil {
		return roundDelta, nil
	}

	oc.applyStreamingReplyTarget(state, parsed)
	roundDelta = parsed.Text
	if roundDelta == "" {
		return roundDelta, nil
	}

	if err := oc.emitVisibleTextDelta(
		ctx,
		log,
		portal,
		state,
		meta,
		typingSignals,
		isHeartbeat,
		roundDelta,
		errText,
		logMessage,
	); err != nil {
		return "", err
	}
	return roundDelta, nil
}

func (oc *AIClient) handleResponseReasoningTextDelta(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	isHeartbeat bool,
	delta string,
	errText string,
	logMessage string,
) error {
	stream := oc.writer(state, portal)
	state.reasoning.WriteString(delta)
	if state.firstToken && state.reasoning.Len() > 0 {
		if err := oc.ensureInitialStreamMessage(
			ctx,
			log,
			portal,
			state,
			meta,
			isHeartbeat,
			"...",
			errText,
			logMessage,
		); err != nil {
			return err
		}
	}
	stream.ReasoningDelta(ctx, delta)
	return nil
}

// appendReasoningText appends non-empty reasoning/summary text to state and emits a UI delta.
// Used by both reasoning_summary_text.delta and reasoning_text.done / reasoning_summary_text.done.
func (oc *AIClient) appendReasoningText(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	text string,
) {
	stream := oc.writer(state, portal)
	if text == "" {
		return
	}
	state.reasoning.WriteString(text)
	stream.ReasoningDelta(ctx, text)
}

func (oc *AIClient) handleResponseRefusalDelta(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	typingSignals *TypingSignaler,
	delta string,
) {
	stream := oc.writer(state, portal)
	if typingSignals != nil {
		typingSignals.SignalTextDelta(delta)
	}
	stream.TextDelta(ctx, delta)
}

func (oc *AIClient) handleResponseRefusalDone(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	refusal string,
) {
	stream := oc.writer(state, portal)
	if refusal == "" {
		return
	}
	stream.TextDelta(ctx, refusal)
}

func (oc *AIClient) handleResponseOutputAnnotationAdded(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	annotation any,
	annotationIndex any,
) {
	stream := oc.writer(state, portal)
	if citation, ok := extractURLCitation(annotation); ok {
		state.sourceCitations = citations.AppendUniqueCitation(state.sourceCitations, citation)
		stream.SourceURL(ctx, citation)
	}
	if document, ok := extractDocumentCitation(annotation); ok {
		state.sourceDocuments = append(state.sourceDocuments, document)
		stream.SourceDocument(ctx, document)
	}
	stream.Data(ctx, "annotation", map[string]any{"annotation": annotation, "index": annotationIndex}, true)
}

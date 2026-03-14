package sdk

import (
	"context"

	"github.com/beeper/agentremote/pkg/shared/citations"
	"github.com/beeper/agentremote/pkg/shared/streamui"
	"github.com/beeper/agentremote/turns"
	"maunium.net/go/mautrix/bridgev2"
)

// Stream is a conversation-level facade backed by a Turn.
type Stream struct {
	turn *Turn
}

func newStream(ctx context.Context, conv *Conversation, agent *Agent, source *SourceRef) *Stream {
	return &Stream{turn: newTurn(ctx, conv, agent, source)}
}

// Turn exposes the underlying turn for advanced use cases.
func (s *Stream) Turn() *Turn {
	if s == nil {
		return nil
	}
	return s.turn
}

// ID returns the underlying turn ID.
func (s *Stream) ID() string {
	if s == nil || s.turn == nil {
		return ""
	}
	return s.turn.ID()
}

// Context returns the underlying turn context.
func (s *Stream) Context() context.Context {
	if s == nil || s.turn == nil {
		return nil
	}
	return s.turn.Context()
}

// SetAgent configures the stream's agent before output starts.
func (s *Stream) SetAgent(agent *Agent) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.SetAgent(agent)
}

// SetSource configures the stream's source before output starts.
func (s *Stream) SetSource(source *SourceRef) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.SetSource(source)
}

// SetID overrides the stream's turn ID before output starts.
func (s *Stream) SetID(turnID string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.SetID(turnID)
}

// SetSender overrides the stream sender before output starts.
func (s *Stream) SetSender(sender bridgev2.EventSender) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.SetSender(sender)
}

// SetFinalMetadataProvider overrides persisted final metadata.
func (s *Stream) SetFinalMetadataProvider(provider FinalMetadataProvider) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.SetFinalMetadataProvider(provider)
}

// SetTransport configures a custom transport for streamed events.
func (s *Stream) SetTransport(hook func(turnID string, seq int, content map[string]any, txnID string) bool) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.SetStreamHook(hook)
}

// WriteText sends a text chunk.
func (s *Stream) WriteText(text string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.WriteText(text)
}

// TextDelta is an alias for WriteText.
func (s *Stream) TextDelta(text string) {
	s.WriteText(text)
}

// WriteReasoning sends a reasoning chunk.
func (s *Stream) WriteReasoning(text string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.WriteReasoning(text)
}

// ReasoningDelta is an alias for WriteReasoning.
func (s *Stream) ReasoningDelta(text string) {
	s.WriteReasoning(text)
}

// ToolStart begins a tool call.
func (s *Stream) ToolStart(toolName, toolCallID string, providerExecuted bool) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.ToolStart(toolName, toolCallID, providerExecuted)
}

// ToolInputDelta emits a tool input delta.
func (s *Stream) ToolInputDelta(toolCallID, delta string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.ToolInputDelta(toolCallID, delta)
}

// ToolInput emits a tool input payload.
func (s *Stream) ToolInput(toolCallID string, input any) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.ToolInput(toolCallID, input)
}

// ToolOutput emits a tool output payload.
func (s *Stream) ToolOutput(toolCallID string, output any) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.ToolOutput(toolCallID, output)
}

// ToolOutputError emits a tool error payload.
func (s *Stream) ToolOutputError(toolCallID, errorText string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.ToolOutputError(toolCallID, errorText)
}

// ToolDenied emits a denied tool result.
func (s *Stream) ToolDenied(toolCallID string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.ToolDenied(toolCallID)
}

// AddSourceURL emits a source URL citation.
func (s *Stream) AddSourceURL(url, title string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.AddSourceURL(url, title)
}

// SourceURL is an alias for AddSourceURL.
func (s *Stream) SourceURL(url, title string) {
	s.AddSourceURL(url, title)
}

// AddSourceDocument emits a source document citation.
func (s *Stream) AddSourceDocument(docID, title, mediaType, filename string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.AddSourceDocument(docID, title, mediaType, filename)
}

// SourceDocument emits a structured source document citation.
func (s *Stream) SourceDocument(document citations.SourceDocument) {
	s.AddSourceDocument(document.ID, document.Title, document.MediaType, document.Filename)
}

// SourceCitation emits a structured source URL citation.
func (s *Stream) SourceCitation(citation citations.SourceCitation) {
	s.AddSourceURL(citation.URL, citation.Title)
}

// AddFile emits a generated file part.
func (s *Stream) AddFile(url, mediaType string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.AddFile(url, mediaType)
}

// File is an alias for AddFile.
func (s *Stream) File(url, mediaType string) {
	s.AddFile(url, mediaType)
}

// GeneratedFile emits a structured generated file part.
func (s *Stream) GeneratedFile(file citations.GeneratedFilePart) {
	s.AddFile(file.URL, file.MediaType)
}

// StepStart begins a visual step group.
func (s *Stream) StepStart() {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.StepStart()
}

// StepFinish ends a visual step group.
func (s *Stream) StepFinish() {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.StepFinish()
}

// SetMetadata merges metadata into the final message and emits it to the UI.
func (s *Stream) SetMetadata(metadata map[string]any) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.SetMetadata(metadata)
}

// Metadata is an alias for SetMetadata.
func (s *Stream) Metadata(metadata map[string]any) {
	s.SetMetadata(metadata)
}

// Error emits a UI error event.
func (s *Stream) Error(text string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.Stream().Error(text)
}

// End finishes the stream.
func (s *Stream) End(finishReason string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.End(finishReason)
}

// EndWithError finishes the stream with an error.
func (s *Stream) EndWithError(errText string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.EndWithError(errText)
}

// Abort aborts the stream.
func (s *Stream) Abort(reason string) {
	if s == nil || s.turn == nil {
		return
	}
	s.turn.Abort(reason)
}

// Emitter returns the underlying stream emitter.
func (s *Stream) Emitter() *streamui.Emitter {
	if s == nil || s.turn == nil {
		return nil
	}
	return s.turn.Emitter()
}

// UIState returns the underlying UI state.
func (s *Stream) UIState() *streamui.UIState {
	if s == nil || s.turn == nil {
		return nil
	}
	return s.turn.UIState()
}

// Session returns the underlying stream session.
func (s *Stream) Session() *turns.StreamSession {
	if s == nil || s.turn == nil {
		return nil
	}
	return s.turn.Session()
}

// TurnStream returns the provider-facing turn stream facade.
func (s *Stream) TurnStream() *TurnStream {
	if s == nil || s.turn == nil {
		return nil
	}
	return s.turn.Stream()
}

// Approvals returns the approval controller for this stream.
func (s *Stream) Approvals() *ApprovalController {
	if s == nil || s.turn == nil {
		return nil
	}
	return s.turn.Approvals()
}

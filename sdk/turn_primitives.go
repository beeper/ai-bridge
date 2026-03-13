package sdk

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/agentremote/pkg/shared/citations"
	"github.com/beeper/agentremote/pkg/shared/streamui"
)

// StreamTransport handles SDK turn stream events for custom transports or tests.
type StreamTransport interface {
	HandleTurnEvent(turnID string, seq int, content map[string]any, txnID string) bool
}

// StreamTransportFunc adapts a function to StreamTransport.
type StreamTransportFunc func(turnID string, seq int, content map[string]any, txnID string) bool

func (f StreamTransportFunc) HandleTurnEvent(turnID string, seq int, content map[string]any, txnID string) bool {
	if f == nil {
		return false
	}
	return f(turnID, seq, content, txnID)
}

// ApprovalHandler handles turn approval requests for provider-driven bridges.
type ApprovalHandler interface {
	Request(ctx context.Context, turn *Turn, req ApprovalRequest) ApprovalHandle
}

// ApprovalHandlerFunc adapts a function to ApprovalHandler.
type ApprovalHandlerFunc func(ctx context.Context, turn *Turn, req ApprovalRequest) ApprovalHandle

func (f ApprovalHandlerFunc) Request(ctx context.Context, turn *Turn, req ApprovalRequest) ApprovalHandle {
	if f == nil {
		return nil
	}
	return f(ctx, turn, req)
}

// FinalMetadataProvider builds the final DB metadata object for a completed turn.
type FinalMetadataProvider interface {
	BuildFinalMetadata(turn *Turn, finishReason string) any
}

// FinalMetadataProviderFunc adapts a function to FinalMetadataProvider.
type FinalMetadataProviderFunc func(turn *Turn, finishReason string) any

func (f FinalMetadataProviderFunc) BuildFinalMetadata(turn *Turn, finishReason string) any {
	if f == nil {
		return nil
	}
	return f(turn, finishReason)
}

// ToolInputOptions controls how a tool input start is represented in the SDK UI stream.
type ToolInputOptions struct {
	ToolName         string
	ProviderExecuted bool
	DisplayTitle     string
}

// ToolOutputOptions controls how a tool output is represented in the SDK UI stream.
type ToolOutputOptions struct {
	ProviderExecuted bool
	Streaming        bool
}

// turnAccessor provides shared valid/portal checks for turn-scoped controllers.
type turnAccessor struct {
	turn *Turn
}

func (a *turnAccessor) valid() bool { return a != nil && a.turn != nil }

func (a *turnAccessor) portal() *bridgev2.Portal {
	if !a.valid() || a.turn.conv == nil {
		return nil
	}
	return a.turn.conv.portal
}

// TurnStream is the provider-facing streaming surface for a turn.
type TurnStream struct {
	turnAccessor
}

// Stream returns the turn's provider-facing streaming surface.
func (t *Turn) Stream() *TurnStream {
	if t == nil {
		return nil
	}
	return &TurnStream{turnAccessor{turn: t}}
}

// Emitter returns the underlying stream emitter as an escape hatch.
func (s *TurnStream) Emitter() *streamui.Emitter {
	if !s.valid() {
		return nil
	}
	return s.turn.emitter
}

// SetTransport configures a custom transport for streamed turn events.
func (s *TurnStream) SetTransport(transport StreamTransport) {
	if !s.valid() {
		return
	}
	if transport == nil {
		s.turn.streamHook = nil
		return
	}
	s.turn.streamHook = transport.HandleTurnEvent
}

// TextDelta emits a text delta.
func (s *TurnStream) TextDelta(text string) {
	if !s.valid() {
		return
	}
	s.turn.WriteText(text)
}

// ReasoningDelta emits a reasoning delta.
func (s *TurnStream) ReasoningDelta(text string) {
	if !s.valid() {
		return
	}
	s.turn.WriteReasoning(text)
}

// Error emits a UI error event for the turn.
func (s *TurnStream) Error(text string) {
	if !s.valid() {
		return
	}
	s.turn.ensureStarted()
	s.turn.emitter.EmitUIError(s.turn.turnCtx, s.portal(), text)
}

// TextEnd closes the current text stream part.
func (s *TurnStream) TextEnd() {
	if !s.valid() {
		return
	}
	s.turn.FinishText()
}

// ReasoningEnd closes the current reasoning stream part.
func (s *TurnStream) ReasoningEnd() {
	if !s.valid() {
		return
	}
	s.turn.FinishReasoning()
}

// EnsureToolInputStart ensures the tool input UI exists and optionally publishes input.
func (s *TurnStream) EnsureToolInputStart(toolCallID string, input any, opts ToolInputOptions) {
	if !s.valid() || strings.TrimSpace(toolCallID) == "" {
		return
	}
	s.turn.ensureStarted()
	toolName := strings.TrimSpace(opts.ToolName)
	displayTitle := strings.TrimSpace(opts.DisplayTitle)
	if displayTitle == "" {
		displayTitle = streamui.ToolDisplayTitle(toolName)
	}
	s.turn.emitter.EnsureUIToolInputStart(s.turn.turnCtx, s.portal(), toolCallID, toolName, opts.ProviderExecuted, displayTitle, nil)
	if input != nil {
		s.turn.emitter.EmitUIToolInputAvailable(s.turn.turnCtx, s.portal(), toolCallID, toolName, input, opts.ProviderExecuted)
	}
}

// ToolInputDelta emits a tool input delta.
func (s *TurnStream) ToolInputDelta(toolCallID, delta string, providerExecuted bool) {
	if !s.valid() {
		return
	}
	s.turn.ensureStarted()
	s.turn.emitter.EmitUIToolInputDelta(s.turn.turnCtx, s.portal(), toolCallID, "", delta, providerExecuted)
}

// ToolInput emits a complete tool input payload.
func (s *TurnStream) ToolInput(toolCallID, toolName string, input any, providerExecuted bool) {
	if !s.valid() {
		return
	}
	s.turn.ensureStarted()
	s.turn.emitter.EmitUIToolInputAvailable(s.turn.turnCtx, s.portal(), toolCallID, toolName, input, providerExecuted)
}

// ToolOutput emits a tool output payload.
func (s *TurnStream) ToolOutput(toolCallID string, output any, opts ToolOutputOptions) {
	if !s.valid() {
		return
	}
	s.turn.ensureStarted()
	s.turn.emitter.EmitUIToolOutputAvailable(s.turn.turnCtx, s.portal(), toolCallID, output, opts.ProviderExecuted, opts.Streaming)
}

// ToolOutputError emits a tool error payload.
func (s *TurnStream) ToolOutputError(toolCallID, errText string, providerExecuted bool) {
	if !s.valid() {
		return
	}
	s.turn.ensureStarted()
	s.turn.emitter.EmitUIToolOutputError(s.turn.turnCtx, s.portal(), toolCallID, errText, providerExecuted)
}

// ToolDenied emits a denied tool result.
func (s *TurnStream) ToolDenied(toolCallID string) {
	if !s.valid() {
		return
	}
	s.turn.ToolDenied(toolCallID)
}

// SourceURL emits a source URL citation.
func (s *TurnStream) SourceURL(url, title string) {
	if !s.valid() {
		return
	}
	s.turn.AddSourceURL(url, title)
}

// SourceCitation emits a source URL citation from a structured citation object.
func (s *TurnStream) SourceCitation(citation citations.SourceCitation) {
	if !s.valid() {
		return
	}
	s.turn.AddSourceURL(citation.URL, citation.Title)
}

// SourceDocument emits a source document citation.
func (s *TurnStream) SourceDocument(document citations.SourceDocument) {
	if !s.valid() {
		return
	}
	s.turn.AddSourceDocument(document.ID, document.Title, document.MediaType, document.Filename)
}

// File emits a generated file part.
func (s *TurnStream) File(url, mediaType string) {
	if !s.valid() {
		return
	}
	s.turn.AddFile(url, mediaType)
}

// GeneratedFile emits a generated file part from a structured file object.
func (s *TurnStream) GeneratedFile(file citations.GeneratedFilePart) {
	if !s.valid() {
		return
	}
	s.turn.AddFile(file.URL, file.MediaType)
}

// StepStart begins a visual step group.
func (s *TurnStream) StepStart() {
	if !s.valid() {
		return
	}
	s.turn.StepStart()
}

// StepFinish ends a visual step group.
func (s *TurnStream) StepFinish() {
	if !s.valid() {
		return
	}
	s.turn.StepFinish()
}

// Metadata merges message metadata for the turn.
func (s *TurnStream) Metadata(metadata map[string]any) {
	if !s.valid() {
		return
	}
	s.turn.SetMetadata(metadata)
}

// ApprovalController is the turn-owned approval surface.
type ApprovalController struct {
	turnAccessor
}

// Approvals returns the turn's approval controller.
func (t *Turn) Approvals() *ApprovalController {
	if t == nil {
		return nil
	}
	return &ApprovalController{turnAccessor{turn: t}}
}

// SetHandler configures a provider-specific approval handler for this turn.
func (a *ApprovalController) SetHandler(handler ApprovalHandler) {
	if !a.valid() {
		return
	}
	if handler == nil {
		a.turn.approvalRequester = nil
		return
	}
	a.turn.approvalRequester = handler.Request
}

// Request creates a new approval request.
func (a *ApprovalController) Request(req ApprovalRequest) ApprovalHandle {
	if !a.valid() {
		return nil
	}
	return a.turn.RequestApproval(req)
}

// EmitRequest emits the approval-request UI state for a provider-managed approval.
func (a *ApprovalController) EmitRequest(approvalID, toolCallID string) {
	if !a.valid() {
		return
	}
	a.turn.ensureStarted()
	a.turn.emitter.EmitUIToolApprovalRequest(a.turn.turnCtx, a.portal(), approvalID, toolCallID)
}

// Respond emits the approval-response UI state for a provider-managed approval.
func (a *ApprovalController) Respond(approvalID, toolCallID string, approved bool, reason string) {
	if !a.valid() {
		return
	}
	a.turn.ensureStarted()
	a.turn.emitter.EmitUIToolApprovalResponse(a.turn.turnCtx, a.portal(), approvalID, toolCallID, approved, reason)
}

// SetStreamTransport configures a custom turn stream transport.
func (t *Turn) SetStreamTransport(transport StreamTransport) {
	t.Stream().SetTransport(transport)
}

// SetApprovalHandler configures a provider-specific approval handler for this turn.
func (t *Turn) SetApprovalHandler(handler ApprovalHandler) {
	t.Approvals().SetHandler(handler)
}

// SetFinalMetadataProvider overrides the final DB metadata object persisted for the assistant message.
func (t *Turn) SetFinalMetadataProvider(provider FinalMetadataProvider) {
	if t == nil {
		return
	}
	if provider == nil {
		t.finalMetadataBuilder = nil
		return
	}
	t.finalMetadataBuilder = provider.BuildFinalMetadata
}

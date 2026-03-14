package streamui

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
)

// ReplayBuilder applies canonical UI parts onto a UIState without a live portal.
// It is intended for backfill and history reconstruction paths.
type ReplayBuilder struct {
	State   *UIState
	emitter *Emitter
	visible strings.Builder
}

// NewReplayBuilder creates a replay helper for an existing UI state.
func NewReplayBuilder(state *UIState) *ReplayBuilder {
	if state == nil {
		return nil
	}
	state.InitMaps()
	builder := &ReplayBuilder{State: state}
	builder.emitter = &Emitter{
		State: state,
		Emit: func(_ context.Context, _ *bridgev2.Portal, part map[string]any) {
			ApplyChunk(state, part)
		},
	}
	return builder
}

func (b *ReplayBuilder) emit(part map[string]any) {
	if b == nil || b.State == nil || len(part) == 0 {
		return
	}
	ApplyChunk(b.State, part)
}

// VisibleText returns the accumulated visible assistant text written via Text().
func (b *ReplayBuilder) VisibleText() string {
	if b == nil {
		return ""
	}
	return b.visible.String()
}

// Start emits the canonical turn start.
func (b *ReplayBuilder) Start(metadata map[string]any) {
	if b == nil || b.State == nil {
		return
	}
	part := map[string]any{
		"type":      "start",
		"messageId": b.State.TurnID,
	}
	if len(metadata) > 0 {
		part["messageMetadata"] = metadata
	}
	b.emit(part)
}

// MessageMetadata emits a metadata-only update.
func (b *ReplayBuilder) MessageMetadata(metadata map[string]any) {
	if len(metadata) == 0 {
		return
	}
	b.emit(map[string]any{
		"type":            "message-metadata",
		"messageMetadata": metadata,
	})
}

// Finish emits the canonical turn finish.
func (b *ReplayBuilder) Finish(finishReason string, metadata map[string]any) {
	if b == nil {
		return
	}
	finishReason = strings.TrimSpace(finishReason)
	if finishReason == "" {
		finishReason = "stop"
	}
	part := map[string]any{
		"type":         "finish",
		"finishReason": finishReason,
	}
	if len(metadata) > 0 {
		part["messageMetadata"] = metadata
	}
	b.emit(part)
}

// Text emits a completed visible text part.
func (b *ReplayBuilder) Text(partID, text string) {
	if b == nil {
		return
	}
	partID = strings.TrimSpace(partID)
	text = strings.TrimSpace(text)
	if partID == "" || text == "" {
		return
	}
	b.emit(map[string]any{"type": "text-start", "id": partID})
	b.emit(map[string]any{"type": "text-delta", "id": partID, "delta": text})
	b.emit(map[string]any{"type": "text-end", "id": partID})
	b.visible.WriteString(text)
}

// Reasoning emits a completed reasoning part.
func (b *ReplayBuilder) Reasoning(partID, text string) {
	if b == nil {
		return
	}
	partID = strings.TrimSpace(partID)
	text = strings.TrimSpace(text)
	if partID == "" || text == "" {
		return
	}
	b.emit(map[string]any{"type": "reasoning-start", "id": partID})
	b.emit(map[string]any{"type": "reasoning-delta", "id": partID, "delta": text})
	b.emit(map[string]any{"type": "reasoning-end", "id": partID})
}

// StepStart emits a step start marker.
func (b *ReplayBuilder) StepStart() {
	b.emit(map[string]any{"type": "start-step"})
}

// StepFinish emits a step finish marker.
func (b *ReplayBuilder) StepFinish() {
	b.emit(map[string]any{"type": "finish-step"})
}

// Data emits a persisted data-* part.
func (b *ReplayBuilder) Data(part map[string]any) {
	b.emit(part)
}

// ToolInput emits a full tool input payload.
func (b *ReplayBuilder) ToolInput(toolCallID, toolName string, input any, providerExecuted bool) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.EmitUIToolInputAvailable(context.Background(), nil, toolCallID, toolName, input, providerExecuted)
}

// ToolInputText emits streamed tool input reconstructed from raw text.
func (b *ReplayBuilder) ToolInputText(toolCallID, toolName, inputText string, providerExecuted bool) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.EmitUIToolInputDelta(context.Background(), nil, toolCallID, toolName, inputText, providerExecuted)
}

// ToolOutput emits a final tool output payload.
func (b *ReplayBuilder) ToolOutput(toolCallID string, output any, providerExecuted bool) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.EmitUIToolOutputAvailable(context.Background(), nil, toolCallID, output, providerExecuted, false)
}

// ToolOutputError emits a final tool error payload.
func (b *ReplayBuilder) ToolOutputError(toolCallID, errorText string, providerExecuted bool) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.EmitUIToolOutputError(context.Background(), nil, toolCallID, errorText, providerExecuted)
}

// ToolDenied emits a denied tool result.
func (b *ReplayBuilder) ToolDenied(toolCallID string) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.EmitUIToolOutputDenied(context.Background(), nil, toolCallID)
}

// ApprovalRequest emits a tool approval request.
func (b *ReplayBuilder) ApprovalRequest(approvalID, toolCallID string) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.EmitUIToolApprovalRequest(context.Background(), nil, approvalID, toolCallID)
}

// File emits a generated file part.
func (b *ReplayBuilder) File(url, mediaType, filename string) {
	if b == nil {
		return
	}
	part := map[string]any{
		"type":      "file",
		"url":       strings.TrimSpace(url),
		"mediaType": strings.TrimSpace(mediaType),
	}
	if part["url"] == "" {
		return
	}
	if part["mediaType"] == "" {
		part["mediaType"] = "application/octet-stream"
	}
	if trimmedFilename := strings.TrimSpace(filename); trimmedFilename != "" {
		part["filename"] = trimmedFilename
	}
	b.emit(part)
}

// SourceURL emits a source-url part.
func (b *ReplayBuilder) SourceURL(sourceID, url, title string) {
	if b == nil {
		return
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}
	part := map[string]any{
		"type": "source-url",
		"url":  url,
	}
	if trimmedID := strings.TrimSpace(sourceID); trimmedID != "" {
		part["sourceId"] = trimmedID
	}
	if trimmedTitle := strings.TrimSpace(title); trimmedTitle != "" {
		part["title"] = trimmedTitle
	}
	b.emit(part)
}

// SourceDocument emits a source-document part.
func (b *ReplayBuilder) SourceDocument(sourceID, title, filename, mediaType string) {
	if b == nil {
		return
	}
	title = strings.TrimSpace(title)
	filename = strings.TrimSpace(filename)
	mediaType = strings.TrimSpace(mediaType)
	if title == "" && filename == "" {
		return
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	part := map[string]any{
		"type":      "source-document",
		"mediaType": mediaType,
	}
	if trimmedID := strings.TrimSpace(sourceID); trimmedID != "" {
		part["sourceId"] = trimmedID
	}
	if title != "" {
		part["title"] = title
	}
	if filename != "" {
		part["filename"] = filename
	}
	b.emit(part)
}

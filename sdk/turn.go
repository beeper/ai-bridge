package sdk

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/pkg/shared/citations"
	"github.com/beeper/agentremote/pkg/shared/streamui"
	"github.com/beeper/agentremote/turns"
)

// Stream is a type alias for Turn, preserved for backward compatibility.
type Stream = Turn

// Turn is the central abstraction for an AI response turn. It wraps
// streamui.Emitter + turns.StreamSession + streamui.UIState and provides
// lazy initialization: no Matrix message is created until first content.
type Turn struct {
	ctx     context.Context
	conv    *Conversation
	emitter *streamui.Emitter
	state   *streamui.UIState
	session *turns.StreamSession
	turnID  string
	started bool
	ended   bool

	agent         *AgentMember
	sourceEventID id.EventID
	replyTo       id.EventID
	threadRoot    id.EventID
	startedAtMs   int64
}

func newTurn(ctx context.Context, conv *Conversation, agent *AgentMember) *Turn {
	turnID := uuid.NewString()
	state := &streamui.UIState{TurnID: turnID}
	state.InitMaps()

	t := &Turn{
		ctx:         ctx,
		conv:        conv,
		state:       state,
		turnID:      turnID,
		agent:       agent,
		startedAtMs: time.Now().UnixMilli(),
	}

	t.emitter = &streamui.Emitter{
		State: state,
		Emit: func(ctx context.Context, portal *bridgev2.Portal, part map[string]any) {
			if t.session != nil {
				t.session.EmitPart(ctx, part)
			}
		},
	}

	// Create stream session with minimal params.
	if conv.portal != nil {
		var seq int
		logger := zerolog.Nop()
		t.session = turns.NewStreamSession(turns.StreamSessionParams{
			TurnID:  turnID,
			NextSeq: func() int { seq++; return seq },
			GetRoomID: func() id.RoomID {
				return conv.portal.MXID
			},
			GetStreamTarget: func() turns.StreamTarget {
				return turns.StreamTarget{}
			},
			GetSuppressSend: func() bool { return false },
			RuntimeFallbackFlag: &atomic.Bool{},
			GetEphemeralSender: func(ctx context.Context) (bridgev2.EphemeralSendingMatrixAPI, bool) {
				return nil, false
			},
			SendDebouncedEdit: func(ctx context.Context, force bool) error { return nil },
			Logger:            &logger,
		})
	}

	return t
}

// newStream creates a Turn (backward-compatible name).
func newStream(ctx context.Context, conv *Conversation) *Turn {
	return newTurn(ctx, conv, nil)
}

func (t *Turn) ensureStarted() {
	if t.started || t.ended {
		return
	}
	t.started = true
	t.emitter.EmitUIStart(t.ctx, t.conv.portal, nil)
}

// WriteText sends a text chunk.
func (t *Turn) WriteText(text string) {
	t.ensureStarted()
	t.emitter.EmitUITextDelta(t.ctx, t.conv.portal, text)
}

// WriteReasoning sends a reasoning/thinking chunk.
func (t *Turn) WriteReasoning(text string) {
	t.ensureStarted()
	t.emitter.EmitUIReasoningDelta(t.ctx, t.conv.portal, text)
}

// ToolStart begins a tool call.
func (t *Turn) ToolStart(toolName, toolCallID string, providerExecuted bool) {
	t.ensureStarted()
	t.emitter.EnsureUIToolInputStart(t.ctx, t.conv.portal, toolCallID, toolName, providerExecuted, toolName, nil)
}

// ToolInputDelta sends a streaming tool input argument chunk.
func (t *Turn) ToolInputDelta(toolCallID, delta string) {
	t.ensureStarted()
	t.emitter.EmitUIToolInputDelta(t.ctx, t.conv.portal, toolCallID, "", delta, false)
}

// ToolInput sends the complete tool input.
func (t *Turn) ToolInput(toolCallID string, input any) {
	t.ensureStarted()
	t.emitter.EmitUIToolInputAvailable(t.ctx, t.conv.portal, toolCallID, "", input, false)
}

// ToolOutput sends the tool execution result.
func (t *Turn) ToolOutput(toolCallID string, output any) {
	t.ensureStarted()
	t.emitter.EmitUIToolOutputAvailable(t.ctx, t.conv.portal, toolCallID, output, false, false)
}

// ToolOutputError reports a tool execution error.
func (t *Turn) ToolOutputError(toolCallID, errorText string) {
	t.ensureStarted()
	t.emitter.EmitUIToolOutputError(t.ctx, t.conv.portal, toolCallID, errorText, false)
}

// ToolDenied reports that the tool execution was denied by the user.
func (t *Turn) ToolDenied(toolCallID string) {
	t.ensureStarted()
	t.emitter.EmitUIToolOutputDenied(t.ctx, t.conv.portal, toolCallID)
}

// RequestApproval sends a tool approval prompt and blocks until the user responds.
func (t *Turn) RequestApproval(toolCallID, toolName string) (ToolApprovalResponse, error) {
	t.ensureStarted()
	client := t.conv.client
	if client == nil || client.approvalFlow == nil || t.conv.portal == nil {
		return ToolApprovalResponse{}, nil
	}

	approvalID := "sdk-" + uuid.NewString()
	ttl := 10 * time.Minute

	_, created := client.approvalFlow.Register(approvalID, ttl, &pendingSDKApprovalData{
		RoomID:     t.conv.portal.MXID,
		TurnID:     t.turnID,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	})
	if !created {
		return ToolApprovalResponse{}, nil
	}

	// Emit UI events for the approval request.
	t.emitter.EmitUIToolApprovalRequest(t.ctx, t.conv.portal, approvalID, toolCallID)

	// Send the approval prompt message.
	presentation := agentremote.ApprovalPromptPresentation{
		Title:       toolName,
		AllowAlways: true,
	}
	client.approvalFlow.SendPrompt(t.ctx, t.conv.portal, agentremote.SendPromptParams{
		ApprovalPromptMessageParams: agentremote.ApprovalPromptMessageParams{
			ApprovalID:   approvalID,
			ToolCallID:   toolCallID,
			ToolName:     toolName,
			TurnID:       t.turnID,
			Presentation: presentation,
			ExpiresAt:    time.Now().Add(ttl),
		},
		RoomID:    t.conv.portal.MXID,
		OwnerMXID: client.userLogin.UserMXID,
	})

	// Block until user decision.
	decision, ok := client.approvalFlow.Wait(t.ctx, approvalID)
	if !ok {
		reason := agentremote.ApprovalReasonTimeout
		if t.ctx.Err() != nil {
			reason = agentremote.ApprovalReasonCancelled
		}
		client.approvalFlow.FinishResolved(approvalID, agentremote.ApprovalDecisionPayload{
			ApprovalID: approvalID,
			Reason:     reason,
		})
		t.emitter.EmitUIToolApprovalResponse(t.ctx, t.conv.portal, approvalID, toolCallID, false, reason)
		return ToolApprovalResponse{Reason: reason}, nil
	}

	t.emitter.EmitUIToolApprovalResponse(t.ctx, t.conv.portal, approvalID, toolCallID, decision.Approved, decision.Reason)
	client.approvalFlow.FinishResolved(approvalID, decision)
	return ToolApprovalResponse{
		Approved: decision.Approved,
		Always:   decision.Always,
		Reason:   decision.Reason,
	}, nil
}

// AddSourceURL adds a source citation URL.
func (t *Turn) AddSourceURL(url, title string) {
	t.ensureStarted()
	t.emitter.EmitUISourceURL(t.ctx, t.conv.portal, citations.SourceCitation{
		URL:   url,
		Title: title,
	})
}

// AddSourceDocument adds a source document citation.
func (t *Turn) AddSourceDocument(docID, title, mediaType, filename string) {
	t.ensureStarted()
	t.emitter.EmitUISourceDocument(t.ctx, t.conv.portal, citations.SourceDocument{
		ID:        docID,
		Title:     title,
		MediaType: mediaType,
		Filename:  filename,
	})
}

// AddFile adds a generated file reference.
func (t *Turn) AddFile(url, mediaType string) {
	t.ensureStarted()
	t.emitter.EmitUIFile(t.ctx, t.conv.portal, url, mediaType)
}

// StepStart begins a visual step grouping.
func (t *Turn) StepStart() {
	t.ensureStarted()
	t.emitter.EmitUIStepStart(t.ctx, t.conv.portal)
}

// StepFinish ends a visual step grouping.
func (t *Turn) StepFinish() {
	t.ensureStarted()
	t.emitter.EmitUIStepFinish(t.ctx, t.conv.portal)
}

// SetMetadata sets message metadata (model, timing, usage).
func (t *Turn) SetMetadata(metadata map[string]any) {
	t.ensureStarted()
	t.emitter.EmitUIMessageMetadata(t.ctx, t.conv.portal, metadata)
}

// SetReplyTo sets the m.in_reply_to relation for this turn's message.
func (t *Turn) SetReplyTo(eventID id.EventID) {
	t.replyTo = eventID
}

// SetThread sets the m.thread relation for this turn's message.
func (t *Turn) SetThread(rootEventID id.EventID) {
	t.threadRoot = rootEventID
}

// SendStatus sends a message status event.
func (t *Turn) SendStatus(status event.MessageStatus, message string) {
	// Status sending is a no-op in the SDK layer; the bridge framework handles this.
	_ = status
	_ = message
}

// End finishes the turn with a reason.
func (t *Turn) End(finishReason string) {
	if t.ended {
		return
	}
	if !t.started {
		// Empty turn: no content was emitted, just mark ended.
		t.ended = true
		return
	}
	t.ended = true
	t.emitter.EmitUIFinish(t.ctx, t.conv.portal, finishReason, nil)
	if t.session != nil {
		t.session.End(t.ctx, turns.EndReasonFinish)
	}
}

// EndWithError finishes the turn with an error.
func (t *Turn) EndWithError(errText string) {
	if t.ended {
		return
	}
	t.ensureStarted()
	t.ended = true
	t.emitter.EmitUIError(t.ctx, t.conv.portal, errText)
	t.emitter.EmitUIFinish(t.ctx, t.conv.portal, "error", nil)
	if t.session != nil {
		t.session.End(t.ctx, turns.EndReasonError)
	}
}

// Abort aborts the turn.
func (t *Turn) Abort(reason string) {
	if t.ended {
		return
	}
	t.ensureStarted()
	t.ended = true
	t.emitter.EmitUIAbort(t.ctx, t.conv.portal, reason)
	if t.session != nil {
		t.session.End(t.ctx, turns.EndReasonDisconnect)
	}
}

// ID returns the turn's unique identifier.
func (t *Turn) ID() string { return t.turnID }

// Emitter returns the underlying streamui.Emitter for escape hatch access.
func (t *Turn) Emitter() *streamui.Emitter { return t.emitter }

// UIState returns the underlying streamui.UIState.
func (t *Turn) UIState() *streamui.UIState { return t.state }

// Session returns the underlying turns.StreamSession.
func (t *Turn) Session() *turns.StreamSession { return t.session }

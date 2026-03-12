package opencode

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
)

func (m *OpenCodeManager) ensureTurnStarted(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, sessionID, messageID string, metadata map[string]any) {
	if m == nil || m.bridge == nil || inst == nil || portal == nil {
		return
	}
	if sessionID == "" || messageID == "" {
		return
	}
	state := inst.ensureTurnState(sessionID, messageID)
	if state == nil {
		return
	}
	turnID := opencodeMessageStreamTurnID(sessionID, messageID)
	if turnID == "" {
		return
	}
	agentID := m.bridge.portalAgentID(portal)
	if state.started {
		if len(metadata) > 0 {
			m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
				"type":            "message-metadata",
				"messageMetadata": metadata,
			})
		}
		return
	}
	part := map[string]any{"type": "start", "messageId": turnID}
	if len(metadata) > 0 {
		part["messageMetadata"] = metadata
	}
	m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, part)
	state.started = true
}

func (m *OpenCodeManager) ensureStepStarted(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, sessionID, messageID string) {
	if m == nil || m.bridge == nil || inst == nil || portal == nil {
		return
	}
	if sessionID == "" || messageID == "" {
		return
	}
	m.ensureTurnStarted(ctx, inst, portal, sessionID, messageID, nil)
	state := inst.turnStateFor(sessionID, messageID)
	if state == nil || state.stepOpen {
		return
	}
	turnID := opencodeMessageStreamTurnID(sessionID, messageID)
	if turnID == "" {
		return
	}
	agentID := m.bridge.portalAgentID(portal)
	m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
		"type": "start-step",
	})
	state.stepOpen = true
}

func (m *OpenCodeManager) closeStepIfOpen(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, sessionID, messageID string) {
	if m == nil || m.bridge == nil || inst == nil || portal == nil {
		return
	}
	if sessionID == "" || messageID == "" {
		return
	}
	state := inst.turnStateFor(sessionID, messageID)
	if state == nil || !state.stepOpen {
		return
	}
	turnID := opencodeMessageStreamTurnID(sessionID, messageID)
	if turnID == "" {
		return
	}
	agentID := m.bridge.portalAgentID(portal)
	m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
		"type": "finish-step",
	})
	state.stepOpen = false
}

func (m *OpenCodeManager) emitTurnFinish(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, sessionID, messageID, finishReason string, metadata map[string]any) {
	if m == nil || m.bridge == nil || inst == nil || portal == nil {
		return
	}
	if sessionID == "" || messageID == "" {
		return
	}
	state := inst.turnStateFor(sessionID, messageID)
	if state == nil || !state.started || state.finished {
		return
	}
	m.closeStepIfOpen(ctx, inst, portal, sessionID, messageID)
	turnID := opencodeMessageStreamTurnID(sessionID, messageID)
	if turnID == "" {
		return
	}
	if finishReason == "" {
		finishReason = "stop"
	}
	agentID := m.bridge.portalAgentID(portal)
	part := map[string]any{
		"type":         "finish",
		"finishReason": finishReason,
	}
	if len(metadata) > 0 {
		part["messageMetadata"] = metadata
	}
	m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, part)
	m.bridge.finishOpenCodeStream(turnID)
	state.finished = true
	inst.removeTurnState(sessionID, messageID)
}

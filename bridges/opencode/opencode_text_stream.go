package opencode

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/agentremote/bridges/opencode/api"
)

func opencodeMessageStreamTurnID(sessionID, messageID string) string {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	if sessionID != "" && messageID != "" {
		return "opencode-msg-" + sessionID + "-" + messageID
	}
	if messageID != "" {
		return "opencode-msg-" + messageID
	}
	return ""
}

func opencodePartStreamID(part api.Part, kind string) string {
	if part.ID == "" {
		return ""
	}
	if kind == "reasoning" {
		return "reasoning-" + part.ID
	}
	return "text-" + part.ID
}

// partTurnID returns the stream turn ID for a part, falling back to the part ID.
func partTurnID(part api.Part) string {
	turnID := opencodeMessageStreamTurnID(part.SessionID, part.MessageID)
	if turnID == "" {
		return "opencode-part-" + part.ID
	}
	return turnID
}

func (m *OpenCodeManager) emitTextStreamDeltaForKind(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, part api.Part, delta, kind string) {
	if m == nil || m.bridge == nil || portal == nil || inst == nil || delta == "" {
		return
	}
	turnID := partTurnID(part)
	partID := opencodePartStreamID(part, kind)
	if partID == "" {
		return
	}
	agentID := m.bridge.portalAgentID(portal)
	m.closeStepIfOpen(ctx, inst, portal, part.SessionID, part.MessageID)
	m.ensureTurnStarted(ctx, inst, portal, part.SessionID, part.MessageID, nil)

	started, _ := inst.partTextStreamFlags(part.SessionID, part.ID).forKind(kind)
	if client, ok := m.bridge.host.(*OpenCodeClient); ok {
		if streamState, writer := client.ensureStreamWriter(ctx, portal, turnID, agentID); writer != nil {
			if kind == "reasoning" {
				writer.ReasoningDelta(ctx, delta)
				streamState.accumulated.WriteString(delta)
			} else {
				writer.TextDelta(ctx, delta)
				streamState.visible.WriteString(delta)
				streamState.accumulated.WriteString(delta)
			}
			if !started {
				inst.setPartTextStreamStarted(part.SessionID, part.ID, kind)
			}
			inst.appendPartTextContent(part.SessionID, part.ID, kind, delta)
			return
		}
	}
	m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
		"type":  kind + "-delta",
		"id":    partID,
		"delta": delta,
	})
	inst.appendPartTextContent(part.SessionID, part.ID, kind, delta)
}

func (m *OpenCodeManager) emitTextStreamEnd(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, part api.Part) {
	if m == nil || m.bridge == nil || portal == nil || inst == nil {
		return
	}
	if part.Time == nil || part.Time.End == 0 {
		return
	}
	if part.Type != "text" && part.Type != "reasoning" {
		return
	}
	kind := part.Type
	turnID := partTurnID(part)
	partID := opencodePartStreamID(part, kind)
	if partID == "" {
		return
	}
	agentID := m.bridge.portalAgentID(portal)
	started, ended := inst.partTextStreamFlags(part.SessionID, part.ID).forKind(kind)
	if !started || ended {
		return
	}
	if client, ok := m.bridge.host.(*OpenCodeClient); ok {
		if _, writer := client.ensureStreamWriter(ctx, portal, turnID, agentID); writer != nil {
			if kind == "reasoning" {
				writer.FinishReasoning(ctx)
			} else {
				writer.FinishText(ctx)
			}
			inst.setPartTextStreamEnded(part.SessionID, part.ID, kind)
			return
		}
	}
	m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
		"type": kind + "-end",
		"id":   partID,
	})
	inst.setPartTextStreamEnded(part.SessionID, part.ID, kind)
}

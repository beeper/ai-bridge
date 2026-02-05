package opencodebridge

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/ai-bridge/pkg/opencode"
)

func opencodeToolCallID(part opencode.Part) string {
	callID := strings.TrimSpace(part.CallID)
	if callID == "" {
		callID = part.ID
	}
	return callID
}

func opencodeToolName(part opencode.Part) string {
	toolName := strings.TrimSpace(part.Tool)
	if toolName == "" {
		toolName = "tool"
	}
	return toolName
}

func opencodeToolStreamTurnID(part opencode.Part) string {
	if part.ID == "" {
		return ""
	}
	return "opencode-tool-" + part.ID
}

func (m *OpenCodeManager) emitToolStreamDelta(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, part opencode.Part, delta string) {
	if m == nil || m.bridge == nil || portal == nil {
		return
	}
	if delta == "" {
		return
	}
	turnID := opencodeToolStreamTurnID(part)
	toolCallID := opencodeToolCallID(part)
	if turnID == "" || toolCallID == "" {
		return
	}
	toolName := opencodeToolName(part)
	meta := m.bridge.portalMeta(portal)
	agentID := ""
	if meta != nil {
		agentID = meta.DefaultAgentID
	}
	started, _, _, _ := inst.partStreamFlags(part.SessionID, part.ID)
	if !started {
		m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
			"type":             "tool-input-start",
			"toolCallId":       toolCallID,
			"toolName":         toolName,
			"providerExecuted": false,
		})
		inst.setPartStreamInputStarted(part.SessionID, part.ID)
	}
	m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
		"type":           "tool-input-delta",
		"toolCallId":     toolCallID,
		"inputTextDelta": delta,
	})
}

func (m *OpenCodeManager) emitToolStreamState(ctx context.Context, inst *openCodeInstance, portal *bridgev2.Portal, part opencode.Part, status string) {
	if m == nil || m.bridge == nil || portal == nil {
		return
	}
	if part.State == nil {
		if status == "completed" || status == "error" {
			m.bridge.finishOpenCodeStream(opencodeToolStreamTurnID(part))
		}
		return
	}
	turnID := opencodeToolStreamTurnID(part)
	toolCallID := opencodeToolCallID(part)
	if turnID == "" || toolCallID == "" {
		return
	}
	toolName := opencodeToolName(part)
	meta := m.bridge.portalMeta(portal)
	agentID := ""
	if meta != nil {
		agentID = meta.DefaultAgentID
	}
	started, inputAvailable, outputAvailable, outputError := inst.partStreamFlags(part.SessionID, part.ID)

	if len(part.State.Input) > 0 && !inputAvailable {
		if !started {
			m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
				"type":             "tool-input-start",
				"toolCallId":       toolCallID,
				"toolName":         toolName,
				"providerExecuted": false,
			})
			inst.setPartStreamInputStarted(part.SessionID, part.ID)
			started = true
		}
		m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
			"type":             "tool-input-available",
			"toolCallId":       toolCallID,
			"toolName":         toolName,
			"input":            part.State.Input,
			"providerExecuted": false,
		})
		inst.setPartStreamInputAvailable(part.SessionID, part.ID)
	}

	if part.State.Output != "" && !outputAvailable {
		m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
			"type":             "tool-output-available",
			"toolCallId":       toolCallID,
			"output":           part.State.Output,
			"providerExecuted": false,
		})
		inst.setPartStreamOutputAvailable(part.SessionID, part.ID)
	}

	if part.State.Error != "" && !outputError {
		m.bridge.emitOpenCodeStreamEvent(ctx, portal, turnID, agentID, map[string]any{
			"type":             "tool-output-error",
			"toolCallId":       toolCallID,
			"errorText":        part.State.Error,
			"providerExecuted": false,
		})
		inst.setPartStreamOutputError(part.SessionID, part.ID)
	}

	if status == "completed" || status == "error" {
		m.bridge.finishOpenCodeStream(turnID)
	}
}

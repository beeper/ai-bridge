package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/cron"
)

func (oc *AIClient) runCronIsolatedAgentJob(job cron.CronJob, message string) (status string, summary string, outputText string, err error) {
	if oc == nil || oc.UserLogin == nil {
		return "error", "", "", fmt.Errorf("missing client")
	}
	ctx := oc.backgroundContext(context.Background())
	agentID := resolveCronAgentID(job.AgentID, &oc.connector.Config)
	portal, err := oc.getOrCreateCronRoom(ctx, agentID, job.ID, job.Name)
	if err != nil {
		return "error", "", "", err
	}
	meta := portalMeta(portal)
	metaSnapshot := clonePortalMetadata(meta)
	if metaSnapshot == nil {
		metaSnapshot = &PortalMetadata{}
	}
	metaSnapshot.AgentID = agentID
	metaSnapshot.DefaultAgentID = agentID

	// Apply model override for this run if provided.
	if strings.TrimSpace(job.Payload.Model) != "" {
		metaSnapshot.Model = strings.TrimSpace(job.Payload.Model)
	}
	if strings.TrimSpace(job.Payload.Thinking) != "" {
		if level, ok := normalizeThinkingLevel(job.Payload.Thinking); ok {
			if level == "off" {
				metaSnapshot.ReasoningEffort = ""
			} else {
				metaSnapshot.ReasoningEffort = level
			}
		}
	}

	timeoutMs := int64(2 * 60 * 1000)
	if job.Payload.TimeoutSeconds != nil && *job.Payload.TimeoutSeconds > 0 {
		timeoutMs = int64(*job.Payload.TimeoutSeconds) * 1000
	}

	sessionKey := cronSessionKey(agentID, job.ID)
	runID := newCronSessionID()
	oc.updateCronSessionEntry(ctx, sessionKey, func(entry cronSessionEntry) cronSessionEntry {
		entry.SessionID = runID
		entry.UpdatedAt = cronSessionUpdatedAt()
		return entry
	})

	userTimezone, _ := oc.resolveUserTimezone()
	cronMessage := buildCronMessage(job.ID, job.Name, message, userTimezone)

	// Capture last assistant message before dispatch.
	lastID, lastTimestamp := oc.lastAssistantMessageInfo(ctx, portal)

	var toolCalls []ToolCallMetadata
	eventID, _, dispatchErr := oc.dispatchInternalMessage(ctx, portal, metaSnapshot, cronMessage, "cron", false)
	if dispatchErr != nil {
		return "error", "", "", dispatchErr
	}
	_ = eventID

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for time.Now().Before(deadline) {
		msg, found := oc.waitForNewAssistantMessage(ctx, portal, lastID, lastTimestamp)
		if found {
			body := ""
			if msg != nil {
				if meta := messageMeta(msg); meta != nil {
					body = strings.TrimSpace(meta.Body)
					toolCalls = meta.ToolCalls
					oc.updateCronSessionEntry(ctx, sessionKey, func(entry cronSessionEntry) cronSessionEntry {
						entry.Model = strings.TrimSpace(meta.Model)
						entry.PromptTokens = meta.PromptTokens
						entry.CompletionTokens = meta.CompletionTokens
						total := meta.PromptTokens + meta.CompletionTokens
						if total > 0 {
							entry.TotalTokens = total
						}
						entry.UpdatedAt = cronSessionUpdatedAt()
						return entry
					})
				}
			}
			outputText = body
			summary = truncateTextForCronSummary(body)
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if outputText == "" {
		return "error", "", "", fmt.Errorf("cron job timed out")
	}

	payload := job.Payload
	deliveryMode := "off"
	if payload.Deliver != nil {
		if *payload.Deliver {
			deliveryMode = "explicit"
		} else {
			deliveryMode = "off"
		}
	} else if strings.TrimSpace(payload.To) != "" {
		deliveryMode = "auto"
	}
	deliveryRequested := deliveryMode == "explicit" || deliveryMode == "auto"
	bestEffort := payload.BestEffortDeliver != nil && *payload.BestEffortDeliver

	ackMax := resolveHeartbeatAckMaxChars(&oc.connector.Config, resolveHeartbeatConfig(&oc.connector.Config, agentID))
	skipHeartbeatDelivery := deliveryRequested && isHeartbeatOnlyText(outputText, ackMax)

	if deliveryRequested && !skipHeartbeatDelivery {
		target := oc.resolveCronDeliveryTarget(agentID, payload)
		if target.Portal == nil || target.RoomID == "" {
			reason := strings.TrimSpace(target.Reason)
			if reason == "" {
				reason = "no-target"
			}
			if bestEffort {
				return "skipped", fmt.Sprintf("Delivery skipped (%s).", reason), outputText, nil
			}
			return "error", summary, outputText, fmt.Errorf("cron delivery failed: %s", reason)
		}
		skipMessagingTool := deliveryMode == "auto" && hasMessagingToolDelivery(toolCalls, payload, target)
		if strings.TrimSpace(outputText) != "" && !skipMessagingTool {
			oc.sendPlainAssistantMessage(ctx, target.Portal, outputText)
		}
	}

	return "ok", summary, outputText, nil
}

func (oc *AIClient) lastAssistantMessageInfo(ctx context.Context, portal *bridgev2.Portal) (string, int64) {
	if portal == nil {
		return "", 0
	}
	messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 5)
	if err != nil {
		return "", 0
	}
	for i := len(messages) - 1; i >= 0; i-- {
		meta := messageMeta(messages[i])
		if meta == nil || meta.Role != "assistant" {
			continue
		}
		return messages[i].MXID.String(), messages[i].Timestamp.UnixMilli()
	}
	return "", 0
}

func (oc *AIClient) waitForNewAssistantMessage(ctx context.Context, portal *bridgev2.Portal, lastID string, lastTimestamp int64) (*database.Message, bool) {
	if portal == nil {
		return nil, false
	}
	messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 5)
	if err != nil {
		return nil, false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		meta := messageMeta(msg)
		if meta == nil || meta.Role != "assistant" {
			continue
		}
		if msg.MXID.String() == lastID {
			return nil, false
		}
		if msg.Timestamp.UnixMilli() <= lastTimestamp {
			return nil, false
		}
		return msg, true
	}
	return nil, false
}

func truncateTextForCronSummary(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const max = 2000
	if len(trimmed) <= max {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:max]) + "…"
}

func isHeartbeatOnlyText(text string, ackMax int) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	shouldSkip, stripped, _ := agents.StripHeartbeatTokenWithMode(trimmed, agents.StripHeartbeatModeHeartbeat, ackMax)
	if shouldSkip && strings.TrimSpace(stripped) == "" {
		return true
	}
	return false
}

func hasMessagingToolDelivery(calls []ToolCallMetadata, payload cron.CronPayload, target cronDeliveryTarget) bool {
	if len(calls) == 0 {
		return false
	}
	desiredTo := strings.TrimSpace(payload.To)
	if desiredTo == "" && target.RoomID != "" {
		desiredTo = target.RoomID.String()
	}
	for _, call := range calls {
		name := strings.ToLower(strings.TrimSpace(call.ToolName))
		if name != "message" && !strings.HasPrefix(name, "message.") {
			continue
		}
		rawTo, ok := call.Input["to"]
		if !ok {
			continue
		}
		to, ok := rawTo.(string)
		if !ok {
			continue
		}
		if desiredTo != "" && strings.TrimSpace(to) == desiredTo {
			return true
		}
	}
	return false
}

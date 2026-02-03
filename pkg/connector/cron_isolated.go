package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

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

	// Capture last assistant message before dispatch.
	lastID, lastTimestamp := oc.lastAssistantMessageInfo(ctx, portal)

	eventID, _, dispatchErr := oc.dispatchInternalMessage(ctx, portal, metaSnapshot, message, "cron", false)
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
				}
			}
			outputText = body
			summary = truncateTextForCronSummary(body)
			return "ok", summary, outputText, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "error", "", "", fmt.Errorf("cron job timed out")
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
	const max = 200
	if len(trimmed) <= max {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:max]) + "…"
}

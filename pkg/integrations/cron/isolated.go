package cron

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beeper/ai-bridge/pkg/agents"
	croncore "github.com/beeper/ai-bridge/pkg/cron"
)

const DefaultCronDeliveryTimeout = 10 * time.Second

type AssistantMessage struct {
	Body             string
	Model            string
	PromptTokens     int64
	CompletionTokens int64
}

type MetadataPatch struct {
	AgentID            string
	Model              *string
	ReasoningEffort    *string
	DisableMessageTool bool
}

type IsolatedRunnerDeps struct {
	DeliveryTimeout time.Duration

	MergeContext            func(ctx context.Context) (context.Context, context.CancelFunc)
	ResolveAgentID          func(raw string) string
	GetOrCreateRoom         func(ctx context.Context, agentID, jobID, jobName string) (any, error)
	BuildDispatchMetadata   func(room any, patch MetadataPatch) any
	NormalizeThinkingLevel  func(raw string) (string, bool)
	SessionKey              func(agentID, jobID string) string
	UpdateSessionEntry      func(ctx context.Context, sessionKey string, updater func(entry SessionEntry) SessionEntry)
	ResolveUserTimezone     func() string
	LastAssistantMessage    func(ctx context.Context, room any) (lastID string, lastTimestamp int64)
	DispatchInternalMessage func(ctx context.Context, room any, metadata any, message string) error
	WaitForAssistantMessage func(ctx context.Context, room any, lastID string, lastTimestamp int64) (AssistantMessage, bool)
	ResolveAckMaxChars      func(agentID string) int
	ResolveDeliveryTarget   func(agentID string, delivery *croncore.CronDelivery) DeliveryTarget
	SendDeliveryMessage     func(ctx context.Context, portal any, body string) error
}

func RunCronIsolatedAgentJob(
	ctx context.Context,
	job croncore.CronJob,
	message string,
	deps IsolatedRunnerDeps,
) (status string, summary string, outputText string, err error) {
	if deps.MergeContext == nil || deps.GetOrCreateRoom == nil || deps.DispatchInternalMessage == nil || deps.WaitForAssistantMessage == nil {
		return "error", "", "", errors.New("missing cron isolated runner dependencies")
	}

	runCtx, cancel := deps.MergeContext(ctx)
	defer cancel()

	agentID := job.AgentID
	if deps.ResolveAgentID != nil {
		agentID = deps.ResolveAgentID(job.AgentID)
	}
	room, err := deps.GetOrCreateRoom(runCtx, agentID, job.ID, job.Name)
	if err != nil {
		return "error", "", "", err
	}

	delivery := job.Delivery
	deliveryMode := croncore.CronDeliveryAnnounce
	if delivery != nil && strings.TrimSpace(string(delivery.Mode)) != "" {
		deliveryMode = delivery.Mode
	}
	if delivery == nil {
		delivery = &croncore.CronDelivery{Mode: deliveryMode}
	}

	patch := MetadataPatch{AgentID: agentID}
	if model := strings.TrimSpace(job.Payload.Model); model != "" {
		patch.Model = &model
	}
	if thinking := strings.TrimSpace(job.Payload.Thinking); thinking != "" && deps.NormalizeThinkingLevel != nil {
		if level, ok := deps.NormalizeThinkingLevel(thinking); ok {
			if level == "off" {
				empty := ""
				patch.ReasoningEffort = &empty
			} else {
				patch.ReasoningEffort = &level
			}
		}
	}
	if deliveryMode == croncore.CronDeliveryAnnounce {
		patch.DisableMessageTool = true
	}

	metadata := any(nil)
	if deps.BuildDispatchMetadata != nil {
		metadata = deps.BuildDispatchMetadata(room, patch)
	}

	sessionKey := fmt.Sprintf("agent:%s:cron:%s", strings.TrimSpace(agentID), strings.TrimSpace(job.ID))
	if deps.SessionKey != nil {
		sessionKey = deps.SessionKey(agentID, job.ID)
	}
	if deps.UpdateSessionEntry != nil {
		runID := uuid.NewString()
		deps.UpdateSessionEntry(runCtx, sessionKey, func(entry SessionEntry) SessionEntry {
			entry.SessionID = runID
			entry.UpdatedAt = time.Now().UnixMilli()
			return entry
		})
	}

	userTimezone := ""
	if deps.ResolveUserTimezone != nil {
		userTimezone = deps.ResolveUserTimezone()
	}
	cronMessage := BuildCronMessage(job.ID, job.Name, message, userTimezone)
	if job.Payload.AllowUnsafeExternal == nil || !*job.Payload.AllowUnsafeExternal {
		cronMessage = WrapSafeExternalPrompt(cronMessage)
	}

	lastID, lastTimestamp := "", int64(0)
	if deps.LastAssistantMessage != nil {
		lastID, lastTimestamp = deps.LastAssistantMessage(runCtx, room)
	}

	if err := deps.DispatchInternalMessage(runCtx, room, metadata, cronMessage); err != nil {
		return "error", "", "", err
	}

	for {
		if err := runCtx.Err(); err != nil {
			return "error", "", "", errors.New("cron job timed out")
		}
		msg, found := deps.WaitForAssistantMessage(runCtx, room, lastID, lastTimestamp)
		if found {
			body := strings.TrimSpace(msg.Body)
			if deps.UpdateSessionEntry != nil {
				deps.UpdateSessionEntry(runCtx, sessionKey, func(entry SessionEntry) SessionEntry {
					entry.Model = strings.TrimSpace(msg.Model)
					entry.PromptTokens = msg.PromptTokens
					entry.CompletionTokens = msg.CompletionTokens
					total := msg.PromptTokens + msg.CompletionTokens
					if total > 0 {
						entry.TotalTokens = total
					}
					entry.UpdatedAt = time.Now().UnixMilli()
					return entry
				})
			}
			outputText = body
			summary = truncateTextForCronSummary(body)
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	if outputText == "" {
		return "error", "", "", errors.New("cron job timed out")
	}

	deliveryRequested := deliveryMode == croncore.CronDeliveryAnnounce
	bestEffort := delivery != nil && delivery.BestEffort != nil && *delivery.BestEffort

	ackMax := 0
	if deps.ResolveAckMaxChars != nil {
		ackMax = deps.ResolveAckMaxChars(agentID)
	}
	skipHeartbeatDelivery := deliveryRequested && isHeartbeatOnlyText(outputText, ackMax)

	if deliveryRequested && !skipHeartbeatDelivery {
		target := DeliveryTarget{}
		if deps.ResolveDeliveryTarget != nil {
			target = deps.ResolveDeliveryTarget(agentID, delivery)
		}
		if target.Portal == nil || strings.TrimSpace(target.RoomID) == "" {
			reason := strings.TrimSpace(target.Reason)
			if reason == "" {
				reason = "no-target"
			}
			if bestEffort {
				return "skipped", fmt.Sprintf("Delivery skipped (%s).", reason), outputText, nil
			}
			return "error", summary, outputText, fmt.Errorf("cron delivery failed: %s", reason)
		}
		if strings.TrimSpace(outputText) != "" {
			if deps.SendDeliveryMessage == nil {
				if bestEffort {
					return "skipped", "Delivery skipped (missing sender).", outputText, nil
				}
				return "error", summary, outputText, errors.New("cron delivery failed: missing sender")
			}
			timeout := deps.DeliveryTimeout
			if timeout <= 0 {
				timeout = DefaultCronDeliveryTimeout
			}
			deliveryCtx, cancel := context.WithTimeout(runCtx, timeout)
			defer cancel()
			if sendErr := deps.SendDeliveryMessage(deliveryCtx, target.Portal, outputText); sendErr != nil {
				if bestEffort {
					return "skipped", fmt.Sprintf("Delivery skipped (%s).", sendErr.Error()), outputText, nil
				}
				return "error", summary, outputText, fmt.Errorf("cron delivery failed: %w", sendErr)
			}
		}
	}

	return "ok", summary, outputText, nil
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
	return strings.TrimSpace(trimmed[:max]) + "..."
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

package connector

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
)

func (oc *AIClient) buildStatusText(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	isGroup bool,
	queueSettings QueueSettings,
) string {
	if meta == nil {
		return "Status unavailable."
	}
	var sb strings.Builder
	sb.WriteString("Status\n")
	sb.WriteString(fmt.Sprintf("Model: %s\n", oc.effectiveModel(meta)))

	thinking := meta.ThinkingLevel
	if thinking == "" {
		if meta.EmitThinking {
			thinking = "on"
		} else {
			thinking = "off"
		}
	}
	sb.WriteString(fmt.Sprintf("Thinking: %s\n", thinking))

	reasoning := strings.TrimSpace(meta.ReasoningEffort)
	if reasoning == "" {
		if meta.EmitThinking {
			reasoning = "on"
		} else {
			reasoning = "off"
		}
	}
	sb.WriteString(fmt.Sprintf("Reasoning: %s\n", reasoning))

	verbose := strings.TrimSpace(meta.VerboseLevel)
	if verbose == "" {
		verbose = "off"
	}
	sb.WriteString(fmt.Sprintf("Verbosity: %s\n", verbose))

	sendPolicy := normalizeSendPolicyMode(meta.SendPolicy)
	if sendPolicy == "" {
		sendPolicy = "allow"
	}
	sendLabel := "on"
	if sendPolicy == "deny" {
		sendLabel = "off"
	}
	sb.WriteString(fmt.Sprintf("Send: %s\n", sendLabel))

	if isGroup {
		activation := oc.resolveGroupActivation(meta)
		sb.WriteString(fmt.Sprintf("Group activation: %s\n", activation))
	}

	sb.WriteString(fmt.Sprintf(
		"Queue: mode=%s, debounce=%dms, cap=%d, drop=%s\n",
		queueSettings.Mode,
		queueSettings.DebounceMs,
		queueSettings.Cap,
		queueSettings.DropPolicy,
	))

	if meta.SessionResetAt > 0 {
		ts := time.UnixMilli(meta.SessionResetAt).Format(time.RFC3339)
		sb.WriteString(fmt.Sprintf("Session reset: %s\n", ts))
	}

	return strings.TrimSpace(sb.String())
}

func (oc *AIClient) buildContextStatus(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata) string {
	if meta == nil || portal == nil {
		return "Context unavailable."
	}
	historyLimit := oc.historyLimit(meta)
	historyCount := 0
	if historyLimit > 0 {
		if history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit); err == nil {
			historyCount = len(history)
		}
	}
	resetLine := ""
	if meta.SessionResetAt > 0 {
		resetLine = fmt.Sprintf("Session reset at: %s\n", time.UnixMilli(meta.SessionResetAt).Format(time.RFC3339))
	}
	return strings.TrimSpace(fmt.Sprintf(
		"Context\n%sHistory limit: %d messages\nHistory loaded: %d messages",
		resetLine,
		historyLimit,
		historyCount,
	))
}

func (oc *AIClient) buildToolsStatusText(meta *PortalMetadata) string {
	var sb strings.Builder
	sb.WriteString("Tool Status:\n\n")

	toolsList := oc.buildAvailableTools(meta)
	sort.Slice(toolsList, func(i, j int) bool {
		return toolsList[i].Name < toolsList[j].Name
	})

	sb.WriteString("Tools:\n")
	for _, tool := range toolsList {
		status := "✗"
		if tool.Enabled {
			status = "✓"
		}
		desc := tool.Description
		if desc == "" {
			desc = tool.DisplayName
		}
		reason := ""
		if !tool.Enabled && tool.Reason != "" {
			reason = fmt.Sprintf(" (%s)", tool.Reason)
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s: %s%s\n", status, tool.Name, desc, reason))
	}

	if meta != nil && !meta.Capabilities.SupportsToolCalling {
		sb.WriteString(fmt.Sprintf("\nNote: Current model (%s) may not support tool calling.\n", oc.effectiveModel(meta)))
	}

	return strings.TrimSpace(sb.String())
}

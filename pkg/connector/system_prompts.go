package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2"
)

func buildGroupIntro(roomName string, activation string) string {
	subjectLine := "You are replying inside a bridged group chat (Matrix room)."
	if strings.TrimSpace(roomName) != "" {
		subjectLine = "You are replying inside the group \"" + strings.TrimSpace(roomName) + "\" (Matrix room)."
	}
	activationLine := "Activation: trigger-only (you are invoked only when explicitly mentioned; recent context may be included)."
	if activation == "always" {
		activationLine = "Activation: always-on (you receive every group message)."
	}
	lines := []string{subjectLine, activationLine}
	if activation == "always" {
		lines = append(lines,
			"If no response is needed, reply with exactly \""+SilentReplyToken+"\" (and nothing else) so the bridge stays silent.",
			"Be extremely selective: reply only when directly addressed or clearly helpful. Otherwise stay silent.",
		)
	}
	lines = append(lines,
		"Be a good group participant: mostly lurk and follow the conversation; reply only when directly addressed or you can add clear value.",
		"Write like a human. Avoid Markdown tables. Use real line breaks sparingly.",
	)
	return strings.Join(lines, " ") + " Address the specific sender noted in the message context."
}

func buildVerboseSystemHint(meta *PortalMetadata) string {
	if meta == nil {
		return ""
	}
	level := strings.ToLower(strings.TrimSpace(meta.VerboseLevel))
	switch level {
	case "on":
		return "Verbosity: on. Provide a bit more detail and context when helpful, but stay focused."
	case "full":
		return "Verbosity: full. Be thorough and detailed. Explain assumptions and reasoning clearly, without unnecessary fluff."
	default:
		return ""
	}
}

func buildSessionIdentityHint(portal *bridgev2.Portal, meta *PortalMetadata) string {
	if portal == nil {
		return ""
	}

	// Prefer Matrix room ID as the canonical sessionKey for tools/user-facing references.
	roomID := ""
	if portal.MXID != "" {
		roomID = strings.TrimSpace(portal.MXID.String())
	}
	portalID := strings.TrimSpace(string(portal.PortalKey.ID))

	if roomID == "" && portalID == "" {
		return ""
	}

	parts := make([]string, 0, 6)
	parts = append(parts, "Session identity:")
	parts = append(parts, "channel=matrix")
	if roomID != "" {
		parts = append(parts, "sessionKey="+roomID)
	}
	if portalID != "" && portalID != roomID {
		parts = append(parts, "portalId="+portalID)
	}
	if meta != nil && strings.TrimSpace(meta.AgentID) != "" {
		parts = append(parts, "agentId="+strings.TrimSpace(meta.AgentID))
	}
	if meta != nil && meta.IsCronRoom {
		if strings.TrimSpace(meta.CronJobID) != "" {
			parts = append(parts, "cronJobId="+strings.TrimSpace(meta.CronJobID))
		} else {
			parts = append(parts, "cronRoom=true")
		}
	}

	if meta != nil && meta.IsCronRoom {
		parts = append(parts, "Note: this is an internal cron room; the cron runner delivers results to the configured target room.")
	}

	// Make the labels actionable: they match the `sessions_*` tools (resolveSessionPortal supports both).
	parts = append(parts, fmt.Sprintf("Use sessionKey (preferred) or portalId to refer to this room in tools (e.g. sessions_send/session_focus)."))
	return strings.Join(parts, " ")
}

func (oc *AIClient) buildAdditionalSystemPrompts(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) []openai.ChatCompletionMessageParamUnion {
	var out []openai.ChatCompletionMessageParamUnion

	if meta != nil && portal != nil && oc.isGroupChat(ctx, portal) {
		activation := oc.resolveGroupActivation(meta)
		shouldIntro := !meta.GroupIntroSent || meta.GroupActivationNeedsIntro
		if shouldIntro {
			intro := buildGroupIntro(oc.matrixRoomDisplayName(ctx, portal), activation)
			if strings.TrimSpace(intro) != "" {
				out = append(out, openai.SystemMessage(intro))
			}
			meta.GroupIntroSent = true
			meta.GroupActivationNeedsIntro = false
			oc.savePortalQuiet(ctx, portal, "group intro")
		}
	}

	if meta != nil {
		if verboseHint := buildVerboseSystemHint(meta); verboseHint != "" {
			out = append(out, openai.SystemMessage(verboseHint))
		}
	}

	if accountHint := oc.buildDesktopAccountHintPrompt(ctx); accountHint != "" {
		out = append(out, openai.SystemMessage(accountHint))
	}

	if ident := buildSessionIdentityHint(portal, meta); ident != "" {
		out = append(out, openai.SystemMessage(ident))
	}

	return out
}

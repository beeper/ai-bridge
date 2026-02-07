package connector

import (
	"strings"

	"github.com/beeper/ai-bridge/pkg/textfs"
)

func readStringArgAny(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func (oc *AIClient) builtinToolApprovalRequirement(toolName string, args map[string]any) (required bool, action string) {
	if oc == nil || !oc.toolApprovalsRuntimeEnabled() {
		return false, ""
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" || !oc.toolApprovalsRequireForTool(toolName) {
		return false, ""
	}
	switch normalizeApprovalToken(toolName) {
	case normalizeApprovalToken(ToolNameMessage):
		action = normalizeMessageAction(readStringArgAny(args, "action"))
		switch action {
		// Read-only / non-destructive actions (do not require approval).
		case "reactions", "search", "read", "member-info", "channel-info", "list-pins",
			// Desktop API read-only surface (ai-bridge message tool actions).
			"desktop-list-chats", "desktop-search-chats", "desktop-search-messages", "desktop-download-asset":
			return false, action
		default:
			return true, action
		}
	case normalizeApprovalToken(ToolNameWrite), normalizeApprovalToken(ToolNameEdit):
		// Virtual FS writes to MEMORY.md/memory/* are low-risk and a core workflow.
		// Keep approvals for non-memory paths.
		p := readStringArgAny(args, "path")
		if p == "" {
			p = readStringArgAny(args, "file_path")
		}
		if normalized, err := textfs.NormalizePath(p); err == nil && textfs.IsMemoryPath(normalized) {
			return false, "memory"
		}
		return true, "workspace"
	case normalizeApprovalToken(ToolNameCron):
		action = normalizeApprovalToken(readStringArgAny(args, "action"))
		switch action {
		case "status", "list", "runs":
			return false, action
		default:
			return true, action
		}
	default:
		return true, ""
	}
}

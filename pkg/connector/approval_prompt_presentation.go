package connector

import (
	"fmt"
	"sort"
	"strings"

	"github.com/beeper/agentremote/pkg/bridgeadapter"
)

func buildBuiltinApprovalPresentation(toolName, action string, args map[string]any) bridgeadapter.ApprovalPromptPresentation {
	toolName = strings.TrimSpace(toolName)
	action = strings.TrimSpace(action)
	title := "Builtin tool request"
	if toolName != "" {
		title = "Builtin tool request: " + toolName
	}
	details := make([]bridgeadapter.ApprovalDetail, 0, 10)
	if toolName != "" {
		details = append(details, bridgeadapter.ApprovalDetail{Label: "Tool", Value: toolName})
	}
	if action != "" {
		details = append(details, bridgeadapter.ApprovalDetail{Label: "Action", Value: action})
	}
	details = appendApprovalDetailsFromMap(details, "Arg", args, 8)
	return bridgeadapter.ApprovalPromptPresentation{
		Title:       title,
		Details:     details,
		AllowAlways: true,
	}
}

func buildMCPApprovalPresentation(serverLabel, toolName string, input any) bridgeadapter.ApprovalPromptPresentation {
	serverLabel = strings.TrimSpace(serverLabel)
	toolName = strings.TrimSpace(toolName)
	title := "MCP tool request"
	if toolName != "" {
		title = "MCP tool request: " + toolName
	}
	details := make([]bridgeadapter.ApprovalDetail, 0, 10)
	if serverLabel != "" {
		details = append(details, bridgeadapter.ApprovalDetail{Label: "Server", Value: serverLabel})
	}
	if toolName != "" {
		details = append(details, bridgeadapter.ApprovalDetail{Label: "Tool", Value: toolName})
	}
	if inputMap, ok := input.(map[string]any); ok && len(inputMap) > 0 {
		details = appendApprovalDetailsFromMap(details, "Input", inputMap, 8)
	} else if summary := approvalValueSummary(input); summary != "" {
		details = append(details, bridgeadapter.ApprovalDetail{Label: "Input", Value: summary})
	}
	return bridgeadapter.ApprovalPromptPresentation{
		Title:       title,
		Details:     details,
		AllowAlways: true,
	}
}

func appendApprovalDetailsFromMap(details []bridgeadapter.ApprovalDetail, labelPrefix string, values map[string]any, max int) []bridgeadapter.ApprovalDetail {
	if len(values) == 0 || max <= 0 {
		return details
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	count := 0
	for _, key := range keys {
		if count >= max {
			break
		}
		if value := approvalValueSummary(values[key]); value != "" {
			details = append(details, bridgeadapter.ApprovalDetail{
				Label: fmt.Sprintf("%s %s", labelPrefix, key),
				Value: value,
			})
			count++
		}
	}
	if len(keys) > max {
		details = append(details, bridgeadapter.ApprovalDetail{
			Label: "Input",
			Value: fmt.Sprintf("%d additional field(s)", len(keys)-max),
		})
	}
	return details
}

func approvalValueSummary(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case *string:
		if typed == nil {
			return ""
		}
		return strings.TrimSpace(*typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64, float32, float64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", typed)
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		if len(items) == 0 {
			return ""
		}
		if len(items) > 3 {
			return fmt.Sprintf("%s (+%d more)", strings.Join(items[:3], ", "), len(items)-3)
		}
		return strings.Join(items, ", ")
	case []any:
		if len(typed) == 0 {
			return ""
		}
		return fmt.Sprintf("%d item(s)", len(typed))
	case map[string]any:
		if len(typed) == 0 {
			return ""
		}
		return fmt.Sprintf("%d field(s)", len(typed))
	default:
		serialized := strings.TrimSpace(stringifyJSONValue(typed))
		if len(serialized) > 160 {
			return serialized[:160] + "..."
		}
		return serialized
	}
}

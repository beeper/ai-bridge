package connector

import "fmt"

func applyThinkingLevel(meta *PortalMetadata, level string) {
	if meta == nil {
		return
	}
	meta.ThinkingLevel = level
	meta.EmitThinking = level != "off"
	if level == "minimal" {
		meta.ReasoningEffort = "low"
	} else if level == "low" || level == "medium" || level == "high" || level == "xhigh" {
		meta.ReasoningEffort = level
	}
}

func applyReasoningLevel(meta *PortalMetadata, level string) {
	if meta == nil {
		return
	}
	if level == "off" {
		meta.EmitThinking = false
		meta.ReasoningEffort = ""
		return
	}
	if level == "on" {
		meta.EmitThinking = true
		return
	}
	meta.EmitThinking = true
	meta.ReasoningEffort = level
}

func formatThinkingAck(level string) string {
	if level == "off" {
		return "Thinking disabled."
	}
	return fmt.Sprintf("Thinking level set to %s.", level)
}

func formatVerboseAck(level string) string {
	switch level {
	case "off":
		return formatSystemAck("Verbose logging disabled.")
	case "full":
		return formatSystemAck("Verbose logging set to full.")
	default:
		return formatSystemAck("Verbose logging enabled.")
	}
}

func formatReasoningAck(level string) string {
	switch level {
	case "off":
		return formatSystemAck("Reasoning visibility disabled.")
	case "stream":
		return formatSystemAck("Reasoning stream enabled (Telegram only).")
	default:
		return formatSystemAck("Reasoning visibility enabled.")
	}
}

func formatElevatedAck(level string) string {
	switch level {
	case "off":
		return formatSystemAck("Elevated mode disabled.")
	case "full":
		return formatSystemAck("Elevated mode set to full (auto-approve).")
	default:
		return formatSystemAck("Elevated mode set to ask.")
	}
}

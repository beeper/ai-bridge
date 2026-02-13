package connector

func resolvePromptWorkspaceDir() string {
	return "/"
}

func resolvePromptReasoningLevel(meta *PortalMetadata) string {
	if meta != nil && meta.EmitThinking {
		return "on"
	}
	return ""
}

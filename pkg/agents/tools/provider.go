package tools

// Provider tools are markers for tools handled by the AI provider's API.
// They have no local Execute function - the provider handles execution.

// IsProviderTool returns true if the tool is handled by the provider API.
func IsProviderTool(t *Tool) bool {
	return t.Type == ToolTypeProvider || t.Type == ToolTypePlugin
}

// ProviderTools returns all provider/plugin tools.
func ProviderTools() []*Tool {
	return []*Tool{}
}

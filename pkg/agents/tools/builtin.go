package tools

// Tool group constants for policy composition.
const (
	GroupSearch  = "group:search"
	GroupCode    = "group:code"
	GroupCalc    = "group:calc"
	GroupOnline  = "group:online"
	GroupBuilder = "group:builder"
)

// BuiltinTools returns all locally-executable builtin tools.
func BuiltinTools() []*Tool {
	return []*Tool{
		Calculator,
		WebSearch,
	}
}

// AllTools returns all tools (builtin + provider markers).
func AllTools() []*Tool {
	tools := BuiltinTools()
	tools = append(tools, ProviderTools()...)
	return tools
}

// DefaultRegistry returns a registry with all default tools registered.
func DefaultRegistry() *Registry {
	reg := NewRegistry()

	// Register all tools
	for _, tool := range AllTools() {
		reg.Register(tool)
	}

	return reg
}

// BuiltinRegistry returns a registry with only builtin tools.
func BuiltinRegistry() *Registry {
	reg := NewRegistry()

	for _, tool := range BuiltinTools() {
		reg.Register(tool)
	}

	return reg
}

// GetBuiltinTool returns a builtin tool by name.
func GetBuiltinTool(name string) *Tool {
	for _, tool := range BuiltinTools() {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

// GetTool returns any tool by name (builtin or provider).
func GetTool(name string) *Tool {
	for _, tool := range AllTools() {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

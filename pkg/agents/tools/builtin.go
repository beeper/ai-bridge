package tools

import "sync"

var (
	toolLookupOnce    sync.Once
	builtinToolByName map[string]*Tool
	allToolByName     map[string]*Tool
)

// Tool group constants for policy composition (like OpenClaw's TOOL_GROUPS).
const (
	GroupSearch    = "group:search"
	GroupCalc      = "group:calc"
	GroupBuilder   = "group:builder"
	GroupChat      = "group:chat"
	GroupMessaging = "group:messaging"
	GroupSessions  = "group:sessions"
	GroupMemory    = "group:memory"
	GroupWeb       = "group:web"
	GroupMedia     = "group:media"
	GroupStatus    = "group:status"
)

// BuiltinTools returns all locally-executable builtin tools.
func BuiltinTools() []*Tool {
	return []*Tool{
		Calculator,
		WebSearch,
		WebSearchOpenRouter,
	}
}

// AllTools returns all tools (builtin + provider markers).
func AllTools() []*Tool {
	tools := BuiltinTools()
	tools = append(tools, SessionTools()...)
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
	toolLookupOnce.Do(initToolLookup)
	return builtinToolByName[name]
}

// GetTool returns any tool by name (builtin or provider).
func GetTool(name string) *Tool {
	toolLookupOnce.Do(initToolLookup)
	return allToolByName[name]
}

func initToolLookup() {
	builtinToolByName = make(map[string]*Tool)
	for _, tool := range BuiltinTools() {
		if _, exists := builtinToolByName[tool.Name]; !exists {
			builtinToolByName[tool.Name] = tool
		}
	}

	allToolByName = make(map[string]*Tool)
	for _, tool := range AllTools() {
		if _, exists := allToolByName[tool.Name]; !exists {
			allToolByName[tool.Name] = tool
		}
	}
}

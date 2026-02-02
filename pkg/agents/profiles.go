package agents

import (
	"slices"
	"strings"

	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// ToolProfile defines access levels (like OpenClaw's tool profiles).
type ToolProfile string

const (
	// ProfileMinimal allows only basic status tools.
	ProfileMinimal ToolProfile = "minimal"
	// ProfileCoding allows filesystem, runtime, and session tools.
	ProfileCoding ToolProfile = "coding"
	// ProfileMessaging allows messaging and session tools.
	ProfileMessaging ToolProfile = "messaging"
	// ProfileFull allows all standard tools.
	ProfileFull ToolProfile = "full"
	// ProfileBoss allows agent management tools.
	ProfileBoss ToolProfile = "boss"
)

// ToolGroups maps group names to tool names for policy composition.
// Matches OpenClaw's TOOL_GROUPS pattern.
var ToolGroups = map[string][]string{
	tools.GroupSearch:    {"web_search"},
	tools.GroupCalc:      {"calculator"},
	tools.GroupBuilder:   {"create_agent", "fork_agent", "edit_agent", "delete_agent", "list_agents", "list_models", "list_tools", "create_room", "modify_room", "list_rooms", "sessions_list", "sessions_history", "sessions_send"},
	tools.GroupChat:      {"set_chat_info"},
	tools.GroupMessaging: {"message"},
	tools.GroupSessions:  {"sessions_list", "sessions_history", "sessions_send"},
	tools.GroupMemory:    {"memory_search", "memory_get"},
	tools.GroupWeb:       {"web_search", "web_fetch"},
	tools.GroupMedia:     {"image", "tts"},
	tools.GroupStatus:    {"session_status"},
}

// ProfileAllowlists define which tool groups each profile allows.
// Matches OpenClaw's TOOL_PROFILES pattern.
var ProfileAllowlists = map[ToolProfile][]string{
	ProfileMinimal:   {tools.GroupSearch, tools.GroupChat, tools.GroupStatus},
	ProfileCoding:    {tools.GroupCalc, tools.GroupWeb, tools.GroupChat, tools.GroupMessaging, tools.GroupStatus},
	ProfileMessaging: {tools.GroupWeb, tools.GroupChat, tools.GroupMessaging, tools.GroupSessions, tools.GroupStatus},
	ProfileFull:      {tools.GroupCalc, tools.GroupWeb, tools.GroupChat, tools.GroupMessaging, tools.GroupMedia, tools.GroupStatus},
	ProfileBoss:      {tools.GroupBuilder, tools.GroupStatus},
}

// ResolveTools returns allowed tool names for an agent based on its profile and overrides.
func ResolveTools(agent *AgentDefinition, available []string) []string {
	if agent == nil {
		return nil
	}

	// Get tools from profile
	allowedSet := make(map[string]bool)

	profile := agent.ToolProfile
	if profile == "" {
		profile = ProfileFull // Default to full
	}

	// Add tools from profile's allowed groups
	for _, group := range ProfileAllowlists[profile] {
		for _, toolName := range ToolGroups[group] {
			allowedSet[toolName] = true
		}
	}

	// Apply overrides
	for toolName, allowed := range agent.ToolOverrides {
		if allowed {
			allowedSet[toolName] = true
		} else {
			delete(allowedSet, toolName)
		}
	}

	// Filter to only available tools
	var result []string
	for _, toolName := range available {
		if allowedSet[toolName] {
			result = append(result, toolName)
		}
	}

	return result
}

// GetProfileGroups returns the tool groups allowed by a profile.
func GetProfileGroups(profile ToolProfile) []string {
	return ProfileAllowlists[profile]
}

// IsToolInProfile checks if a tool is allowed by a profile.
func IsToolInProfile(profile ToolProfile, toolName string) bool {
	for _, group := range ProfileAllowlists[profile] {
		if slices.Contains(ToolGroups[group], toolName) {
			return true
		}
	}
	return false
}

// CreatePolicyFromProfile creates a tool policy from an agent's profile and overrides.
func CreatePolicyFromProfile(agent *AgentDefinition, registry *tools.Registry) *tools.Policy {
	if agent == nil {
		return tools.AllowAllPolicy()
	}

	policy := tools.NewPolicy()

	profile := agent.ToolProfile
	if profile == "" {
		profile = ProfileFull
	}

	// Allow tools from profile groups
	for _, group := range ProfileAllowlists[profile] {
		policy.AllowGroup(registry, group)
	}

	// Apply alsoAllow (additive, supports wildcards)
	for _, toolName := range agent.ToolAlsoAllow {
		if strings.Contains(toolName, "*") {
			policy.AllowPattern(toolName)
		} else {
			policy.Allow(toolName)
		}
	}

	// Apply overrides (can override alsoAllow)
	for toolName, allowed := range agent.ToolOverrides {
		if allowed {
			policy.Allow(toolName)
		} else {
			policy.Deny(toolName)
		}
	}

	return policy
}

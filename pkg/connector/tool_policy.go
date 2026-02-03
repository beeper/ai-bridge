package connector

import (
	"context"
	"sort"
	"strings"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/agents/toolpolicy"
	agenttools "github.com/beeper/ai-bridge/pkg/agents/tools"
	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

func (oc *AIClient) resolveToolPolicyModelContext(meta *PortalMetadata) (provider string, modelID string) {
	modelID = oc.effectiveModel(meta)
	backend, actual := ParseModelPrefix(modelID)
	if backend != "" {
		modelID = actual
	}
	provider = ""
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		provider = parts[0]
	}
	if provider == "" {
		loginMeta := loginMetadata(oc.UserLogin)
		if loginMeta != nil {
			provider = loginMeta.Provider
		}
	}
	return provider, modelID
}

func (oc *AIClient) resolveToolPolicies(meta *PortalMetadata) (*agents.AgentDefinition, []*toolpolicy.ToolPolicy) {
	var agent *agents.AgentDefinition
	if hasAssignedAgent(meta) {
		store := NewAgentStoreAdapter(oc)
		agent, _ = store.GetAgentForRoom(context.Background(), meta)
	}
	globalTools := oc.connector.Config.ToolPolicy
	provider, modelID := oc.resolveToolPolicyModelContext(meta)

	effective := toolpolicy.ResolveEffectiveToolPolicy(struct {
		Global        *toolpolicy.GlobalToolPolicyConfig
		Agent         *toolpolicy.ToolPolicyConfig
		ModelProvider string
		ModelID       string
	}{
		Global:        globalTools,
		Agent:         func() *toolpolicy.ToolPolicyConfig { if agent != nil { return agent.Tools }; return nil }(),
		ModelProvider: provider,
		ModelID:       modelID,
	})

	profilePolicy := toolpolicy.ResolveToolProfilePolicy(effective.Profile)
	providerProfilePolicy := toolpolicy.ResolveToolProfilePolicy(effective.ProviderProfile)
	profilePolicy = toolpolicy.MergeAlsoAllow(profilePolicy, effective.ProfileAlsoAllow)
	providerProfilePolicy = toolpolicy.MergeAlsoAllow(providerProfilePolicy, effective.ProviderAlsoAllow)

	policies := []*toolpolicy.ToolPolicy{
		effective.GlobalPolicy,
		effective.GlobalProviderPolicy,
		effective.AgentPolicy,
		effective.AgentProviderPolicy,
		profilePolicy,
		providerProfilePolicy,
	}

	return agent, policies
}

func (oc *AIClient) isToolAllowedByPolicy(meta *PortalMetadata, toolName string) bool {
	_, policies := oc.resolveToolPolicies(meta)
	return toolpolicy.IsToolAllowedByPolicies(toolName, policies)
}

func (oc *AIClient) isToolAvailable(meta *PortalMetadata, toolName string) (bool, SettingSource, string) {
	if meta == nil {
		return false, SourceGlobalDefault, "Missing room metadata"
	}

	if !meta.Capabilities.SupportsToolCalling {
		return false, SourceModelLimit, "Model does not support tools"
	}

	switch toolName {
	case ToolNameAnalyzeImage:
		toolName = ToolNameImage
	}

	if toolName == ToolNameMessage && !hasAssignedAgent(meta) {
		return false, SourceGlobalDefault, "Requires an assigned agent"
	}
	if agenttools.IsSessionTool(toolName) && !hasAssignedAgent(meta) {
		return false, SourceGlobalDefault, "Requires an assigned agent"
	}
	if agenttools.IsBossTool(toolName) && !meta.IsBuilderRoom {
		return false, SourceGlobalDefault, "Builder room only"
	}
	if (toolName == toolspec.GravatarFetchName || toolName == toolspec.GravatarSetName) && !hasBossAgent(meta) {
		return false, SourceGlobalDefault, "Boss agent only"
	}

	if toolName == ToolNameImageGenerate && !oc.canUseImageGeneration() {
		return false, SourceProviderLimit, "Image generation not available for this provider"
	}
	if toolName == ToolNameImage {
		if model, _ := oc.resolveVisionModelForImage(context.Background(), meta); model == "" {
			return false, SourceModelLimit, "No vision-capable model available"
		}
	}

	return true, SourceGlobalDefault, ""
}

// isToolEnabled checks if a specific tool is enabled (policy + availability).
func (oc *AIClient) isToolEnabled(meta *PortalMetadata, toolName string) bool {
	switch toolName {
	case ToolNameAnalyzeImage:
		toolName = ToolNameImage
	}

	available, _, _ := oc.isToolAvailable(meta, toolName)
	if !available {
		return false
	}

	return oc.isToolAllowedByPolicy(meta, toolName)
}

func (oc *AIClient) toolNamesForPortal(meta *PortalMetadata) []string {
	nameSet := make(map[string]struct{})
	for _, tool := range BuiltinTools() {
		nameSet[tool.Name] = struct{}{}
	}
	for _, tool := range agenttools.SessionTools() {
		nameSet[tool.Name] = struct{}{}
	}
	if meta != nil && meta.IsBuilderRoom {
		for _, tool := range agenttools.BossTools() {
			nameSet[tool.Name] = struct{}{}
		}
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

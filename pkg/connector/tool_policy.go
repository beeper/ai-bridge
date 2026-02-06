package connector

import (
	"context"
	"sort"
	"strings"

	"github.com/beeper/ai-bridge/pkg/agents/toolpolicy"
	agenttools "github.com/beeper/ai-bridge/pkg/agents/tools"
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

func (oc *AIClient) isToolAllowedByPolicy(meta *PortalMetadata, toolName string) bool {
	resolution := oc.resolveToolPolicies(meta)
	normalized := toolpolicy.NormalizeToolName(toolName)
	if normalized == "" {
		return false
	}
	_, ok := resolution.allowed[normalized]
	return ok
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

	if agenttools.IsBossTool(toolName) && !(meta.IsBuilderRoom || hasBossAgent(meta)) {
		return false, SourceGlobalDefault, "Builder room only"
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
	toolName = normalizeToolAlias(toolName)
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
	for _, tool := range agenttools.ProviderTools() {
		nameSet[tool.Name] = struct{}{}
	}
	if meta != nil && (meta.IsBuilderRoom || hasBossAgent(meta)) {
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

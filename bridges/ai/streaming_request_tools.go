package ai

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/beeper/agentremote/pkg/agents"
	"github.com/beeper/agentremote/pkg/agents/tools"
)

func (oc *AIClient) filterEnabledTools(meta *PortalMetadata, allTools []*tools.Tool) []*tools.Tool {
	var enabled []*tools.Tool
	for _, tool := range allTools {
		if oc.isToolEnabled(meta, tool.Name) {
			enabled = append(enabled, tool)
		}
	}
	return enabled
}

func (oc *AIClient) selectedStreamingToolDescriptors(
	ctx context.Context,
	meta *PortalMetadata,
	allowResolvedBossAgent bool,
) []openAIToolDescriptor {
	if meta != nil && !oc.getModelCapabilitiesForMeta(meta).SupportsToolCalling {
		return nil
	}

	var descriptors []openAIToolDescriptor
	builtinTools := oc.selectedBuiltinToolsForTurn(ctx, meta)
	if len(builtinTools) > 0 {
		descriptors = append(descriptors, toolDescriptorsFromDefinitions(builtinTools, &oc.log)...)
	}

	if meta == nil {
		return descriptors
	}

	agentID := resolveAgentID(meta)
	isBossRoom := hasBossAgent(meta) || (allowResolvedBossAgent && agents.IsBossAgent(agentID))
	if isBossRoom {
		descriptors = append(descriptors, toolDescriptorsFromBossTools(oc.filterEnabledTools(meta, tools.BossTools()), &oc.log)...)
		return descriptors
	}

	if agentID == "" {
		return descriptors
	}

	descriptors = append(descriptors, toolDescriptorsFromBossTools(oc.filterEnabledTools(meta, tools.SessionTools()), &oc.log)...)
	return descriptors
}

func (oc *AIClient) selectedResponsesStreamingTools(
	ctx context.Context,
	meta *PortalMetadata,
	allowResolvedBossAgent bool,
) []responses.ToolUnionParam {
	descriptors := oc.selectedStreamingToolDescriptors(ctx, meta, allowResolvedBossAgent)
	if len(descriptors) == 0 {
		return nil
	}
	return dedupeToolParams(descriptorsToResponsesTools(descriptors, resolveToolStrictMode(oc.isOpenRouterProvider())))
}

func (oc *AIClient) selectedChatStreamingTools(
	ctx context.Context,
	meta *PortalMetadata,
) []openai.ChatCompletionToolUnionParam {
	descriptors := oc.selectedStreamingToolDescriptors(ctx, meta, false)
	if len(descriptors) == 0 {
		return nil
	}
	return dedupeChatToolParams(descriptorsToChatTools(descriptors, resolveToolStrictMode(oc.isOpenRouterProvider())))
}

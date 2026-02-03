package connector

import (
	"strings"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

func (oc *AIClient) toolDescriptionForPortal(meta *PortalMetadata, toolName string, fallback string) string {
	name := strings.TrimSpace(toolName)
	switch name {
	case toolspec.ImageName:
		if meta != nil && meta.Capabilities.SupportsVision {
			return toolspec.ImageDescriptionVisionHint
		}
	case toolspec.WebSearchName:
		return oc.resolveWebSearchDescription(fallback)
	}
	return fallback
}

func (oc *AIClient) resolveWebSearchDescription(fallback string) string {
	provider := ""
	if oc != nil && oc.connector != nil && oc.connector.Config.Tools.Search != nil {
		provider = strings.TrimSpace(oc.connector.Config.Tools.Search.Provider)
	}
	provider = strings.ToLower(provider)
	if provider == "perplexity" || provider == "openrouter" {
		return "Search the web using Perplexity Sonar (direct or via OpenRouter). Returns AI-synthesized answers with citations from real-time web search."
	}
	if fallback != "" {
		return fallback
	}
	return toolspec.WebSearchDescription
}

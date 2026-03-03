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
		if fallback == "" {
			return toolspec.WebSearchDescription
		}
	}
	return fallback
}

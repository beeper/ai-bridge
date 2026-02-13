package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/simpleagent"
)

// buildBootstrapContextFiles intentionally returns no extra bootstrap files in simple mode.
func (oc *AIClient) buildBootstrapContextFiles(context.Context, string, *PortalMetadata) []agents.EmbeddedContextFile {
	return nil
}

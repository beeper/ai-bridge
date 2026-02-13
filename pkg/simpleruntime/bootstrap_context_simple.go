package connector

import (
	"context"
)

// buildBootstrapContextFiles intentionally returns no extra bootstrap files in simple mode.
func (oc *AIClient) buildBootstrapContextFiles(context.Context, string, *PortalMetadata) []AgentEmbeddedContextFile {
	return nil
}

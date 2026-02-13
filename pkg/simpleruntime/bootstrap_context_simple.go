package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/simpledeps/agents"
	"github.com/beeper/ai-bridge/pkg/textfs"
)

func (oc *AIClient) buildBootstrapContextFiles(context.Context, string, *PortalMetadata) []agents.EmbeddedContextFile {
	return nil
}

func userMdHasValues(content string) bool { return content != "" }

func (oc *AIClient) maybeAutoDeleteBootstrap(context.Context, *textfs.Store) {}

func (oc *AIClient) applySoulEvilToContextFiles(_ context.Context, files []agents.EmbeddedContextFile, _ string) []agents.EmbeddedContextFile {
	return files
}

func findSoulFileIndex(_ []agents.EmbeddedContextFile) int { return -1 }

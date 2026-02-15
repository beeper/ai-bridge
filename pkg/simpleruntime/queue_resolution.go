package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/ai-bridge/pkg/core/aiqueue"
)

func (oc *AIClient) resolveQueueSettingsForPortal(
	_ context.Context,
	_ *bridgev2.Portal,
	_ *PortalMetadata,
	inlineMode aiqueue.QueueMode,
	inlineOpts aiqueue.QueueInlineOptions,
) aiqueue.QueueSettings {
	var cfg *Config
	if oc != nil && oc.connector != nil {
		cfg = &oc.connector.Config
	}
	return resolveQueueSettings(queueResolveParams{
		cfg:        cfg,
		channel:    "matrix",
		inlineMode: inlineMode,
		inlineOpts: inlineOpts,
	})
}

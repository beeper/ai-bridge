package ai

import (
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/event"
)

func requireClientMeta(ce *commands.Event) (*AIClient, *PortalMetadata, bool) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		markCommandFailure(ce, "Couldn't load AI settings. Try again.", event.MessageStatusGenericError)
		ce.Reply("Couldn't load AI settings. Try again.")
		return nil, nil, false
	}
	return client, meta, true
}

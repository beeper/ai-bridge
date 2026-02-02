package connector

import (
	"context"

	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/event"
)

func getPinnedEventIDs(ctx context.Context, btc *BridgeToolContext) []string {
	var pinnedEvents []string
	matrixConn, ok := btc.Client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if ok {
		stateEvent, err := matrixConn.GetStateEvent(ctx, btc.Portal.MXID, event.StatePinnedEvents, "")
		if err == nil && stateEvent != nil {
			if content, ok := stateEvent.Content.Parsed.(*event.PinnedEventsEventContent); ok {
				for _, evtID := range content.Pinned {
					pinnedEvents = append(pinnedEvents, evtID.String())
				}
			}
		}
	}
	return pinnedEvents
}

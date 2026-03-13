package helpers

import (
	"context"

	"github.com/beeper/agentremote/sdk"
)

// BroadcastCommandDescriptions sends MSC4391 command-description state events
// for all SDK commands into the given room.
func BroadcastCommandDescriptions(ctx context.Context, conv *sdk.Conversation, commands []sdk.Command) error {
	portal := conv.Portal()
	if portal == nil || portal.MXID == "" {
		return nil
	}
	login := conv.Login()
	if login == nil || login.Bridge == nil || login.Bridge.Bot == nil {
		return nil
	}
	bot := login.Bridge.Bot
	sdk.BroadcastCommandDescriptions(ctx, portal, bot, commands)
	return nil
}

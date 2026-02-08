package connector

import (
	"context"
	"errors"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"github.com/beeper/ai-bridge/pkg/agents"
)

func requireClientMeta(ce *commands.Event) (*AIClient, *PortalMetadata, bool) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if meta != nil && meta.IsCodexRoom {
		ce.Reply("This command isn't supported in Codex rooms. Try `!ai status`, `!ai reset`, or `!ai approve`.")
		return nil, nil, false
	}
	if client == nil || meta == nil {
		ce.Reply("Couldn't load AI settings. Try again.")
		return nil, nil, false
	}
	return client, meta, true
}

func getCodexClient(ce *commands.Event) *CodexClient {
	if ce == nil || ce.User == nil {
		return nil
	}

	defaultLogin := ce.User.GetDefaultLogin()
	br := ce.Bridge
	if ce.User.Bridge != nil {
		br = ce.User.Bridge
	}

	login := resolveLoginForCommand(ce.Ctx, ce.Portal, defaultLogin, func(ctx context.Context, id networkid.UserLoginID) (*bridgev2.UserLogin, error) {
		if br == nil {
			return nil, errors.New("missing bridge")
		}
		return br.GetExistingUserLoginByID(ctx, id)
	})
	if login == nil {
		return nil
	}
	client, ok := login.Client.(*CodexClient)
	if !ok {
		return nil
	}
	return client
}

func requireClient(ce *commands.Event) (*AIClient, bool) {
	client := getAIClient(ce)
	if client == nil {
		ce.Reply("Couldn't load AI settings. Try again.")
		return nil, false
	}
	return client, true
}

func requirePortal(ce *commands.Event) (*bridgev2.Portal, bool) {
	if ce.Portal == nil {
		ce.Reply("This command can only be used in a room.")
		return nil, false
	}
	return ce.Portal, true
}

func rejectBossOverrides(ce *commands.Event, meta *PortalMetadata, message string) bool {
	if agents.IsBossAgent(resolveAgentID(meta)) {
		ce.Reply(message)
		return true
	}
	return false
}

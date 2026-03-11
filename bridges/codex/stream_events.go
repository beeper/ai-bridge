package codex

import (
	"fmt"
	"net/url"
	"strings"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

func defaultCodexChatPortalKey(loginID networkid.UserLoginID) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("codex:%s:default-chat", loginID)),
		Receiver: loginID,
	}
}

func codexThreadPortalKey(loginID networkid.UserLoginID, threadID string) networkid.PortalKey {
	return networkid.PortalKey{
		ID: networkid.PortalID(
			fmt.Sprintf(
				"codex:%s:thread:%s",
				loginID,
				url.PathEscape(strings.TrimSpace(threadID)),
			),
		),
		Receiver: loginID,
	}
}

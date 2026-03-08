package openclaw

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/shared/openclawconv"
)

func makeOpenClawUserLoginID(mxid id.UserID, ordinal int) networkid.UserLoginID {
	escaped := url.PathEscape(string(mxid))
	base := networkid.UserLoginID(fmt.Sprintf("openclaw:%s", escaped))
	if ordinal <= 1 {
		return base
	}
	return networkid.UserLoginID(fmt.Sprintf("%s:%d", base, ordinal))
}

func nextOpenClawUserLoginID(user *bridgev2.User) networkid.UserLoginID {
	used := make(map[string]struct{})
	for _, existing := range user.GetUserLogins() {
		if existing == nil {
			continue
		}
		used[string(existing.ID)] = struct{}{}
	}
	for ordinal := 1; ; ordinal++ {
		loginID := makeOpenClawUserLoginID(user.MXID, ordinal)
		if _, ok := used[string(loginID)]; !ok {
			return loginID
		}
	}
}

func openClawGatewayID(gatewayURL, label string) string {
	key := strings.ToLower(strings.TrimSpace(gatewayURL)) + "|" + strings.ToLower(strings.TrimSpace(label))
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func openClawPortalKey(loginID networkid.UserLoginID, gatewayID, sessionKey string) networkid.PortalKey {
	return networkid.PortalKey{
		ID: networkid.PortalID(
			"openclaw:" +
				string(loginID) + ":" +
				url.PathEscape(strings.TrimSpace(gatewayID)) + ":" +
				url.PathEscape(strings.TrimSpace(sessionKey)),
		),
		Receiver: loginID,
	}
}

func openClawGhostUserID(agentID string) networkid.UserID {
	trimmed := strings.TrimSpace(agentID)
	if trimmed == "" {
		trimmed = "gateway"
	}
	return networkid.UserID("openclaw-agent:" + url.PathEscape(trimmed))
}

func parseOpenClawGhostID(ghostID string) (string, bool) {
	suffix, ok := strings.CutPrefix(strings.TrimSpace(ghostID), "openclaw-agent:")
	if !ok {
		return "", false
	}
	value, err := url.PathUnescape(suffix)
	if err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

func openClawAgentIDFromSessionKey(sessionKey string) string {
	return openclawconv.AgentIDFromSessionKey(sessionKey)
}

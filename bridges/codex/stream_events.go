package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/beeper/ai-bridge/pkg/matrixevents"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var StreamEventMessageType = matrixevents.StreamEventMessageType
var RoomCapabilitiesEventType = matrixevents.RoomCapabilitiesEventType
var RoomSettingsEventType = matrixevents.RoomSettingsEventType

type matrixEphemeralSender interface {
	SendEphemeralEvent(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, txnID string) (*mautrix.RespSendEvent, error)
}

func buildStreamEventEnvelope(state *streamingState, part map[string]any) (turnID string, seq int, content map[string]any, ok bool) {
	turnID = strings.TrimSpace(state.turnID)
	if turnID == "" {
		return "", 0, nil, false
	}
	state.sequenceNum++
	seq = state.sequenceNum
	env, err := matrixevents.BuildStreamEventEnvelope(turnID, seq, part, matrixevents.StreamEventOpts{
		TargetEventID: state.initialEventID.String(),
		AgentID:       state.agentID,
	})
	if err != nil {
		return "", 0, nil, false
	}
	return turnID, seq, env, true
}

func buildStreamEventTxnID(turnID string, seq int) string {
	return matrixevents.BuildStreamEventTxnID(turnID, seq)
}

func defaultCodexChatPortalKey(loginID networkid.UserLoginID) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("codex:%s:default-chat", loginID)),
		Receiver: loginID,
	}
}


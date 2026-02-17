package bridgeadapter

import (
	"fmt"

	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func MatrixMessageID(eventID id.EventID) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("mx:%s", string(eventID)))
}

func HumanUserID(prefix string, loginID networkid.UserLoginID) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("%s:%s", prefix, loginID))
}

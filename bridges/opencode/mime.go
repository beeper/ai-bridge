package opencode

import (
	"maunium.net/go/mautrix/event"

	"github.com/beeper/agentremote/pkg/shared/media"
)

func messageTypeForMIME(mimeType string) event.MessageType {
	return media.MessageTypeForMIME(mimeType)
}

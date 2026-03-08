package connector

import (
	"strings"

	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/ai-bridge/pkg/connector/msgconv"
)

func ensureCanonicalUserMessage(msg *database.Message) {
	if msg == nil {
		return
	}
	meta, ok := msg.Metadata.(*MessageMetadata)
	if !ok || meta == nil || strings.TrimSpace(meta.Role) != "user" {
		return
	}
	if len(meta.CanonicalUIMessage) > 0 && meta.CanonicalSchema == "ai-sdk-ui-message-v1" {
		return
	}
	messageID := ""
	if msg.MXID != "" {
		messageID = msg.MXID.String()
	} else if msg.ID != "" {
		messageID = string(msg.ID)
	}
	meta.CanonicalSchema = "ai-sdk-ui-message-v1"
	meta.CanonicalUIMessage = msgconv.BuildUserUIMessage(msgconv.UserUIMessageParams{
		MessageID: messageID,
		Text:      meta.Body,
		MediaURL:  meta.MediaURL,
		MimeType:  meta.MimeType,
	})
}

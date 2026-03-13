// Package helpers provides shared utility functions for SDK bridges.
package helpers

import (
	"context"
	"errors"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// UploadMedia uploads media data to Matrix and returns the content URI.
func UploadMedia(ctx context.Context, data []byte, mediaType, filename string, portal *bridgev2.Portal, login *bridgev2.UserLogin) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	if login == nil || login.Bridge == nil || login.Bridge.Bot == nil {
		return "", nil, errors.New("bridge is unavailable")
	}
	if portal == nil {
		return "", nil, errors.New("missing portal")
	}
	return login.Bridge.Bot.UploadMedia(ctx, portal.MXID, data, filename, mediaType)
}

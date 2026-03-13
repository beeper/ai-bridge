// Package helpers provides shared utility functions for SDK bridges.
package helpers

import (
	"context"
	"errors"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote"
	sharedmedia "github.com/beeper/agentremote/pkg/shared/media"
)

// DownloadMedia downloads media from a Matrix content URI and returns the raw bytes and MIME type.
func DownloadMedia(ctx context.Context, url string, login *bridgev2.UserLogin) ([]byte, string, error) {
	return agentremote.DownloadMediaBytes(ctx, login, url, nil, 0)
}

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

// DecodeBase64Media decodes a base64-encoded media string.
func DecodeBase64Media(data string) ([]byte, string, error) {
	decoded, _, err := sharedmedia.DecodeBase64(data)
	if err != nil {
		return nil, "", err
	}
	return decoded, "application/octet-stream", nil
}

// ParseDataURI parses a data: URI into raw bytes and MIME type.
// Format: data:[<mediatype>][;base64],<data>
func ParseDataURI(uri string) ([]byte, string, error) {
	data, mediaType, err := sharedmedia.DecodeDataURI(uri)
	if err != nil {
		return nil, "", err
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return data, mediaType, nil
}

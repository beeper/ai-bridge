// Package helpers provides shared utility functions for SDK bridges.
package helpers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// DownloadMedia downloads media from a Matrix content URI and returns the raw bytes and MIME type.
func DownloadMedia(ctx context.Context, url string, login *bridgev2.UserLogin) ([]byte, string, error) {
	if strings.TrimSpace(url) == "" {
		return nil, "", errors.New("missing media URL")
	}
	if login == nil || login.Bridge == nil || login.Bridge.Bot == nil {
		return nil, "", errors.New("bridge is unavailable")
	}
	var data []byte
	err := login.Bridge.Bot.DownloadMediaToFile(ctx, id.ContentURIString(url), nil, false, func(f *os.File) error {
		var err error
		data, err = io.ReadAll(f)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	mimeType := http.DetectContentType(data)
	return data, mimeType, nil
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
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 data: %w", err)
	}
	return decoded, "application/octet-stream", nil
}

// ParseDataURI parses a data: URI into raw bytes and MIME type.
// Format: data:[<mediatype>][;base64],<data>
func ParseDataURI(uri string) ([]byte, string, error) {
	if !strings.HasPrefix(uri, "data:") {
		return nil, "", errors.New("not a data URI")
	}
	rest := uri[5:] // strip "data:"
	commaIdx := strings.IndexByte(rest, ',')
	if commaIdx < 0 {
		return nil, "", errors.New("invalid data URI: missing comma")
	}
	meta := rest[:commaIdx]
	encoded := rest[commaIdx+1:]

	mediaType := "application/octet-stream"
	isBase64 := false
	parts := strings.Split(meta, ";")
	for i, part := range parts {
		if i == 0 && part != "" {
			mediaType = part
		}
		if part == "base64" {
			isBase64 = true
		}
	}

	if !isBase64 {
		return nil, "", errors.New("only base64 data URIs are supported")
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 in data URI: %w", err)
	}
	return data, mediaType, nil
}

package connector

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix/event"
)

func (oc *AIClient) downloadMediaBase64(
	ctx context.Context,
	mediaURL string,
	encryptedFile *event.EncryptedFileInfo,
	maxSizeMB int,
	fallbackMime string,
) (string, string, error) {
	b64Data, actualMimeType, err := oc.downloadAndEncodeMedia(ctx, mediaURL, encryptedFile, maxSizeMB)
	if err != nil {
		return "", "", err
	}
	if actualMimeType == "" || actualMimeType == "application/octet-stream" {
		actualMimeType = fallbackMime
	}
	return b64Data, actualMimeType, nil
}

func buildDataURL(mimeType, b64Data string) string {
	return fmt.Sprintf("data:%s;base64,%s", mimeType, b64Data)
}

package connector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func (oc *AIClient) downloadMediaBytes(
	ctx context.Context,
	mediaURL string,
	encryptedFile *event.EncryptedFileInfo,
	maxBytes int,
	fallbackMime string,
) ([]byte, string, error) {
	downloadURL := mediaURL
	if encryptedFile != nil {
		downloadURL = string(encryptedFile.URL)
	}

	if strings.HasPrefix(downloadURL, "file://") || strings.HasPrefix(downloadURL, "/") {
		path := downloadURL
		if strings.HasPrefix(path, "file://") {
			path = strings.TrimPrefix(path, "file://")
			if unescaped, err := url.PathUnescape(path); err == nil {
				path = unescaped
			}
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, "", fmt.Errorf("failed to stat local file: %w", err)
		}
		if maxBytes > 0 && info.Size() > int64(maxBytes) {
			return nil, "", fmt.Errorf("media too large: %d bytes (max %d)", info.Size(), maxBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read local file: %w", err)
		}
		if encryptedFile != nil {
			if err := encryptedFile.DecryptInPlace(data); err != nil {
				return nil, "", fmt.Errorf("failed to decrypt media: %w", err)
			}
		}
		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		return data, normalizeFallbackMime(mimeType, fallbackMime), nil
	}

	if strings.HasPrefix(downloadURL, "mxc://") {
		if oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.Bot == nil {
			return nil, "", errors.New("matrix API unavailable for MXC media download")
		}
		data, err := oc.UserLogin.Bridge.Bot.DownloadMedia(ctx, id.ContentURIString(downloadURL), encryptedFile)
		if err != nil {
			return nil, "", fmt.Errorf("failed to download media via Matrix API: %w", err)
		}
		if maxBytes > 0 && len(data) > maxBytes {
			return nil, "", fmt.Errorf("media too large (max %d bytes)", maxBytes)
		}
		return data, normalizeFallbackMime(http.DetectContentType(data), fallbackMime), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create download request: %w", err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("media download failed: HTTP %d", resp.StatusCode)
	}
	if maxBytes > 0 && resp.ContentLength > 0 && resp.ContentLength > int64(maxBytes) {
		return nil, "", fmt.Errorf("media too large: %d bytes (max %d)", resp.ContentLength, maxBytes)
	}

	var reader io.Reader = resp.Body
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, int64(maxBytes)+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read media: %w", err)
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, "", fmt.Errorf("media too large (max %d bytes)", maxBytes)
	}
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return data, normalizeFallbackMime(mimeType, fallbackMime), nil
}

func normalizeFallbackMime(actual string, fallback string) string {
	actual = strings.TrimSpace(actual)
	if actual == "" || actual == "application/octet-stream" {
		actual = strings.TrimSpace(fallback)
	}
	if actual == "" {
		actual = "application/octet-stream"
	}
	return actual
}

// downloadMediaBase64 downloads media and returns base64-encoded data with MIME type.
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
	return "data:" + mimeType + ";base64," + b64Data
}

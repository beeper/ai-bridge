package connector

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// decodeBase64Image decodes a base64-encoded image and detects its MIME type.
// Handles both raw base64 and data URL format (data:image/png;base64,...).
func decodeBase64Image(b64Data string) ([]byte, string, error) {
	var mimeType string

	// Handle data URL format: data:{mimeType};base64,{data}
	if after, found := strings.CutPrefix(b64Data, "data:"); found {
		prefix, data, hasComma := strings.Cut(after, ",")
		if !hasComma {
			return nil, "", fmt.Errorf("invalid data URL: no comma found")
		}
		// Extract MIME type from "{mimeType};base64" prefix
		if mime, _, hasBase64 := strings.Cut(prefix, ";base64"); hasBase64 {
			mimeType = mime
		}
		b64Data = data
	}

	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		// Try URL-safe base64 as fallback
		data, err = base64.URLEncoding.DecodeString(b64Data)
		if err != nil {
			return nil, "", fmt.Errorf("base64 decode failed: %w", err)
		}
	}

	// If MIME type wasn't extracted from data URL, detect from bytes
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
		// Fallback to PNG if detection fails (common for AI-generated images)
		if mimeType == "application/octet-stream" {
			mimeType = "image/png"
		}
	}

	return data, mimeType, nil
}

// sendGeneratedImage uploads an AI-generated image to Matrix and sends it as a message
func (oc *AIClient) sendGeneratedImage(
	ctx context.Context,
	portal *bridgev2.Portal,
	imageData []byte,
	mimeType string,
	turnID string,
) (id.EventID, error) {
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	// Generate filename based on timestamp and mime type
	ext := "png"
	switch mimeType {
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	case "image/gif":
		ext = "gif"
	}
	fileName := fmt.Sprintf("generated-%d.%s", time.Now().UnixMilli(), ext)

	// Upload to Matrix
	uri, file, err := intent.UploadMedia(ctx, portal.MXID, imageData, fileName, mimeType)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	// Build image message content with AI metadata
	rawContent := map[string]any{
		"msgtype": event.MsgImage,
		"body":    fileName,
		"info": map[string]any{
			"mimetype": mimeType,
			"size":     len(imageData),
		},
	}

	if file != nil {
		rawContent["file"] = file
	} else {
		rawContent["url"] = string(uri)
	}

	// Add image generation metadata
	if turnID != "" {
		rawContent["com.beeper.ai.image_generation"] = map[string]any{
			"turn_id": turnID,
		}
	}

	// Send message
	eventContent := &event.Content{Raw: rawContent}
	resp, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		return "", fmt.Errorf("send failed: %w", err)
	}
	return resp.EventID, nil
}

package connector

import (
	"context"
	"fmt"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// sendGeneratedAudio uploads TTS-generated audio to Matrix and sends it as a voice message.
func (oc *AIClient) sendGeneratedAudio(
	ctx context.Context,
	portal *bridgev2.Portal,
	audioData []byte,
	mimeType string,
	turnID string,
) (id.EventID, error) {
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	// Determine file extension based on MIME type
	ext := "mp3"
	switch mimeType {
	case "audio/wav", "audio/x-wav":
		ext = "wav"
	case "audio/ogg":
		ext = "ogg"
	case "audio/opus":
		ext = "opus"
	case "audio/flac":
		ext = "flac"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		ext = "m4a"
	}
	fileName := fmt.Sprintf("tts-%d.%s", time.Now().UnixMilli(), ext)

	// Upload to Matrix
	uri, file, err := intent.UploadMedia(ctx, portal.MXID, audioData, fileName, mimeType)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	// Build audio message content
	rawContent := map[string]any{
		"msgtype": event.MsgAudio,
		"body":    fileName,
		"info": map[string]any{
			"mimetype": mimeType,
			"size":     len(audioData),
		},
	}

	if file != nil {
		rawContent["file"] = file
	} else {
		rawContent["url"] = string(uri)
	}

	// Add TTS metadata
	if turnID != "" {
		rawContent["com.beeper.ai.tts"] = map[string]any{
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

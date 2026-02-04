package connector

// MediaUnderstandingCapability identifies the type of media being understood.
type MediaUnderstandingCapability string

const (
	MediaCapabilityImage MediaUnderstandingCapability = "image"
	MediaCapabilityAudio MediaUnderstandingCapability = "audio"
	MediaCapabilityVideo MediaUnderstandingCapability = "video"
)

// MediaUnderstandingKind identifies the output kind.
type MediaUnderstandingKind string

const (
	MediaKindAudioTranscription MediaUnderstandingKind = "audio.transcription"
	MediaKindImageDescription   MediaUnderstandingKind = "image.description"
	MediaKindVideoDescription   MediaUnderstandingKind = "video.description"
)

// MediaUnderstandingOutput represents a single media understanding result.
type MediaUnderstandingOutput struct {
	Kind            MediaUnderstandingKind `json:"kind"`
	AttachmentIndex int                    `json:"attachment_index"`
	Text            string                 `json:"text"`
	Provider        string                 `json:"provider"`
	Model           string                 `json:"model,omitempty"`
}

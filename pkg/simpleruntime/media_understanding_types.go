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

// MediaUnderstandingModelDecision records a single model attempt.
type MediaUnderstandingModelDecision struct {
	Type     string `json:"type,omitempty"` // provider | cli
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Outcome  string `json:"outcome,omitempty"` // success | skipped | failed
	Reason   string `json:"reason,omitempty"`
}

// MediaUnderstandingAttachmentDecision records attempts for one attachment.
type MediaUnderstandingAttachmentDecision struct {
	AttachmentIndex int                               `json:"attachment_index"`
	Attempts        []MediaUnderstandingModelDecision `json:"attempts,omitempty"`
	Chosen          *MediaUnderstandingModelDecision  `json:"chosen,omitempty"`
}

// MediaUnderstandingDecision summarizes the overall outcome for a capability.
type MediaUnderstandingDecision struct {
	Capability  MediaUnderstandingCapability           `json:"capability"`
	Outcome     string                                 `json:"outcome,omitempty"` // success | skipped | disabled | scope-deny | no-attachment
	Attachments []MediaUnderstandingAttachmentDecision `json:"attachments,omitempty"`
}

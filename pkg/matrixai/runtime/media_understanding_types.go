package runtime

type MediaUnderstandingCapability string
type MediaUnderstandingKind string

type MediaUnderstandingOutput struct {
	Kind            MediaUnderstandingKind `json:"kind"`
	AttachmentIndex int                    `json:"attachment_index"`
	Text            string                 `json:"text"`
	Provider        string                 `json:"provider"`
	Model           string                 `json:"model,omitempty"`
}

type MediaUnderstandingModelDecision struct {
	Type     string `json:"type,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type MediaUnderstandingAttachmentDecision struct {
	AttachmentIndex int                               `json:"attachment_index"`
	Attempts        []MediaUnderstandingModelDecision `json:"attempts,omitempty"`
	Chosen          *MediaUnderstandingModelDecision  `json:"chosen,omitempty"`
}

type MediaUnderstandingDecision struct {
	Capability  MediaUnderstandingCapability           `json:"capability"`
	Outcome     string                                 `json:"outcome,omitempty"`
	Attachments []MediaUnderstandingAttachmentDecision `json:"attachments,omitempty"`
}

const (
	MediaCapabilityImage MediaUnderstandingCapability = "image"
	MediaCapabilityAudio MediaUnderstandingCapability = "audio"
	MediaCapabilityVideo MediaUnderstandingCapability = "video"
)

const (
	MediaKindAudioTranscription MediaUnderstandingKind = "audio.transcription"
	MediaKindImageDescription   MediaUnderstandingKind = "image.description"
	MediaKindVideoDescription   MediaUnderstandingKind = "video.description"
)

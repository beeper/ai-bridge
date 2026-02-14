package connector

import "github.com/beeper/ai-bridge/pkg/core/aimedia"

type (
	MediaUnderstandingCapability       = aimedia.MediaUnderstandingCapability
	MediaUnderstandingKind             = aimedia.MediaUnderstandingKind
	MediaUnderstandingOutput           = aimedia.MediaUnderstandingOutput
	MediaUnderstandingModelDecision    = aimedia.MediaUnderstandingModelDecision
	MediaUnderstandingAttachmentDecision = aimedia.MediaUnderstandingAttachmentDecision
	MediaUnderstandingDecision         = aimedia.MediaUnderstandingDecision
)

const (
	MediaCapabilityImage = aimedia.MediaCapabilityImage
	MediaCapabilityAudio = aimedia.MediaCapabilityAudio
	MediaCapabilityVideo = aimedia.MediaCapabilityVideo
)

const (
	MediaKindAudioTranscription = aimedia.MediaKindAudioTranscription
	MediaKindImageDescription   = aimedia.MediaKindImageDescription
	MediaKindVideoDescription   = aimedia.MediaKindVideoDescription
)

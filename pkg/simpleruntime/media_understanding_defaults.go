package connector

import "github.com/beeper/ai-bridge/pkg/core/aimedia"

const (
	mediaMB                        = aimedia.MediaMB
	defaultMediaMaxChars           = aimedia.DefaultMediaMaxChars
	defaultMediaConcurrency        = aimedia.DefaultMediaConcurrency
	defaultVideoMaxBase64Bytes     = aimedia.DefaultVideoMaxBase64Bytes
	defaultImageUnderstandingLimit = aimedia.DefaultImageUnderstandingLimit
)

var defaultMaxCharsByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: aimedia.DefaultMaxCharsByCapability[aimedia.MediaCapabilityImage],
	MediaCapabilityAudio: aimedia.DefaultMaxCharsByCapability[aimedia.MediaCapabilityAudio],
	MediaCapabilityVideo: aimedia.DefaultMaxCharsByCapability[aimedia.MediaCapabilityVideo],
}
var defaultMaxBytesByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: aimedia.DefaultMaxBytesByCapability[aimedia.MediaCapabilityImage],
	MediaCapabilityAudio: aimedia.DefaultMaxBytesByCapability[aimedia.MediaCapabilityAudio],
	MediaCapabilityVideo: aimedia.DefaultMaxBytesByCapability[aimedia.MediaCapabilityVideo],
}
var defaultTimeoutSecondsByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: aimedia.DefaultTimeoutSecondsByCapability[aimedia.MediaCapabilityImage],
	MediaCapabilityAudio: aimedia.DefaultTimeoutSecondsByCapability[aimedia.MediaCapabilityAudio],
	MediaCapabilityVideo: aimedia.DefaultTimeoutSecondsByCapability[aimedia.MediaCapabilityVideo],
}
var defaultPromptByCapability = map[MediaUnderstandingCapability]string{
	MediaCapabilityImage: aimedia.DefaultPromptByCapability[aimedia.MediaCapabilityImage],
	MediaCapabilityAudio: aimedia.DefaultPromptByCapability[aimedia.MediaCapabilityAudio],
	MediaCapabilityVideo: aimedia.DefaultPromptByCapability[aimedia.MediaCapabilityVideo],
}
var defaultAudioModelsByProvider = aimedia.DefaultAudioModelsByProvider

const defaultOpenRouterGoogleModel = aimedia.DefaultOpenRouterGoogleModel

var defaultImageModelsByProvider = aimedia.DefaultImageModelsByProvider

var resolveVideoMaxBase64Bytes = aimedia.ResolveVideoMaxBase64Bytes

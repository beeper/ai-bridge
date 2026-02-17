package runtime

import "github.com/beeper/ai-bridge/pkg/core/aimedia"

const (
	mediaMB                        = aimedia.MediaMB
	defaultMediaMaxChars           = aimedia.DefaultMediaMaxChars
	defaultMediaConcurrency        = aimedia.DefaultMediaConcurrency
	defaultVideoMaxBase64Bytes     = aimedia.DefaultVideoMaxBase64Bytes
	defaultImageUnderstandingLimit = aimedia.DefaultImageUnderstandingLimit
)

var defaultTimeoutSecondsByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: aimedia.DefaultTimeoutSecondsByCapability[aimedia.MediaCapabilityImage],
	MediaCapabilityAudio: aimedia.DefaultTimeoutSecondsByCapability[aimedia.MediaCapabilityAudio],
	MediaCapabilityVideo: aimedia.DefaultTimeoutSecondsByCapability[aimedia.MediaCapabilityVideo],
}
var defaultAudioModelsByProvider = aimedia.DefaultAudioModelsByProvider

const defaultOpenRouterGoogleModel = aimedia.DefaultOpenRouterGoogleModel

var defaultImageModelsByProvider = aimedia.DefaultImageModelsByProvider

var resolveVideoMaxBase64Bytes = aimedia.ResolveVideoMaxBase64Bytes

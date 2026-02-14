package connector

import "github.com/beeper/ai-bridge/pkg/aimedia"

const (
	mediaMB                        = aimedia.MediaMB
	defaultMediaMaxChars           = aimedia.DefaultMediaMaxChars
	defaultMediaConcurrency        = aimedia.DefaultMediaConcurrency
	defaultVideoMaxBase64Bytes     = aimedia.DefaultVideoMaxBase64Bytes
	defaultImageUnderstandingLimit = aimedia.DefaultImageUnderstandingLimit
)

var defaultMaxCharsByCapability = aimedia.DefaultMaxCharsByCapability
var defaultMaxBytesByCapability = aimedia.DefaultMaxBytesByCapability
var defaultTimeoutSecondsByCapability = aimedia.DefaultTimeoutSecondsByCapability
var defaultPromptByCapability = aimedia.DefaultPromptByCapability
var defaultAudioModelsByProvider = aimedia.DefaultAudioModelsByProvider

const defaultOpenRouterGoogleModel = aimedia.DefaultOpenRouterGoogleModel

var defaultImageModelsByProvider = aimedia.DefaultImageModelsByProvider

var resolveVideoMaxBase64Bytes = aimedia.ResolveVideoMaxBase64Bytes

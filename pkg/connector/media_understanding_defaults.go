package connector

const (
	mediaMB                        = 1024 * 1024
	defaultMediaMaxChars           = 500
	defaultMediaConcurrency        = 2
	defaultVideoMaxBase64Bytes     = 70 * mediaMB
	defaultImageUnderstandingLimit = 1024
)

var defaultMaxCharsByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: defaultMediaMaxChars,
	MediaCapabilityAudio: 0,
	MediaCapabilityVideo: defaultMediaMaxChars,
}

var defaultMaxBytesByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: 10 * mediaMB,
	MediaCapabilityAudio: 20 * mediaMB,
	MediaCapabilityVideo: 50 * mediaMB,
}

var defaultTimeoutSecondsByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: 60,
	MediaCapabilityAudio: 60,
	MediaCapabilityVideo: 120,
}

var defaultPromptByCapability = map[MediaUnderstandingCapability]string{
	MediaCapabilityImage: "Describe the image.",
	MediaCapabilityAudio: "Transcribe the audio.",
	MediaCapabilityVideo: "Describe the video.",
}

var defaultAudioModelsByProvider = map[string]string{
	"groq":     "whisper-large-v3-turbo",
	"openai":   "gpt-4o-mini-transcribe",
	"deepgram": "nova-3",
}

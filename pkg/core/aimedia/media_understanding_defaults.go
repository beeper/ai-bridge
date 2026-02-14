package aimedia

const (
	MediaMB                        = 1024 * 1024
	DefaultMediaMaxChars           = 500
	DefaultMediaConcurrency        = 2
	DefaultVideoMaxBase64Bytes     = 70 * MediaMB
	DefaultImageUnderstandingLimit = 1024
)

var DefaultMaxCharsByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: DefaultMediaMaxChars,
	MediaCapabilityAudio: 0,
	MediaCapabilityVideo: DefaultMediaMaxChars,
}

var DefaultMaxBytesByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: 10 * MediaMB,
	MediaCapabilityAudio: 20 * MediaMB,
	MediaCapabilityVideo: 50 * MediaMB,
}

var DefaultTimeoutSecondsByCapability = map[MediaUnderstandingCapability]int{
	MediaCapabilityImage: 60,
	MediaCapabilityAudio: 60,
	MediaCapabilityVideo: 120,
}

var DefaultPromptByCapability = map[MediaUnderstandingCapability]string{
	MediaCapabilityImage: "Describe the image.",
	MediaCapabilityAudio: "Transcribe the audio.",
	MediaCapabilityVideo: "Describe the video.",
}

var DefaultAudioModelsByProvider = map[string]string{
	"groq":     "whisper-large-v3-turbo",
	"openai":   "gpt-4o-transcribe",
	"deepgram": "nova-3",
}

const DefaultOpenRouterGoogleModel = "google/gemini-3-flash-preview"

var DefaultImageModelsByProvider = map[string]string{
	"openai":     "gpt-5-mini",
	"openrouter": DefaultOpenRouterGoogleModel,
}

func ResolveVideoMaxBase64Bytes(maxBytes int) int {
	if maxBytes <= 0 {
		return DefaultVideoMaxBase64Bytes
	}
	expanded := int(float64(maxBytes) * (4.0 / 3.0))
	if expanded > DefaultVideoMaxBase64Bytes {
		return DefaultVideoMaxBase64Bytes
	}
	return expanded
}

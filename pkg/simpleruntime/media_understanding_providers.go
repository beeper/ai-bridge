package connector

import "github.com/beeper/ai-bridge/pkg/core/aimedia"

// Re-export provider transport types and functions from the library.
type (
	mediaAudioRequest = aimedia.AudioRequest
	mediaVideoRequest = aimedia.VideoRequest
	mediaImageRequest = aimedia.ImageRequest
)

var (
	mediaProviderCapabilities = aimedia.ProviderCapabilities
	normalizeMediaProviderID  = aimedia.NormalizeProviderID
	normalizeMediaBaseURL     = aimedia.NormalizeBaseURL
	readErrorResponse         = aimedia.ReadErrorResponse
	headerExists              = aimedia.HeaderExists
	applyHeaderMap            = aimedia.ApplyHeaderMap
	resolveProviderQuery      = aimedia.ResolveProviderQuery
	buildDeepgramQuery        = aimedia.BuildDeepgramQuery
	transcribeOpenAIAudio     = aimedia.TranscribeOpenAIAudio
	transcribeDeepgramAudio   = aimedia.TranscribeDeepgramAudio
	transcribeGeminiAudio     = aimedia.TranscribeGeminiAudio
	describeGeminiVideo       = aimedia.DescribeGeminiVideo
	describeGeminiImage       = aimedia.DescribeGeminiImage
	hasProviderAuthHeader     = aimedia.HasProviderAuthHeader
	resolveMediaFileName      = aimedia.ResolveMediaFileName
)

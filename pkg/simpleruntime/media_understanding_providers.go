package connector

import (
	"context"
	"net/http"
	"time"

	"github.com/beeper/ai-bridge/pkg/core/aimedia"
)

type mediaAudioRequest struct {
	Provider string
	APIKey   string
	BaseURL  string
	Headers  map[string]string
	Model    string
	Language string
	Prompt   string
	MimeType string
	FileName string
	Data     []byte
	Timeout  time.Duration
}

type mediaVideoRequest struct {
	APIKey   string
	BaseURL  string
	Headers  map[string]string
	Model    string
	Prompt   string
	MimeType string
	Data     []byte
	Timeout  time.Duration
}

type mediaImageRequest struct {
	APIKey   string
	BaseURL  string
	Headers  map[string]string
	Model    string
	Prompt   string
	MimeType string
	Data     []byte
	Timeout  time.Duration
}

func normalizeMediaProviderID(id string) string {
	return aimedia.NormalizeProviderID(id)
}

func normalizeMediaBaseURL(value string, fallback string) string {
	return aimedia.NormalizeBaseURL(value, fallback)
}

func readErrorResponse(res *http.Response) string {
	return aimedia.ReadErrorResponse(res)
}

func headerExists(headers http.Header, key string) bool {
	return aimedia.HeaderExists(headers, key)
}

func applyHeaderMap(headers http.Header, values map[string]string) {
	aimedia.ApplyHeaderMap(headers, values)
}

func resolveProviderQuery(providerID string, capCfg *MediaUnderstandingConfig, modelCfg MediaUnderstandingModelConfig) map[string]any {
	return aimedia.ResolveProviderQuery(providerID, toCoreMediaConfig(capCfg), toCoreMediaModelConfig(modelCfg))
}

func buildDeepgramQuery(cfg *MediaUnderstandingConfig, entry MediaUnderstandingModelConfig) map[string]any {
	return aimedia.BuildDeepgramQuery(toCoreMediaConfig(cfg), toCoreMediaModelConfig(entry))
}

func transcribeOpenAIAudio(ctx context.Context, request mediaAudioRequest) (string, error) {
	return aimedia.TranscribeOpenAIAudio(ctx, toCoreAudioRequest(request))
}

func transcribeDeepgramAudio(ctx context.Context, request mediaAudioRequest, query map[string]any) (string, error) {
	return aimedia.TranscribeDeepgramAudio(ctx, toCoreAudioRequest(request), query)
}

func transcribeGeminiAudio(ctx context.Context, request mediaAudioRequest) (string, error) {
	return aimedia.TranscribeGeminiAudio(ctx, toCoreAudioRequest(request))
}

func describeGeminiVideo(ctx context.Context, request mediaVideoRequest) (string, error) {
	return aimedia.DescribeGeminiVideo(ctx, toCoreVideoRequest(request))
}

func describeGeminiImage(ctx context.Context, request mediaImageRequest) (string, error) {
	return aimedia.DescribeGeminiImage(ctx, toCoreImageRequest(request))
}

func hasProviderAuthHeader(providerID string, headers map[string]string) bool {
	return aimedia.HasProviderAuthHeader(providerID, headers)
}

func resolveMediaFileName(fileName string, capability string, mediaURL string) string {
	return aimedia.ResolveMediaFileName(fileName, capability, mediaURL)
}

func toCoreAudioRequest(request mediaAudioRequest) aimedia.AudioRequest {
	return aimedia.AudioRequest{
		Provider: request.Provider,
		APIKey:   request.APIKey,
		BaseURL:  request.BaseURL,
		Headers:  request.Headers,
		Model:    request.Model,
		Language: request.Language,
		Prompt:   request.Prompt,
		MimeType: request.MimeType,
		FileName: request.FileName,
		Data:     request.Data,
		Timeout:  request.Timeout,
	}
}

func toCoreVideoRequest(request mediaVideoRequest) aimedia.VideoRequest {
	return aimedia.VideoRequest{
		APIKey:   request.APIKey,
		BaseURL:  request.BaseURL,
		Headers:  request.Headers,
		Model:    request.Model,
		Prompt:   request.Prompt,
		MimeType: request.MimeType,
		Data:     request.Data,
		Timeout:  request.Timeout,
	}
}

func toCoreImageRequest(request mediaImageRequest) aimedia.ImageRequest {
	return aimedia.ImageRequest{
		APIKey:   request.APIKey,
		BaseURL:  request.BaseURL,
		Headers:  request.Headers,
		Model:    request.Model,
		Prompt:   request.Prompt,
		MimeType: request.MimeType,
		Data:     request.Data,
		Timeout:  request.Timeout,
	}
}

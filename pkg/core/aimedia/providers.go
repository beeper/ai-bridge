package aimedia

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultOpenAITranscriptionBaseURL = "https://api.openai.com/v1"
	DefaultGroqTranscriptionBaseURL   = "https://api.groq.com/openai/v1"
	DefaultDeepgramBaseURL            = "https://api.deepgram.com/v1"
	DefaultGoogleBaseURL              = "https://generativelanguage.googleapis.com/v1beta"
	DefaultGoogleAudioModel           = "gemini-3-flash-preview"
	DefaultGoogleImageModel           = "gemini-3-flash-preview"
	DefaultGoogleVideoModel           = "gemini-3-flash-preview"
)

// ProviderCapabilities maps provider IDs to the media capabilities they support.
var ProviderCapabilities = map[string][]MediaUnderstandingCapability{
	"openai":     {MediaCapabilityImage, MediaCapabilityAudio},
	"groq":       {MediaCapabilityAudio},
	"deepgram":   {MediaCapabilityAudio},
	"google":     {MediaCapabilityImage, MediaCapabilityAudio, MediaCapabilityVideo},
	"openrouter": {MediaCapabilityImage, MediaCapabilityVideo},
}

// NormalizeProviderID normalizes a provider name (e.g. "gemini" -> "google").
func NormalizeProviderID(id string) string {
	normalized := strings.ToLower(strings.TrimSpace(id))
	switch normalized {
	case "gemini":
		return "google"
	case "beeper":
		return "openrouter"
	case "magic_proxy":
		return "openrouter"
	default:
		return normalized
	}
}

// NormalizeBaseURL trims a base URL or returns the fallback.
func NormalizeBaseURL(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return strings.TrimRight(trimmed, "/")
}

// AudioRequest holds parameters for an audio transcription API call.
type AudioRequest struct {
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

// Base64Data returns the Data field as a base64-encoded string.
func (r AudioRequest) Base64Data() string {
	return base64.StdEncoding.EncodeToString(r.Data)
}

// MimeTypeOrDefault returns the MimeType if set, otherwise the fallback.
func (r AudioRequest) MimeTypeOrDefault(fallback string) string {
	if strings.TrimSpace(r.MimeType) != "" {
		return r.MimeType
	}
	return fallback
}

// VideoRequest holds parameters for a video description API call.
type VideoRequest struct {
	APIKey   string
	BaseURL  string
	Headers  map[string]string
	Model    string
	Prompt   string
	MimeType string
	Data     []byte
	Timeout  time.Duration
}

// Base64Data returns the Data field as a base64-encoded string.
func (r VideoRequest) Base64Data() string {
	return base64.StdEncoding.EncodeToString(r.Data)
}

// MimeTypeOrDefault returns the MimeType if set, otherwise the fallback.
func (r VideoRequest) MimeTypeOrDefault(fallback string) string {
	if strings.TrimSpace(r.MimeType) != "" {
		return r.MimeType
	}
	return fallback
}

// ImageRequest holds parameters for an image description API call.
type ImageRequest struct {
	APIKey   string
	BaseURL  string
	Headers  map[string]string
	Model    string
	Prompt   string
	MimeType string
	Data     []byte
	Timeout  time.Duration
}

// Base64Data returns the Data field as a base64-encoded string.
func (r ImageRequest) Base64Data() string {
	return base64.StdEncoding.EncodeToString(r.Data)
}

// MimeTypeOrDefault returns the MimeType if set, otherwise the fallback.
func (r ImageRequest) MimeTypeOrDefault(fallback string) string {
	if strings.TrimSpace(r.MimeType) != "" {
		return r.MimeType
	}
	return fallback
}

// TranscribeOpenAIAudio sends audio data to an OpenAI-style
// transcription endpoint and returns the text.
func TranscribeOpenAIAudio(ctx context.Context, params AudioRequest) (string, error) {
	baseURL := NormalizeBaseURL(params.BaseURL, DefaultOpenAITranscriptionBaseURL)
	if params.Provider == "groq" {
		baseURL = NormalizeBaseURL(params.BaseURL, DefaultGroqTranscriptionBaseURL)
	}
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = DefaultAudioModelsByProvider[params.Provider]
	}
	if model == "" {
		return "", errors.New("missing transcription model")
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", params.FileName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(params.Data); err != nil {
		return "", err
	}
	_ = writer.WriteField("model", model)
	if params.Language != "" {
		_ = writer.WriteField("language", params.Language)
	}
	if params.Prompt != "" {
		_ = writer.WriteField("prompt", params.Prompt)
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/transcriptions", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ApplyHeaderMap(req.Header, params.Headers)
	if !HeaderExists(req.Header, "Authorization") && params.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+params.APIKey)
	}

	client := &http.Client{Timeout: params.Timeout}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		detail := ReadErrorResponse(res)
		if detail != "" {
			return "", fmt.Errorf("audio transcription failed (HTTP %d): %s", res.StatusCode, detail)
		}
		return "", fmt.Errorf("audio transcription failed (HTTP %d)", res.StatusCode)
	}
	defer res.Body.Close()
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}
	text := strings.TrimSpace(payload.Text)
	if text == "" {
		return "", errors.New("audio transcription response missing text")
	}
	return text, nil
}

// TranscribeDeepgramAudio sends audio data to Deepgram's API.
func TranscribeDeepgramAudio(ctx context.Context, params AudioRequest, query map[string]any) (string, error) {
	baseURL := NormalizeBaseURL(params.BaseURL, DefaultDeepgramBaseURL)
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = DefaultAudioModelsByProvider["deepgram"]
	}
	if model == "" {
		return "", errors.New("missing transcription model")
	}

	endpoint, err := url.Parse(baseURL + "/listen")
	if err != nil {
		return "", err
	}
	q := endpoint.Query()
	q.Set("model", model)
	if params.Language != "" {
		q.Set("language", params.Language)
	}
	for key, value := range query {
		if value == nil {
			continue
		}
		q.Set(key, fmt.Sprint(value))
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(params.Data))
	if err != nil {
		return "", err
	}
	ApplyHeaderMap(req.Header, params.Headers)
	if !HeaderExists(req.Header, "Authorization") && params.APIKey != "" {
		req.Header.Set("Authorization", "Token "+params.APIKey)
	}
	if !HeaderExists(req.Header, "Content-Type") {
		mimeType := params.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		req.Header.Set("Content-Type", mimeType)
	}

	client := &http.Client{Timeout: params.Timeout}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		detail := ReadErrorResponse(res)
		if detail != "" {
			return "", fmt.Errorf("audio transcription failed (HTTP %d): %s", res.StatusCode, detail)
		}
		return "", fmt.Errorf("audio transcription failed (HTTP %d)", res.StatusCode)
	}
	defer res.Body.Close()
	var payload struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Results.Channels) == 0 || len(payload.Results.Channels[0].Alternatives) == 0 {
		return "", errors.New("audio transcription response missing transcript")
	}
	text := strings.TrimSpace(payload.Results.Channels[0].Alternatives[0].Transcript)
	if text == "" {
		return "", errors.New("audio transcription response missing transcript")
	}
	return text, nil
}

// TranscribeGeminiAudio sends audio data to Google's Gemini API.
func TranscribeGeminiAudio(ctx context.Context, params AudioRequest) (string, error) {
	baseURL := NormalizeBaseURL(params.BaseURL, DefaultGoogleBaseURL)
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = DefaultGoogleAudioModel
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", baseURL, model)

	prompt := strings.TrimSpace(params.Prompt)
	if prompt == "" {
		prompt = DefaultPromptByCapability[MediaCapabilityAudio]
	}

	body := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": prompt},
					{
						"inline_data": map[string]any{
							"mime_type": params.MimeTypeOrDefault("audio/wav"),
							"data":      params.Base64Data(),
						},
					},
				},
			},
		},
	}
	return callGeminiGenerateContent(ctx, endpoint, body, params.Headers, params.APIKey, params.Timeout, "audio transcription")
}

// DescribeGeminiVideo sends video data to Google's Gemini API for description.
func DescribeGeminiVideo(ctx context.Context, params VideoRequest) (string, error) {
	baseURL := NormalizeBaseURL(params.BaseURL, DefaultGoogleBaseURL)
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = DefaultGoogleVideoModel
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", baseURL, model)

	prompt := strings.TrimSpace(params.Prompt)
	if prompt == "" {
		prompt = DefaultPromptByCapability[MediaCapabilityVideo]
	}

	body := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": prompt},
					{
						"inline_data": map[string]any{
							"mime_type": params.MimeTypeOrDefault("video/mp4"),
							"data":      params.Base64Data(),
						},
					},
				},
			},
		},
	}
	return callGeminiGenerateContent(ctx, endpoint, body, params.Headers, params.APIKey, params.Timeout, "video description")
}

// DescribeGeminiImage sends image data to Google's Gemini API for description.
func DescribeGeminiImage(ctx context.Context, params ImageRequest) (string, error) {
	baseURL := NormalizeBaseURL(params.BaseURL, DefaultGoogleBaseURL)
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = DefaultGoogleImageModel
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", baseURL, model)

	prompt := strings.TrimSpace(params.Prompt)
	if prompt == "" {
		prompt = DefaultPromptByCapability[MediaCapabilityImage]
	}

	body := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": prompt},
					{
						"inline_data": map[string]any{
							"mime_type": params.MimeTypeOrDefault("image/jpeg"),
							"data":      params.Base64Data(),
						},
					},
				},
			},
		},
	}
	return callGeminiGenerateContent(ctx, endpoint, body, params.Headers, params.APIKey, params.Timeout, "image description")
}

// callGeminiGenerateContent is a shared helper for Gemini API calls.
func callGeminiGenerateContent(ctx context.Context, endpoint string, body map[string]any, headers map[string]string, apiKey string, timeout time.Duration, kind string) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	ApplyHeaderMap(req.Header, headers)
	if !HeaderExists(req.Header, "Content-Type") {
		req.Header.Set("Content-Type", "application/json")
	}
	if !HeaderExists(req.Header, "X-Goog-Api-Key") && apiKey != "" {
		req.Header.Set("X-Goog-Api-Key", apiKey)
	}

	client := &http.Client{Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		detail := ReadErrorResponse(res)
		if detail != "" {
			return "", fmt.Errorf("%s failed (HTTP %d): %s", kind, res.StatusCode, detail)
		}
		return "", fmt.Errorf("%s failed (HTTP %d)", kind, res.StatusCode)
	}
	defer res.Body.Close()
	var payloadResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payloadResp); err != nil {
		return "", err
	}
	if len(payloadResp.Candidates) == 0 {
		return "", fmt.Errorf("%s response missing text", kind)
	}
	var parts []string
	for _, part := range payloadResp.Candidates[0].Content.Parts {
		if trimmed := strings.TrimSpace(part.Text); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("%s response missing text", kind)
	}
	return strings.Join(parts, "\n"), nil
}

// ResolveMediaFileName picks a filename from fallback, URL or message type.
func ResolveMediaFileName(fallback string, msgType string, mediaURL string) string {
	base := strings.TrimSpace(fallback)
	if base != "" {
		return base
	}
	if mediaURL != "" {
		if parsed, err := url.Parse(mediaURL); err == nil {
			if parsed.Path != "" {
				if name := filepath.Base(parsed.Path); name != "." && name != "/" {
					return name
				}
			}
		}
		if strings.HasPrefix(mediaURL, "file://") {
			path := strings.TrimPrefix(mediaURL, "file://")
			if name := filepath.Base(path); name != "." && name != "/" {
				return name
			}
		}
	}
	if msgType != "" {
		return msgType
	}
	return "media"
}

// ReadErrorResponse reads up to 4KB from an error response body.
func ReadErrorResponse(res *http.Response) string {
	if res == nil || res.Body == nil {
		return ""
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

// HeaderExists checks if an HTTP header is present.
func HeaderExists(headers http.Header, name string) bool {
	_, ok := headers[http.CanonicalHeaderKey(name)]
	return ok
}

// ApplyHeaderMap sets headers from a string map, skipping empty values.
func ApplyHeaderMap(headers http.Header, values map[string]string) {
	for key, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		headers.Set(key, trimmed)
	}
}

// ResolveProviderQuery builds the query/options map for a media provider.
func ResolveProviderQuery(providerID string, cfg *CapabilityConfig, entry ModelConfig) map[string]any {
	merged := map[string]any{}
	var cfgOptions map[string]map[string]any
	if cfg != nil {
		cfgOptions = cfg.ProviderOptions
	}
	for _, src := range []map[string]map[string]any{cfgOptions, entry.ProviderOptions} {
		if src == nil {
			continue
		}
		options, ok := src[providerID]
		if !ok {
			continue
		}
		for key, value := range options {
			if value == nil {
				continue
			}
			merged[key] = value
		}
	}
	if providerID != "deepgram" {
		if len(merged) == 0 {
			return nil
		}
		return merged
	}
	normalized := map[string]any{}
	for key, value := range merged {
		switch key {
		case "detectLanguage":
			normalized["detect_language"] = value
		case "smartFormat":
			normalized["smart_format"] = value
		default:
			normalized[key] = value
		}
	}
	for key, value := range BuildDeepgramQuery(cfg, entry) {
		if _, ok := normalized[key]; !ok {
			normalized[key] = value
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// BuildDeepgramQuery produces Deepgram query params.
func BuildDeepgramQuery(cfg *CapabilityConfig, entry ModelConfig) map[string]any {
	var source *DeepgramConfig
	if entry.Deepgram != nil {
		source = entry.Deepgram
	} else if cfg != nil && cfg.Deepgram != nil {
		source = cfg.Deepgram
	}
	if source == nil {
		return nil
	}
	query := map[string]any{}
	if source.DetectLanguage != nil {
		query["detect_language"] = *source.DetectLanguage
	}
	if source.Punctuate != nil {
		query["punctuate"] = *source.Punctuate
	}
	if source.SmartFormat != nil {
		query["smart_format"] = *source.SmartFormat
	}
	if len(query) == 0 {
		return nil
	}
	return query
}

// HasProviderAuthHeader checks if the merged headers contain an Authorization
// or provider-specific auth header for the given provider.
func HasProviderAuthHeader(providerID string, headers map[string]string) bool {
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(key))
		if k == "authorization" {
			return true
		}
		if providerID == "google" && k == "x-goog-api-key" {
			return true
		}
		if providerID == "deepgram" && k == "authorization" {
			return true
		}
	}
	return false
}

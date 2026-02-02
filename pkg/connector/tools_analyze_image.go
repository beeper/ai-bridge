package connector

import (
	"context"
	"fmt"
	"strings"
)

// executeAnalyzeImage analyzes an image with a custom prompt using vision capabilities.
func executeAnalyzeImage(ctx context.Context, args map[string]any) (string, error) {
	imageURL, ok := args["image_url"].(string)
	if !ok || imageURL == "" {
		return "", fmt.Errorf("missing or invalid 'image_url' argument")
	}

	prompt, ok := args["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("missing or invalid 'prompt' argument")
	}

	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("analyze_image requires bridge context")
	}

	// Check if the current model supports vision
	if btc.Meta == nil || !btc.Meta.Capabilities.SupportsVision {
		return "", fmt.Errorf("current model does not support vision/image analysis")
	}

	// Get image data based on URL type
	var imageB64, mimeType string
	var err error

	if strings.HasPrefix(imageURL, "data:") {
		// Parse data URI (data:image/png;base64,...)
		imageB64, mimeType, err = parseDataURI(imageURL)
		if err != nil {
			return "", fmt.Errorf("failed to parse data URI: %w", err)
		}
	} else if strings.HasPrefix(imageURL, "mxc://") {
		// Matrix media URL - use bridge's download function
		imageB64, mimeType, err = btc.Client.downloadAndEncodeMedia(ctx, imageURL, nil, 20)
		if err != nil {
			return "", fmt.Errorf("failed to download Matrix media: %w", err)
		}
	} else if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		// HTTP(S) URL - fetch and encode
		imageB64, err = fetchImageAsBase64(ctx, imageURL)
		if err != nil {
			return "", fmt.Errorf("failed to fetch image: %w", err)
		}
		// Infer mime type from URL or default to jpeg
		mimeType = inferMimeTypeFromURL(imageURL)
	} else {
		return "", fmt.Errorf("unsupported URL scheme: must be http://, https://, mxc://, or data:")
	}

	// Build vision request with image and prompt
	messages := []UnifiedMessage{
		{
			Role: RoleUser,
			Content: []ContentPart{
				{
					Type:     ContentTypeImage,
					ImageB64: imageB64,
					MimeType: mimeType,
				},
				{
					Type: ContentTypeText,
					Text: prompt,
				},
			},
		},
	}

	// Call the AI provider for vision analysis
	resp, err := btc.Client.provider.Generate(ctx, GenerateParams{
		Model:               btc.Meta.Model,
		Messages:            messages,
		MaxCompletionTokens: 4096,
	})
	if err != nil {
		return "", fmt.Errorf("vision analysis failed: %w", err)
	}

	// Return the analysis result
	return fmt.Sprintf(`{"analysis":%q,"image_url":%q}`, resp.Content, imageURL), nil
}

// parseDataURI parses a data URI and returns base64 data and mime type.
func parseDataURI(dataURI string) (string, string, error) {
	// Format: data:[<mediatype>][;base64],<data>
	if !strings.HasPrefix(dataURI, "data:") {
		return "", "", fmt.Errorf("not a data URI")
	}

	// Remove "data:" prefix
	rest := dataURI[5:]

	// Find the comma separator
	commaIdx := strings.Index(rest, ",")
	if commaIdx == -1 {
		return "", "", fmt.Errorf("invalid data URI: no comma separator")
	}

	metadata := rest[:commaIdx]
	data := rest[commaIdx+1:]

	// Check if base64 encoded
	if !strings.Contains(metadata, ";base64") {
		return "", "", fmt.Errorf("only base64 data URIs are supported")
	}

	// Extract mime type
	mimeType := strings.Split(metadata, ";")[0]
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return data, mimeType, nil
}

// inferMimeTypeFromURL guesses the mime type from a URL's file extension.
func inferMimeTypeFromURL(imageURL string) string {
	lowerURL := strings.ToLower(imageURL)
	switch {
	case strings.Contains(lowerURL, ".png"):
		return "image/png"
	case strings.Contains(lowerURL, ".gif"):
		return "image/gif"
	case strings.Contains(lowerURL, ".webp"):
		return "image/webp"
	case strings.Contains(lowerURL, ".svg"):
		return "image/svg+xml"
	default:
		return "image/jpeg"
	}
}

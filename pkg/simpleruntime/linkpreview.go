package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/linkpreview"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Backward-compatible type aliases that delegate to pkg/linkpreview.
type (
	LinkPreviewConfig = linkpreview.Config
	PreviewWithImage  = linkpreview.PreviewWithImage
	LinkPreviewer     = linkpreview.Previewer
)

// DefaultLinkPreviewConfig returns sensible defaults.
func DefaultLinkPreviewConfig() LinkPreviewConfig {
	return linkpreview.DefaultConfig()
}

// NewLinkPreviewer creates a new link previewer with the given config.
func NewLinkPreviewer(config LinkPreviewConfig) *LinkPreviewer {
	return linkpreview.NewPreviewer(config)
}

// ExtractURLs extracts URLs from text, returning up to maxURLs unique URLs.
func ExtractURLs(text string, maxURLs int) []string {
	return linkpreview.ExtractURLs(text, maxURLs)
}

// UploadPreviewImages uploads images from PreviewWithImage to Matrix and returns final BeeperLinkPreviews.
func UploadPreviewImages(ctx context.Context, previews []*PreviewWithImage, intent bridgev2.MatrixAPI, roomID id.RoomID) []*event.BeeperLinkPreview {
	return linkpreview.UploadPreviewImages(ctx, previews, intent, roomID)
}

// ExtractBeeperPreviews extracts just the BeeperLinkPreview from PreviewWithImage slice.
func ExtractBeeperPreviews(previews []*PreviewWithImage) []*event.BeeperLinkPreview {
	return linkpreview.ExtractBeeperPreviews(previews)
}

// FormatPreviewsForContext formats link previews for injection into LLM context.
func FormatPreviewsForContext(previews []*event.BeeperLinkPreview, maxChars int) string {
	return linkpreview.FormatPreviewsForContext(previews, maxChars)
}

// ParseExistingLinkPreviews extracts link previews from a Matrix event's raw content.
func ParseExistingLinkPreviews(rawContent map[string]any) []*event.BeeperLinkPreview {
	return linkpreview.ParseExistingLinkPreviews(rawContent)
}

// PreviewsToMapSlice converts BeeperLinkPreviews to a format suitable for JSON serialization.
func PreviewsToMapSlice(previews []*event.BeeperLinkPreview) []map[string]any {
	return linkpreview.PreviewsToMapSlice(previews)
}

// sourceCitationToLinkpreviewCitation converts the connector-internal sourceCitation
// to the public linkpreview.Citation type.
func sourceCitationToLinkpreviewCitation(c sourceCitation) linkpreview.Citation {
	return linkpreview.Citation{
		URL:         c.URL,
		Title:       c.Title,
		Description: c.Description,
		SiteName:    c.SiteName,
		Image:       c.Image,
	}
}

// sourceCitationsToLinkpreviewCitations converts a slice of sourceCitation to linkpreview.Citation.
func sourceCitationsToLinkpreviewCitations(cs []sourceCitation) []linkpreview.Citation {
	if len(cs) == 0 {
		return nil
	}
	out := make([]linkpreview.Citation, len(cs))
	for i, c := range cs {
		out[i] = sourceCitationToLinkpreviewCitation(c)
	}
	return out
}

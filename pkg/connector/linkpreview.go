package connector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/dyatlov/go-opengraph/opengraph"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// LinkPreviewConfig holds configuration for link preview functionality.
type LinkPreviewConfig struct {
	Enabled         bool          `yaml:"enabled"`
	MaxURLsInbound  int           `yaml:"max_urls_inbound"`  // Max URLs to process from user messages
	MaxURLsOutbound int           `yaml:"max_urls_outbound"` // Max URLs to preview in AI responses
	FetchTimeout    time.Duration `yaml:"fetch_timeout"`     // Timeout for fetching each URL
	MaxContentChars int           `yaml:"max_content_chars"` // Max chars for description in context
	MaxPageBytes    int64         `yaml:"max_page_bytes"`    // Max page size to download
}

// DefaultLinkPreviewConfig returns sensible defaults.
func DefaultLinkPreviewConfig() LinkPreviewConfig {
	return LinkPreviewConfig{
		Enabled:         true,
		MaxURLsInbound:  3,
		MaxURLsOutbound: 5,
		FetchTimeout:    10 * time.Second,
		MaxContentChars: 500,
		MaxPageBytes:    10 * 1024 * 1024, // 10MB
	}
}

// LinkPreviewer handles URL preview generation.
type LinkPreviewer struct {
	config     LinkPreviewConfig
	httpClient *http.Client
}

// NewLinkPreviewer creates a new link previewer with the given config.
func NewLinkPreviewer(config LinkPreviewConfig) *LinkPreviewer {
	return &LinkPreviewer{
		config: config,
		httpClient: &http.Client{
			Timeout: config.FetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// URL matching regex - matches http/https URLs
var urlRegex = regexp.MustCompile(`https?://[^\s<>\[\]()'"]+[^\s<>\[\]()'",.:;!?]`)

// ExtractURLs extracts URLs from text, returning up to maxURLs unique URLs.
func ExtractURLs(text string, maxURLs int) []string {
	if maxURLs <= 0 {
		return nil
	}

	matches := urlRegex.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	// Deduplicate and limit
	seen := make(map[string]bool)
	var urls []string
	for _, match := range matches {
		// Clean up trailing punctuation that might have been captured
		cleaned := strings.TrimRight(match, ".,;:!?")
		if seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		urls = append(urls, cleaned)
		if len(urls) >= maxURLs {
			break
		}
	}
	return urls
}

// FetchPreview fetches and generates a link preview for a URL.
func (lp *LinkPreviewer) FetchPreview(ctx context.Context, urlStr string) (*event.BeeperLinkPreview, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to look like a browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := lp.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml") {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, lp.config.MaxPageBytes)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse with OpenGraph
	og := opengraph.NewOpenGraph()
	if err := og.ProcessHTML(strings.NewReader(string(body))); err != nil {
		return nil, fmt.Errorf("failed to parse OpenGraph: %w", err)
	}

	// Fallback parsing with goquery if OpenGraph data is incomplete
	var doc *goquery.Document
	if og.Title == "" || og.Description == "" {
		doc, err = goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err == nil {
			if og.Title == "" {
				og.Title = extractTitle(doc)
			}
			if og.Description == "" {
				og.Description = extractDescription(doc)
			}
		}
	}

	// Build preview
	preview := &event.BeeperLinkPreview{
		LinkPreview: event.LinkPreview{
			CanonicalURL: og.URL,
			Title:        summarizeText(og.Title, 30, 150),
			Type:         og.Type,
			Description:  summarizeText(og.Description, 50, 200),
			SiteName:     og.SiteName,
		},
		MatchedURL: urlStr,
	}

	// Use the original URL if canonical is empty
	if preview.CanonicalURL == "" {
		preview.CanonicalURL = urlStr
	}

	// Handle image (we don't upload it, just note its URL for context)
	// In a full implementation, we'd upload to Matrix and set ImageURL
	if len(og.Images) > 0 && og.Images[0].URL != "" {
		// For now, we skip image uploading - just use the preview data
		// Image handling would require Matrix media upload
	}

	return preview, nil
}

// FetchPreviews fetches previews for multiple URLs in parallel.
func (lp *LinkPreviewer) FetchPreviews(ctx context.Context, urls []string) []*event.BeeperLinkPreview {
	if len(urls) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	results := make([]*event.BeeperLinkPreview, len(urls))
	
	for i, u := range urls {
		wg.Add(1)
		go func(idx int, urlStr string) {
			defer wg.Done()
			preview, err := lp.FetchPreview(ctx, urlStr)
			if err == nil && preview != nil {
				results[idx] = preview
			}
		}(i, u)
	}

	wg.Wait()

	// Filter out nil results
	var previews []*event.BeeperLinkPreview
	for _, p := range results {
		if p != nil {
			previews = append(previews, p)
		}
	}
	return previews
}

// FormatPreviewsForContext formats link previews for injection into LLM context.
func FormatPreviewsForContext(previews []*event.BeeperLinkPreview, maxChars int) string {
	if len(previews) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n[Referenced Links]\n")

	for i, p := range previews {
		if p == nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, p.MatchedURL))
		if p.Title != "" {
			sb.WriteString(fmt.Sprintf("   Title: %s\n", p.Title))
		}
		if p.Description != "" {
			desc := p.Description
			if len(desc) > maxChars {
				desc = desc[:maxChars] + "..."
			}
			sb.WriteString(fmt.Sprintf("   Description: %s\n", desc))
		}
		if p.SiteName != "" {
			sb.WriteString(fmt.Sprintf("   Site: %s\n", p.SiteName))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ParseExistingLinkPreviews extracts link previews from a Matrix event's raw content.
func ParseExistingLinkPreviews(rawContent map[string]any) []*event.BeeperLinkPreview {
	previewsRaw, ok := rawContent["com.beeper.linkpreviews"]
	if !ok {
		return nil
	}

	previewsList, ok := previewsRaw.([]any)
	if !ok {
		return nil
	}

	var previews []*event.BeeperLinkPreview
	for _, p := range previewsList {
		previewMap, ok := p.(map[string]any)
		if !ok {
			continue
		}

		preview := &event.BeeperLinkPreview{}
		
		if v, ok := previewMap["matched_url"].(string); ok {
			preview.MatchedURL = v
		}
		if v, ok := previewMap["og:url"].(string); ok {
			preview.CanonicalURL = v
		}
		if v, ok := previewMap["og:title"].(string); ok {
			preview.Title = v
		}
		if v, ok := previewMap["og:description"].(string); ok {
			preview.Description = v
		}
		if v, ok := previewMap["og:type"].(string); ok {
			preview.Type = v
		}
		if v, ok := previewMap["og:site_name"].(string); ok {
			preview.SiteName = v
		}
		if v, ok := previewMap["og:image"].(string); ok {
			preview.ImageURL = id.ContentURIString(v)
		}

		if preview.MatchedURL != "" || preview.CanonicalURL != "" {
			previews = append(previews, preview)
		}
	}

	return previews
}

// PreviewsToMapSlice converts BeeperLinkPreviews to a format suitable for JSON serialization.
func PreviewsToMapSlice(previews []*event.BeeperLinkPreview) []map[string]any {
	if len(previews) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(previews))
	for _, p := range previews {
		if p == nil {
			continue
		}

		m := map[string]any{
			"matched_url": p.MatchedURL,
		}
		if p.CanonicalURL != "" {
			m["og:url"] = p.CanonicalURL
		}
		if p.Title != "" {
			m["og:title"] = p.Title
		}
		if p.Description != "" {
			m["og:description"] = p.Description
		}
		if p.Type != "" {
			m["og:type"] = p.Type
		}
		if p.SiteName != "" {
			m["og:site_name"] = p.SiteName
		}
		if p.ImageURL != "" {
			m["og:image"] = string(p.ImageURL)
		}

		result = append(result, m)
	}
	return result
}

// extractTitle extracts a title from HTML using goquery.
func extractTitle(doc *goquery.Document) string {
	// Try <title> tag first
	if title := doc.Find("title").First().Text(); title != "" {
		return strings.TrimSpace(title)
	}
	// Try h1
	if h1 := doc.Find("h1").First().Text(); h1 != "" {
		return strings.TrimSpace(h1)
	}
	// Try h2
	if h2 := doc.Find("h2").First().Text(); h2 != "" {
		return strings.TrimSpace(h2)
	}
	return ""
}

// extractDescription extracts a description from HTML using goquery.
func extractDescription(doc *goquery.Document) string {
	// Try meta description
	if desc, exists := doc.Find("meta[name='description']").First().Attr("content"); exists && desc != "" {
		return strings.TrimSpace(desc)
	}
	// Try first paragraph
	if p := doc.Find("p").First().Text(); p != "" {
		return strings.TrimSpace(p)
	}
	return ""
}

// summarizeText truncates text to maxWords and maxLength.
func summarizeText(text string, maxWords, maxLength int) string {
	// Normalize whitespace
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	if text == "" {
		return ""
	}

	// Limit words
	words := strings.Fields(text)
	if len(words) > maxWords {
		text = strings.Join(words[:maxWords], " ")
	}

	// Limit length
	if len(text) > maxLength {
		text = text[:maxLength]
		// Try to cut at word boundary
		if lastSpace := strings.LastIndex(text, " "); lastSpace > maxLength/2 {
			text = text[:lastSpace]
		}
		text += "..."
	}

	return text
}

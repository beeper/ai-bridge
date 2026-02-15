// Package citations provides utilities for extracting, merging and formatting
// source citations from OpenAI-style API responses and tool outputs.
package citations

import (
	"encoding/json"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
)

// Source represents a web source citation with metadata.
type Source struct {
	URL         string
	Title       string
	Description string
	Published   string
	SiteName    string
	Author      string
	Image       string
	Favicon     string
}

// Document represents a file/document citation.
type Document struct {
	ID        string
	Title     string
	Filename  string
	MediaType string
}

// ExtractURLCitation extracts a url_citation from a raw annotation map.
func ExtractURLCitation(annotation any) (Source, bool) {
	raw, ok := annotation.(map[string]any)
	if !ok {
		return Source{}, false
	}
	typ, _ := raw["type"].(string)
	if typ != "url_citation" {
		return Source{}, false
	}
	urlStr, ok := readStringArg(raw, "url")
	if !ok {
		return Source{}, false
	}
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return Source{}, false
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return Source{}, false
	}
	title, _ := readStringArg(raw, "title")
	return Source{URL: urlStr, Title: title}, true
}

// ExtractDocumentCitation extracts a document citation from a raw annotation.
func ExtractDocumentCitation(annotation any) (Document, bool) {
	raw, ok := annotation.(map[string]any)
	if !ok {
		return Document{}, false
	}
	typ, _ := raw["type"].(string)
	switch typ {
	case "file_citation", "container_file_citation", "file_path":
	default:
		return Document{}, false
	}

	fileID, _ := readStringArg(raw, "file_id")
	filename, _ := readStringArg(raw, "filename")
	title := filename
	if strings.TrimSpace(title) == "" {
		title = fileID
	}
	if strings.TrimSpace(title) == "" {
		return Document{}, false
	}
	mediaType := "application/octet-stream"
	if ext := strings.TrimSpace(filepath.Ext(filename)); ext != "" {
		if inferred := mime.TypeByExtension(ext); inferred != "" {
			mediaType = inferred
		}
	}

	return Document{
		ID:        fileID,
		Title:     title,
		Filename:  filename,
		MediaType: mediaType,
	}, true
}

// ExtractWebSearchCitations extracts source citations from a web search tool
// output JSON. The toolName is compared against webSearchToolName after
// applying the optional normalizer function. If normalize is nil, the raw
// toolName is compared directly.
func ExtractWebSearchCitations(toolName, output string, webSearchToolName string, normalize func(string) string) []Source {
	name := strings.TrimSpace(toolName)
	if normalize != nil {
		name = normalize(name)
	}
	if name != webSearchToolName {
		return nil
	}
	output = strings.TrimSpace(output)
	if output == "" || !strings.HasPrefix(output, "{") {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil
	}

	rawResults, ok := payload["results"].([]any)
	if !ok || len(rawResults) == 0 {
		return nil
	}

	out := make([]Source, 0, len(rawResults))
	for _, rawResult := range rawResults {
		entry, ok := rawResult.(map[string]any)
		if !ok {
			continue
		}
		urlStr, ok := readStringArg(entry, "url")
		if !ok {
			continue
		}
		parsed, err := url.Parse(urlStr)
		if err != nil {
			continue
		}
		switch parsed.Scheme {
		case "http", "https":
		default:
			continue
		}
		title, _ := readStringArg(entry, "title")
		description, _ := readStringArg(entry, "description")
		published, _ := readStringArg(entry, "published")
		siteName, _ := readStringArg(entry, "siteName")
		author, _ := readStringArg(entry, "author")
		image, _ := readStringArg(entry, "image")
		favicon, _ := readStringArg(entry, "favicon")
		out = append(out, Source{
			URL:         urlStr,
			Title:       title,
			Description: description,
			Published:   published,
			SiteName:    siteName,
			Author:      author,
			Image:       image,
			Favicon:     favicon,
		})
	}
	return out
}

// MergeSources deduplicates and merges source citations by URL.
func MergeSources(existing, incoming []Source) []Source {
	if len(incoming) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(incoming))
	merged := make([]Source, 0, len(existing)+len(incoming))
	for _, citation := range existing {
		urlStr := strings.TrimSpace(citation.URL)
		if urlStr == "" {
			continue
		}
		if idx, ok := seen[urlStr]; ok {
			merged[idx] = mergeFields(merged[idx], citation)
			continue
		}
		seen[urlStr] = len(merged)
		merged = append(merged, citation)
	}
	for _, citation := range incoming {
		urlStr := strings.TrimSpace(citation.URL)
		if urlStr == "" {
			continue
		}
		if idx, ok := seen[urlStr]; ok {
			merged[idx] = mergeFields(merged[idx], citation)
			continue
		}
		seen[urlStr] = len(merged)
		merged = append(merged, citation)
	}
	return merged
}

// ProviderMetadata builds a provider metadata map from a Source for Matrix
// message formatting. Returns nil if the source has no extra metadata.
func ProviderMetadata(c Source) map[string]any {
	meta := map[string]any{}
	if desc := strings.TrimSpace(c.Description); desc != "" {
		meta["description"] = desc
	}
	if published := strings.TrimSpace(c.Published); published != "" {
		meta["published"] = published
	}
	if site := strings.TrimSpace(c.SiteName); site != "" {
		meta["site_name"] = site
		meta["siteName"] = site // camelCase alias for client usage
	}
	if author := strings.TrimSpace(c.Author); author != "" {
		meta["author"] = author
	}
	if image := strings.TrimSpace(c.Image); image != "" {
		meta["image"] = image
	}
	if favicon := strings.TrimSpace(c.Favicon); favicon != "" {
		meta["favicon"] = favicon
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func mergeFields(dst, src Source) Source {
	if strings.TrimSpace(dst.Title) == "" {
		dst.Title = src.Title
	}
	if strings.TrimSpace(dst.Description) == "" {
		dst.Description = src.Description
	}
	if strings.TrimSpace(dst.Published) == "" {
		dst.Published = src.Published
	}
	if strings.TrimSpace(dst.SiteName) == "" {
		dst.SiteName = src.SiteName
	}
	if strings.TrimSpace(dst.Author) == "" {
		dst.Author = src.Author
	}
	if strings.TrimSpace(dst.Image) == "" {
		dst.Image = src.Image
	}
	if strings.TrimSpace(dst.Favicon) == "" {
		dst.Favicon = src.Favicon
	}
	return dst
}

func readStringArg(args map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if raw, ok := args[key]; ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return s, true
			}
		}
	}
	return "", false
}

// Package citations provides shared citation and document types and helper
// functions used by both the connector and bridge packages.
package citations

import (
	"fmt"
	"strings"
)

// SourceCitation represents a URL citation extracted from AI tool output.
type SourceCitation struct {
	URL         string
	Title       string
	Description string
	Published   string
	SiteName    string
	Author      string
	Image       string
	Favicon     string
}

// SourceDocument represents a file/document citation.
type SourceDocument struct {
	ID        string
	Title     string
	Filename  string
	MediaType string
}

// GeneratedFilePart pairs a URL with its media type for generated files.
type GeneratedFilePart struct {
	URL       string
	MediaType string
}

// ProviderMetadata builds the providerMetadata map for a source-url part from
// a SourceCitation. The keys match what the desktop client reads (e.g.
// "siteName" in camelCase). Emit both siteName and site_name during transition.
func ProviderMetadata(c SourceCitation) map[string]any {
	meta := map[string]any{}
	setIfNonEmpty := func(key, val string) {
		if v := strings.TrimSpace(val); v != "" {
			meta[key] = v
		}
	}
	setIfNonEmpty("description", c.Description)
	setIfNonEmpty("published", c.Published)
	if v := strings.TrimSpace(c.SiteName); v != "" {
		meta["siteName"] = v
		meta["site_name"] = v
	}
	setIfNonEmpty("author", c.Author)
	setIfNonEmpty("image", c.Image)
	setIfNonEmpty("favicon", c.Favicon)
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// MergeCitationFields fills empty fields of dst from src.
func MergeCitationFields(dst, src SourceCitation) SourceCitation {
	mergeField(&dst.Title, src.Title)
	mergeField(&dst.Description, src.Description)
	mergeField(&dst.Published, src.Published)
	mergeField(&dst.SiteName, src.SiteName)
	mergeField(&dst.Author, src.Author)
	mergeField(&dst.Image, src.Image)
	mergeField(&dst.Favicon, src.Favicon)
	return dst
}

// mergeField sets dst to src if dst is empty after trimming.
func mergeField(dst *string, src string) {
	if strings.TrimSpace(*dst) == "" {
		*dst = src
	}
}

// MergeSourceCitations deduplicates citations by URL, merging fields when the
// same URL appears more than once.
func MergeSourceCitations(existing, incoming []SourceCitation) []SourceCitation {
	if len(incoming) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(incoming))
	// Allocate a fresh slice to avoid mutating the backing array of existing.
	merged := make([]SourceCitation, 0, len(existing)+len(incoming))
	addCitation := func(citation SourceCitation) {
		url := strings.TrimSpace(citation.URL)
		if url == "" {
			return
		}
		if idx, ok := seen[url]; ok {
			merged[idx] = MergeCitationFields(merged[idx], citation)
			return
		}
		seen[url] = len(merged)
		merged = append(merged, citation)
	}
	for _, c := range existing {
		addCitation(c)
	}
	for _, c := range incoming {
		addCitation(c)
	}
	return merged
}

// AppendUniqueCitation appends a single citation, deduplicating by URL without
// allocating a map. Use this on hot paths (e.g. streaming) where citations
// arrive one at a time.
func AppendUniqueCitation(citations []SourceCitation, c SourceCitation) []SourceCitation {
	url := strings.TrimSpace(c.URL)
	if url == "" {
		return citations
	}
	for i, existing := range citations {
		if strings.TrimSpace(existing.URL) == url {
			citations[i] = MergeCitationFields(existing, c)
			return citations
		}
	}
	return append(citations, c)
}

// BuildSourceParts converts citations and documents into stream-event source
// parts. This is the base version without link-preview enrichment; callers
// needing preview data should use the connector-specific variant.
func BuildSourceParts(citations []SourceCitation, documents []SourceDocument) []map[string]any {
	if len(citations) == 0 && len(documents) == 0 {
		return nil
	}
	parts := make([]map[string]any, 0, len(citations)+len(documents))
	seen := make(map[string]struct{}, len(citations)+len(documents))
	for _, c := range citations {
		url := strings.TrimSpace(c.URL)
		if url == "" {
			continue
		}
		seenKey := "url:" + url
		if _, ok := seen[seenKey]; ok {
			continue
		}
		seen[seenKey] = struct{}{}
		p := map[string]any{
			"type":     "source-url",
			"sourceId": fmt.Sprintf("source-%d", len(parts)+1),
			"url":      url,
		}
		if title := strings.TrimSpace(c.Title); title != "" {
			p["title"] = title
		}
		if meta := ProviderMetadata(c); len(meta) > 0 {
			p["providerMetadata"] = meta
		}
		parts = append(parts, p)
	}
	for _, d := range documents {
		key := strings.TrimSpace(d.ID)
		if key == "" {
			key = strings.TrimSpace(d.Filename)
		}
		if key == "" {
			key = strings.TrimSpace(d.Title)
		}
		if key == "" {
			continue
		}
		seenKey := "doc:" + key
		if _, ok := seen[seenKey]; ok {
			continue
		}
		seen[seenKey] = struct{}{}
		p := map[string]any{
			"type":      "source-document",
			"sourceId":  fmt.Sprintf("source-%d", len(parts)+1),
			"mediaType": d.MediaType,
			"title":     d.Title,
		}
		if fn := strings.TrimSpace(d.Filename); fn != "" {
			p["filename"] = fn
		}
		parts = append(parts, p)
	}
	return parts
}

// GeneratedFilesToParts converts generated files into stream-event parts.
func GeneratedFilesToParts(files []GeneratedFilePart) []map[string]any {
	if len(files) == 0 {
		return nil
	}
	parts := make([]map[string]any, 0, len(files))
	for _, file := range files {
		url := strings.TrimSpace(file.URL)
		if url == "" {
			continue
		}
		parts = append(parts, map[string]any{
			"type":      "file",
			"url":       url,
			"mediaType": strings.TrimSpace(file.MediaType),
		})
	}
	return parts
}

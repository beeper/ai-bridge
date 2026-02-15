package connector

import "github.com/beeper/ai-bridge/pkg/matrixai/citations"

type sourceCitation struct {
	URL         string
	Title       string
	Description string
	Published   string
	SiteName    string
	Author      string
	Image       string
	Favicon     string
}

type sourceDocument struct {
	ID        string
	Title     string
	Filename  string
	MediaType string
}

func citationProviderMetadata(c sourceCitation) map[string]any {
	return citations.ProviderMetadata(citations.Source(c))
}

func extractURLCitation(annotation any) (sourceCitation, bool) {
	citation, ok := citations.ExtractURLCitation(annotation)
	if !ok {
		return sourceCitation{}, false
	}
	return sourceCitation(citation), true
}

func extractDocumentCitation(annotation any) (sourceDocument, bool) {
	document, ok := citations.ExtractDocumentCitation(annotation)
	if !ok {
		return sourceDocument{}, false
	}
	return sourceDocument(document), true
}

func mergeSourceCitations(existing, incoming []sourceCitation) []sourceCitation {
	merged := citations.MergeSources(toCitationSources(existing), toCitationSources(incoming))
	return fromCitationSources(merged)
}

// extractWebSearchCitationsFromToolOutput delegates to the library,
// injecting the connector-level tool name and normalizer.
func extractWebSearchCitationsFromToolOutput(toolName, output string) []sourceCitation {
	results := citations.ExtractWebSearchCitations(toolName, output, ToolNameWebSearch, normalizeToolAlias)
	return fromCitationSources(results)
}

func toCitationSources(values []sourceCitation) []citations.Source {
	if len(values) == 0 {
		return nil
	}
	out := make([]citations.Source, 0, len(values))
	for _, value := range values {
		out = append(out, citations.Source(value))
	}
	return out
}

func fromCitationSources(values []citations.Source) []sourceCitation {
	if len(values) == 0 {
		return nil
	}
	out := make([]sourceCitation, 0, len(values))
	for _, value := range values {
		out = append(out, sourceCitation(value))
	}
	return out
}

package connector

import "github.com/beeper/ai-bridge/pkg/matrixai/citations"

// Type aliases for the library citation types.
type sourceCitation = citations.Source
type sourceDocument = citations.Document

// Re-export citation functions from the library package.
var (
	extractURLCitation       = citations.ExtractURLCitation
	extractDocumentCitation  = citations.ExtractDocumentCitation
	mergeSourceCitations     = citations.MergeSources
	citationProviderMetadata = citations.ProviderMetadata
)

// extractWebSearchCitationsFromToolOutput delegates to the library,
// injecting the connector-level tool name and normalizer.
func extractWebSearchCitationsFromToolOutput(toolName, output string) []sourceCitation {
	return citations.ExtractWebSearchCitations(toolName, output, ToolNameWebSearch, normalizeToolAlias)
}

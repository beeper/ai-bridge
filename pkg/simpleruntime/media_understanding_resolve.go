package connector

import "github.com/beeper/ai-bridge/pkg/core/aimedia"

// Re-export resolve functions from the library.
var (
	resolveMediaTimeoutSeconds = aimedia.ResolveTimeout
	resolveMediaPrompt         = aimedia.ResolvePrompt
	resolveMediaMaxChars       = aimedia.ResolveMaxChars
	resolveMediaMaxBytes       = aimedia.ResolveMaxBytes
	resolveMediaLanguage       = aimedia.ResolveLanguage
	resolveMediaEntries        = aimedia.ResolveEntries
	capabilityInList           = aimedia.CapabilityInList
	capabilityInCapabilities   = aimedia.CapabilityInCapabilities
	providerSupportsCapability = aimedia.ProviderSupportsCapability
)

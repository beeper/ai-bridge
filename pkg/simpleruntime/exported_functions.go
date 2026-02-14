package connector

import (
	"context"

	"github.com/openai/openai-go/v3"

	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"github.com/beeper/ai-bridge/modules/runtime/commandregistry"
)

// ============================================================================
// Exported package-level functions (for downstream bridge re-export)
// ============================================================================

// HumanUserID returns the ghost UserID for a human login.
func HumanUserID(loginID networkid.UserLoginID) networkid.UserID {
	return humanUserID(loginID)
}

// ModelUserID returns the ghost UserID for a model.
func ModelUserID(modelID string) networkid.UserID {
	return modelUserID(modelID)
}

// DefaultChatPortalKey returns the default portal key for a login.
func DefaultChatPortalKey(loginID networkid.UserLoginID) networkid.PortalKey {
	return defaultChatPortalKey(loginID)
}

// ClonePortalMetadata returns a deep copy of portal metadata.
func ClonePortalMetadata(src *PortalMetadata) *PortalMetadata {
	return clonePortalMetadata(src)
}

// ShouldIncludeInHistory reports whether a message should be included in history.
func ShouldIncludeInHistory(meta *MessageMetadata) bool {
	return shouldIncludeInHistory(meta)
}

// ParseModelFromGhostID extracts the model ID from a ghost user ID.
func ParseModelFromGhostID(ghostID string) string {
	return parseModelFromGhostID(ghostID)
}

// JoinProxyPath joins a base URL and suffix path.
func JoinProxyPath(base, suffix string) string {
	return joinProxyPath(base, suffix)
}

// NormalizeMagicProxyBaseURL normalizes the magic proxy base URL.
func NormalizeMagicProxyBaseURL(raw string) string {
	return normalizeMagicProxyBaseURL(raw)
}

// NormalizeMimeString normalizes a MIME type string.
func NormalizeMimeString(value string) string {
	return normalizeMimeString(value)
}

// NormalizeToolAlias resolves tool name aliases.
func NormalizeToolAlias(name string) string {
	return normalizeToolAlias(name)
}

// ModelOverrideFromContext extracts a model override from context.
func ModelOverrideFromContext(ctx context.Context) (string, bool) {
	return modelOverrideFromContext(ctx)
}

// WithModelOverride returns a context with a model override set.
func WithModelOverride(ctx context.Context, model string) context.Context {
	return withModelOverride(ctx, model)
}

// GetModelCapabilities returns the capabilities of a model.
func GetModelCapabilities(modelID string, info *ModelInfo) ModelCapabilities {
	return getModelCapabilities(modelID, info)
}

// ServiceTokensEmpty reports whether service tokens are empty.
func ServiceTokensEmpty(tokens *ServiceTokens) bool {
	return serviceTokensEmpty(tokens)
}

// ShouldFallbackOnError reports whether the error warrants model fallback.
func ShouldFallbackOnError(err error) bool {
	return shouldFallbackOnError(err)
}

// TraceEnabled reports whether tracing is enabled for the portal.
func TraceEnabled(meta *PortalMetadata) bool {
	return traceEnabled(meta)
}

// TraceFull reports whether full tracing is enabled for the portal.
func TraceFull(meta *PortalMetadata) bool {
	return traceFull(meta)
}

// ToolSchemaToMap converts a tool schema to a map.
func ToolSchemaToMap(schema any) map[string]any {
	return toolSchemaToMap(schema)
}

// ResolveSessionStorePath returns the session store path for the config.
func ResolveSessionStorePath(cfg *Config, agentID string) string {
	return resolveSessionStorePath(cfg, agentID)
}

// DedupeChatToolParams deduplicates tool parameters by name.
func DedupeChatToolParams(tools []openai.ChatCompletionToolUnionParam) []openai.ChatCompletionToolUnionParam {
	return dedupeChatToolParams(tools)
}

// EstimateMessageChars estimates the character count of a message.
func EstimateMessageChars(msg openai.ChatCompletionMessageParamUnion) int {
	return estimateMessageChars(msg)
}

// SmartTruncatePrompt truncates a prompt to reduce tokens.
func SmartTruncatePrompt(
	prompt []openai.ChatCompletionMessageParamUnion,
	targetReduction float64,
) []openai.ChatCompletionMessageParamUnion {
	return smartTruncatePrompt(prompt, targetReduction)
}

// RegisterAICommand registers a command definition and returns the handler.
func RegisterAICommand(def commandregistry.Definition) *commands.FullHandler {
	return registerAICommand(def)
}

// RequireClient extracts the AIClient from a command event.
func RequireClient(ce *commands.Event) (*AIClient, bool) {
	return requireClient(ce)
}

// RequireClientMeta extracts the AIClient and PortalMetadata from a command event.
func RequireClientMeta(ce *commands.Event) (*AIClient, *PortalMetadata, bool) {
	return requireClientMeta(ce)
}

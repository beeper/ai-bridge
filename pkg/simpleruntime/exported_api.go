package connector

import (
	"context"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// ============================================================================
// AIClient field accessors
// ============================================================================

// API returns the underlying OpenAI client.
func (oc *AIClient) API() openai.Client { return oc.api }

// SetAPI replaces the underlying OpenAI client.
func (oc *AIClient) SetAPI(c openai.Client) { oc.api = c }

// APIKey returns the API key used by this client.
func (oc *AIClient) APIKey() string { return oc.apiKey }

// Logger returns the logger for this client.
func (oc *AIClient) Logger() *zerolog.Logger { return &oc.log }

// Provider returns the AI provider for this client.
func (oc *AIClient) Provider() AIProvider { return oc.provider }

// SetProvider replaces the AI provider for this client.
func (oc *AIClient) SetProvider(p AIProvider) { oc.provider = p }

// SetStreamingHooks replaces the streaming hooks for this client.
func (oc *AIClient) SetStreamingHooks(h StreamingHooks) { oc.streamingHooks = h }

// GetStreamingHooks returns the streaming hooks for this client.
func (oc *AIClient) GetStreamingHooks() StreamingHooks { return oc.streamingHooks }


// ============================================================================
// AIClient exported methods (used by downstream bridges via struct embedding)
// ============================================================================

// SendSystemNotice sends a system notice to a portal.
func (oc *AIClient) SendSystemNotice(ctx context.Context, portal *bridgev2.Portal, text string) {
	oc.sendSystemNotice(ctx, portal, text)
}

// LoggerForContext returns the logger enriched with context metadata.
func (oc *AIClient) LoggerForContext(ctx context.Context) *zerolog.Logger {
	return oc.loggerForContext(ctx)
}

// BackgroundContext returns a context that survives request cancellation.
func (oc *AIClient) BackgroundContext(ctx context.Context) context.Context {
	return oc.backgroundContext(ctx)
}

// EffectiveModel returns the effective model ID for a portal.
func (oc *AIClient) EffectiveModel(meta *PortalMetadata) string {
	return oc.effectiveModel(meta)
}


// ResolveModelID validates and resolves a model ID.
func (oc *AIClient) ResolveModelID(ctx context.Context, modelID string) (string, bool, error) {
	return oc.resolveModelID(ctx, modelID)
}

// ResolveUserTimezone returns the user's timezone name and location.
func (oc *AIClient) ResolveUserTimezone() (string, *time.Location) {
	return oc.resolveUserTimezone()
}

// ResolveVisionModelForImage returns the vision model for image understanding.
func (oc *AIClient) ResolveVisionModelForImage(ctx context.Context, meta *PortalMetadata) (string, bool) {
	return oc.resolveVisionModelForImage(ctx, meta)
}

// FindModelInfo looks up model information by ID.
func (oc *AIClient) FindModelInfo(modelID string) *ModelInfo {
	return oc.findModelInfo(modelID)
}

// GetModelContextWindow returns the context window size for the portal's model.
func (oc *AIClient) GetModelContextWindow(meta *PortalMetadata) int {
	return oc.getModelContextWindow(meta)
}

// ModelFallbackChain returns the fallback model chain for a portal.
func (oc *AIClient) ModelFallbackChain(ctx context.Context, meta *PortalMetadata) []string {
	return oc.modelFallbackChain(ctx, meta)
}

// OverrideModel returns portal metadata with the model overridden.
func (oc *AIClient) OverrideModel(meta *PortalMetadata, modelID string) *PortalMetadata {
	return oc.overrideModel(meta, modelID)
}

// ImplicitModelCatalogEntries returns implicit model catalog entries.
func (oc *AIClient) ImplicitModelCatalogEntries(meta *UserLoginMetadata) []ModelCatalogEntry {
	return oc.implicitModelCatalogEntries(meta)
}

// CanUseImageGeneration checks if image generation is available.
func (oc *AIClient) CanUseImageGeneration() bool {
	return oc.canUseImageGeneration()
}

// EnsureModelInRoom ensures the AI ghost is present in the portal room.
func (oc *AIClient) EnsureModelInRoom(ctx context.Context, portal *bridgev2.Portal) error {
	return oc.ensureModelInRoom(ctx, portal)
}

// GetModelIntent returns the Matrix API for the model ghost.
func (oc *AIClient) GetModelIntent(ctx context.Context, portal *bridgev2.Portal) bridgev2.MatrixAPI {
	return oc.getModelIntent(ctx, portal)
}

// CancelRoomRun cancels the active room run for a room.
func (oc *AIClient) CancelRoomRun(roomID id.RoomID) bool {
	return oc.cancelRoomRun(roomID)
}

// ClearPendingQueue clears the pending message queue for a room.
func (oc *AIClient) ClearPendingQueue(roomID id.RoomID) {
	oc.clearPendingQueue(roomID)
}

// SavePortalQuiet saves a portal without sending Matrix events.
func (oc *AIClient) SavePortalQuiet(ctx context.Context, portal *bridgev2.Portal, action string) {
	oc.savePortalQuiet(ctx, portal, action)
}

// SendWelcomeMessage sends the welcome message to a new chat.
func (oc *AIClient) SendWelcomeMessage(ctx context.Context, portal *bridgev2.Portal) {
	oc.sendWelcomeMessage(ctx, portal)
}

// SetRoomNameNoSave sets the room name without persisting.
func (oc *AIClient) SetRoomNameNoSave(ctx context.Context, portal *bridgev2.Portal, name string) error {
	return oc.setRoomNameNoSave(ctx, portal, name)
}

// SendPlainAssistantMessage sends a plain text assistant message.
func (oc *AIClient) SendPlainAssistantMessage(ctx context.Context, portal *bridgev2.Portal, text string) error {
	return oc.sendPlainAssistantMessageWithResult(ctx, portal, text)
}

// ExecuteBuiltinTool executes a named builtin tool.
func (oc *AIClient) ExecuteBuiltinTool(ctx context.Context, portal *bridgev2.Portal, toolName string, argsJSON string) (string, error) {
	return oc.executeBuiltinTool(ctx, portal, toolName, argsJSON)
}

// ListAllChatPortals lists all chat portals for this user.
func (oc *AIClient) ListAllChatPortals(ctx context.Context) ([]*bridgev2.Portal, error) {
	return oc.listAllChatPortals(ctx)
}

// BuildPrompt builds the complete prompt for a portal conversation.
func (oc *AIClient) BuildPrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, latest string, eventID id.EventID) ([]openai.ChatCompletionMessageParamUnion, error) {
	return oc.buildPrompt(ctx, portal, meta, latest, eventID)
}


// InitPortalForChat initializes a portal for a new chat.
func (oc *AIClient) InitPortalForChat(ctx context.Context, opts PortalInitOpts) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	return oc.initPortalForChat(ctx, opts)
}

// MarkMessageSendSuccess marks a streaming message as sent.
func (oc *AIClient) MarkMessageSendSuccess(ctx context.Context, portal *bridgev2.Portal, evt *event.Event) {
	oc.markMessageSendSuccess(ctx, portal, evt, nil)
}

// DisconnectCtx returns the per-login cancellation context.
func (oc *AIClient) DisconnectCtx() context.Context { return oc.disconnectCtx }

// SetDisconnectCtx sets the per-login cancellation context and cancel func.
func (oc *AIClient) SetDisconnectCtx(ctx context.Context, cancel context.CancelFunc) {
	oc.disconnectCtx = ctx
	oc.disconnectCancel = cancel
}

// ClearActiveRoomsAndQueues clears in-flight rooms and pending queues (for logout).
func (oc *AIClient) ClearActiveRoomsAndQueues() {
	oc.activeRoomsMu.Lock()
	clear(oc.activeRooms)
	oc.activeRoomsMu.Unlock()

	oc.pendingQueuesMu.Lock()
	clear(oc.pendingQueues)
	oc.pendingQueuesMu.Unlock()
}

// ResolveServiceConfig returns the service config for the current login.
func (oc *AIClient) ResolveServiceConfig() ServiceConfigMap {
	return oc.connector.resolveServiceConfig(loginMetadata(oc.UserLogin))
}

// BuildBasePrompt builds only the base prompt (system + history) for a portal.
func (oc *AIClient) BuildBasePrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata) ([]openai.ChatCompletionMessageParamUnion, error) {
	return oc.buildBasePrompt(ctx, portal, meta)
}

// DispatchInternalMessage dispatches an internal message to a portal.
func (oc *AIClient) DispatchInternalMessage(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, body, source string, excludeFromHistory bool) (id.EventID, bool, error) {
	return oc.dispatchInternalMessage(ctx, portal, meta, body, source, excludeFromHistory)
}

// DownloadAndEncodeMedia downloads an mxc URL and returns base64 + MIME type.
func (oc *AIClient) DownloadAndEncodeMedia(ctx context.Context, mxcURL string, encryptedFile *event.EncryptedFileInfo, maxSizeMB int) (string, string, error) {
	return oc.downloadAndEncodeMedia(ctx, mxcURL, encryptedFile, maxSizeMB)
}

// HasQueuedItems checks if a room has any pending queued items.
func (oc *AIClient) HasQueuedItems(roomID id.RoomID) (hasItems bool, droppedCount int) {
	q := oc.getQueueSnapshot(roomID)
	if q == nil {
		return false, 0
	}
	return len(q.items) > 0 || q.droppedCount > 0, q.droppedCount
}

// ListAvailableModels lists all available models.
func (oc *AIClient) ListAvailableModels(ctx context.Context, forceRefresh bool) ([]ModelInfo, error) {
	return oc.listAvailableModels(ctx, forceRefresh)
}

// ModelIDForAPI returns the model ID suitable for API calls.
func (oc *AIClient) ModelIDForAPI(modelID string) string {
	return oc.modelIDForAPI(modelID)
}

// ResponseFunc is the exported alias for the response handler type.
type ResponseFunc = responseFunc

// SelectResponseFn selects the appropriate response function and log label.
func (oc *AIClient) SelectResponseFn(meta *PortalMetadata, prompt []openai.ChatCompletionMessageParamUnion) (ResponseFunc, string) {
	return oc.selectResponseFn(meta, prompt)
}

// ResponseWithRetryAndReasoningFallback runs response with retry and reasoning fallback.
func (oc *AIClient) ResponseWithRetryAndReasoningFallback(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
	responseFn ResponseFunc,
	logLabel string,
) (bool, error) {
	return oc.responseWithRetryAndReasoningFallback(ctx, evt, portal, meta, prompt, responseFn, logLabel)
}

// StreamingResponseWithRetry runs a streaming response with retry logic.
func (oc *AIClient) StreamingResponseWithRetry(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	oc.streamingResponseWithRetry(ctx, evt, portal, meta, prompt)
}

// SendPlainAssistantMessageWithResult sends a plain assistant message and returns error.
func (oc *AIClient) SendPlainAssistantMessageWithResult(ctx context.Context, portal *bridgev2.Portal, text string) error {
	return oc.sendPlainAssistantMessageWithResult(ctx, portal, text)
}

// SetRoomSystemPrompt sets the room's system prompt and saves.
func (oc *AIClient) SetRoomSystemPrompt(ctx context.Context, portal *bridgev2.Portal, prompt string) error {
	return oc.setRoomSystemPrompt(ctx, portal, prompt)
}

// SetRoomSystemPromptNoSave sets the room's system prompt without saving.
func (oc *AIClient) SetRoomSystemPromptNoSave(ctx context.Context, portal *bridgev2.Portal, prompt string) error {
	return oc.setRoomSystemPromptNoSave(ctx, portal, prompt)
}

// SetRoomTopic sets the room topic.
func (oc *AIClient) SetRoomTopic(ctx context.Context, portal *bridgev2.Portal, topic string) error {
	return oc.setRoomTopic(ctx, portal, topic)
}

// SetRoomName sets the room name (with save).
func (oc *AIClient) SetRoomName(ctx context.Context, portal *bridgev2.Portal, name string) error {
	return oc.setRoomName(ctx, portal, name)
}


// ToolExecutor is the exported alias for the builtin tool executor signature.
type ToolExecutor = toolExecutor

// BuildPromptWithLinkContext builds a prompt with link preview context.
func (oc *AIClient) BuildPromptWithLinkContext(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	latest string,
	rawEventContent map[string]any,
	eventID id.EventID,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	return oc.buildPromptWithLinkContext(ctx, portal, meta, latest, rawEventContent, eventID)
}

// BuildMatrixInboundBody builds the message body for an inbound Matrix event.
func (oc *AIClient) BuildMatrixInboundBody(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	evt *event.Event,
	rawBody string,
	senderName string,
	roomName string,
	isGroup bool,
) string {
	return oc.buildMatrixInboundBody(ctx, portal, meta, evt, rawBody, senderName, roomName, isGroup)
}

// ============================================================================
// OpenAIConnector exported resolver methods
// ============================================================================

// ApplyRuntimeDefaults applies default configuration values.
func (oc *OpenAIConnector) ApplyRuntimeDefaults() { oc.applyRuntimeDefaults() }

// Policy returns the BridgePolicy for this connector.
func (oc *OpenAIConnector) Policy() BridgePolicy { return oc.policy }

// ResolveProviderAPIKey returns the API key for the given login metadata.
func (oc *OpenAIConnector) ResolveProviderAPIKey(meta *UserLoginMetadata) string {
	return oc.resolveProviderAPIKey(meta)
}

// ResolveServiceConfig returns the service configuration map for the given login metadata.
func (oc *OpenAIConnector) ResolveServiceConfig(meta *UserLoginMetadata) ServiceConfigMap {
	return oc.resolveServiceConfig(meta)
}

// ResolveBeeperBaseURL returns the Beeper base URL for the given login metadata.
func (oc *OpenAIConnector) ResolveBeeperBaseURL(meta *UserLoginMetadata) string {
	return oc.resolveBeeperBaseURL(meta)
}

// ResolveBeeperToken returns the Beeper token for the given login metadata.
func (oc *OpenAIConnector) ResolveBeeperToken(meta *UserLoginMetadata) string {
	return oc.resolveBeeperToken(meta)
}

// ResolveOpenAIAPIKey returns the OpenAI API key for the given login metadata.
func (oc *OpenAIConnector) ResolveOpenAIAPIKey(meta *UserLoginMetadata) string {
	return oc.resolveOpenAIAPIKey(meta)
}

// ResolveOpenAIBaseURL returns the OpenAI base URL.
func (oc *OpenAIConnector) ResolveOpenAIBaseURL() string {
	return oc.resolveOpenAIBaseURL()
}

// ResolveOpenRouterAPIKey returns the OpenRouter API key for the given login metadata.
func (oc *OpenAIConnector) ResolveOpenRouterAPIKey(meta *UserLoginMetadata) string {
	return oc.resolveOpenRouterAPIKey(meta)
}

// ResolveProxyRoot returns the proxy root URL for the given login metadata.
func (oc *OpenAIConnector) ResolveProxyRoot(meta *UserLoginMetadata) string {
	return oc.resolveProxyRoot(meta)
}

// ResolveExaProxyBaseURL returns the Exa proxy base URL for the given login metadata.
func (oc *OpenAIConnector) ResolveExaProxyBaseURL(meta *UserLoginMetadata) string {
	return oc.resolveExaProxyBaseURL(meta)
}

// ============================================================================
// Test-accessible constants and helpers
// ============================================================================

// DefaultRawModeSystemPrompt is the default system prompt for model-only (raw) rooms.
const DefaultRawModeSystemPrompt = defaultRawModeSystemPrompt

// BuildSessionIdentityHint builds the session identity hint from portal and metadata.
var BuildSessionIdentityHint = buildSessionIdentityHint

// SessionGreetingPrompt is the greeting prompt for new sessions.
const SessionGreetingPrompt = sessionGreetingPrompt

// MaybePrependSessionGreeting prepends a session greeting if applicable.
var MaybePrependSessionGreeting = maybePrependSessionGreeting

// FormatHeartbeatSummary formats a heartbeat summary string.
var FormatHeartbeatSummary = formatHeartbeatSummary

// ============================================================================
// Exported helper functions
// ============================================================================

// PortalMeta extracts the PortalMetadata from a bridgev2.Portal.
func PortalMeta(portal *bridgev2.Portal) *PortalMetadata {
	return portalMeta(portal)
}

// LoginMeta extracts the UserLoginMetadata from a bridgev2.UserLogin.
func LoginMeta(login *bridgev2.UserLogin) *UserLoginMetadata {
	return loginMetadata(login)
}

// MessageMeta extracts the MessageMetadata from a database.Message.
func MessageMeta(msg *database.Message) *MessageMetadata {
	return messageMeta(msg)
}

// ============================================================================
// Client factory
// ============================================================================

// ClientFactory creates a bridgev2.NetworkAPI for a login.
// Downstream bridges can use this to create their own client types.
type ClientFactory func(login *bridgev2.UserLogin, connector *OpenAIConnector, apiKey string) (bridgev2.NetworkAPI, error)

// SetClientFactory sets a custom client factory on the connector.
// When set, LoadUserLogin will use this factory instead of the default newAIClient.
func (oc *OpenAIConnector) SetClientFactory(f ClientFactory) { oc.clientFactory = f }

// NewAIClient creates a new base AIClient. Exported for use by downstream bridges
// that need to create a base client before wrapping it.
func NewAIClient(login *bridgev2.UserLogin, connector *OpenAIConnector, apiKey string) (*AIClient, error) {
	return newAIClient(login, connector, apiKey)
}

// GetClients returns a snapshot of all registered clients.
func (oc *OpenAIConnector) GetClients() map[networkid.UserLoginID]bridgev2.NetworkAPI {
	oc.clientsMu.Lock()
	defer oc.clientsMu.Unlock()
	result := make(map[networkid.UserLoginID]bridgev2.NetworkAPI, len(oc.clients))
	for k, v := range oc.clients {
		result[k] = v
	}
	return result
}

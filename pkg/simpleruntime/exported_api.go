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

// GetHeartbeatRunner returns the heartbeat runner for this client.
func (oc *AIClient) GetHeartbeatRunner() *HeartbeatRunner { return oc.heartbeatRunner }

// SetHeartbeatRunner replaces the heartbeat runner for this client.
func (oc *AIClient) SetHeartbeatRunner(r *HeartbeatRunner) { oc.heartbeatRunner = r }

// GetHeartbeatWake returns the heartbeat wake signal for this client.
func (oc *AIClient) GetHeartbeatWake() *HeartbeatWake { return oc.heartbeatWake }

// SetHeartbeatWake replaces the heartbeat wake signal for this client.
func (oc *AIClient) SetHeartbeatWake(w *HeartbeatWake) { oc.heartbeatWake = w }

// ============================================================================
// AIClient exported method wrappers (for downstream bridge embedding)
// ============================================================================

// ExportedSendSystemNotice sends a system notice to a portal.
func (oc *AIClient) ExportedSendSystemNotice(ctx context.Context, portal *bridgev2.Portal, text string) {
	oc.sendSystemNotice(ctx, portal, text)
}

// ExportedLoggerForContext returns the logger enriched with context metadata.
func (oc *AIClient) ExportedLoggerForContext(ctx context.Context) *zerolog.Logger {
	return oc.loggerForContext(ctx)
}

// ExportedBackgroundContext returns a context that survives request cancellation.
func (oc *AIClient) ExportedBackgroundContext(ctx context.Context) context.Context {
	return oc.backgroundContext(ctx)
}

// ExportedEffectiveModel returns the effective model ID for a portal.
func (oc *AIClient) ExportedEffectiveModel(meta *PortalMetadata) string {
	return oc.effectiveModel(meta)
}

// ExportedResolveModelID validates and resolves a model ID.
func (oc *AIClient) ExportedResolveModelID(ctx context.Context, modelID string) (string, bool, error) {
	return oc.resolveModelID(ctx, modelID)
}

// ExportedResolveUserTimezone returns the user's timezone name and location.
func (oc *AIClient) ExportedResolveUserTimezone() (string, *time.Location) {
	return oc.resolveUserTimezone()
}

// ExportedResolveVisionModelForImage returns the vision model for image understanding.
func (oc *AIClient) ExportedResolveVisionModelForImage(ctx context.Context, meta *PortalMetadata) (string, bool) {
	return oc.resolveVisionModelForImage(ctx, meta)
}

// ExportedFindModelInfo looks up model information by ID.
func (oc *AIClient) ExportedFindModelInfo(modelID string) *ModelInfo {
	return oc.findModelInfo(modelID)
}

// ExportedGetModelContextWindow returns the context window size for the portal's model.
func (oc *AIClient) ExportedGetModelContextWindow(meta *PortalMetadata) int {
	return oc.getModelContextWindow(meta)
}

// ExportedModelFallbackChain returns the fallback model chain for a portal.
func (oc *AIClient) ExportedModelFallbackChain(ctx context.Context, meta *PortalMetadata) []string {
	return oc.modelFallbackChain(ctx, meta)
}

// ExportedOverrideModel returns portal metadata with the model overridden.
func (oc *AIClient) ExportedOverrideModel(meta *PortalMetadata, modelID string) *PortalMetadata {
	return oc.overrideModel(meta, modelID)
}

// ExportedImplicitModelCatalogEntries returns implicit model catalog entries.
func (oc *AIClient) ExportedImplicitModelCatalogEntries(meta *UserLoginMetadata) []ModelCatalogEntry {
	return oc.implicitModelCatalogEntries(meta)
}

// ExportedCanUseImageGeneration checks if image generation is available.
func (oc *AIClient) ExportedCanUseImageGeneration() bool {
	return oc.canUseImageGeneration()
}

// ExportedBridgeStateBackend returns the state store backend.
func (oc *AIClient) ExportedBridgeStateBackend() StateStoreBackend {
	return oc.bridgeStateBackend()
}

// ExportedEnsureModelInRoom ensures the AI ghost is present in the portal room.
func (oc *AIClient) ExportedEnsureModelInRoom(ctx context.Context, portal *bridgev2.Portal) error {
	return oc.ensureModelInRoom(ctx, portal)
}

// ExportedGetModelIntent returns the Matrix API for the model ghost.
func (oc *AIClient) ExportedGetModelIntent(ctx context.Context, portal *bridgev2.Portal) bridgev2.MatrixAPI {
	return oc.getModelIntent(ctx, portal)
}

// ExportedCancelRoomRun cancels the active room run for a room.
func (oc *AIClient) ExportedCancelRoomRun(roomID id.RoomID) bool {
	return oc.cancelRoomRun(roomID)
}

// ExportedClearPendingQueue clears the pending message queue for a room.
func (oc *AIClient) ExportedClearPendingQueue(roomID id.RoomID) {
	oc.clearPendingQueue(roomID)
}

// ExportedEmitHeartbeatEvent persists a heartbeat event payload.
func (oc *AIClient) ExportedEmitHeartbeatEvent(evt *HeartbeatEventPayload) {
	oc.emitHeartbeatEvent(evt)
}

// ExportedSavePortalQuiet saves a portal without sending Matrix events.
func (oc *AIClient) ExportedSavePortalQuiet(ctx context.Context, portal *bridgev2.Portal, action string) {
	oc.savePortalQuiet(ctx, portal, action)
}

// ExportedSendWelcomeMessage sends the welcome message to a new chat.
func (oc *AIClient) ExportedSendWelcomeMessage(ctx context.Context, portal *bridgev2.Portal) {
	oc.sendWelcomeMessage(ctx, portal)
}

// ExportedSetRoomNameNoSave sets the room name without persisting.
func (oc *AIClient) ExportedSetRoomNameNoSave(ctx context.Context, portal *bridgev2.Portal, name string) error {
	return oc.setRoomNameNoSave(ctx, portal, name)
}

// ExportedSendPlainAssistantMessage sends a plain text assistant message.
func (oc *AIClient) ExportedSendPlainAssistantMessage(ctx context.Context, portal *bridgev2.Portal, text string) error {
	return oc.sendPlainAssistantMessageWithResult(ctx, portal, text)
}

// ExportedExecuteBuiltinTool executes a named builtin tool.
func (oc *AIClient) ExportedExecuteBuiltinTool(ctx context.Context, portal *bridgev2.Portal, toolName string, argsJSON string) (string, error) {
	return oc.executeBuiltinTool(ctx, portal, toolName, argsJSON)
}

// ExportedListAllChatPortals lists all chat portals for this user.
func (oc *AIClient) ExportedListAllChatPortals(ctx context.Context) ([]*bridgev2.Portal, error) {
	return oc.listAllChatPortals(ctx)
}

// ExportedBuildPrompt builds the complete prompt for a portal conversation.
func (oc *AIClient) ExportedBuildPrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, latest string, eventID id.EventID) ([]openai.ChatCompletionMessageParamUnion, error) {
	return oc.buildPrompt(ctx, portal, meta, latest, eventID)
}

// ExportedInitPortalForChat initializes a portal for a new chat.
func (oc *AIClient) ExportedInitPortalForChat(ctx context.Context, opts PortalInitOpts) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	return oc.initPortalForChat(ctx, opts)
}

// ExportedMarkMessageSendSuccess marks a streaming message as sent.
func (oc *AIClient) ExportedMarkMessageSendSuccess(ctx context.Context, portal *bridgev2.Portal, evt *event.Event) {
	// streamingState cannot be passed from outside, so we call with nil state.
	// This exported variant is for simple cases where streaming state is not needed.
	oc.markMessageSendSuccess(ctx, portal, evt, nil)
}

// DisconnectCtx returns the per-login cancellation context.
func (oc *AIClient) DisconnectCtx() context.Context { return oc.disconnectCtx }

// SetDisconnectCtx sets the per-login cancellation context and cancel func.
func (oc *AIClient) SetDisconnectCtx(ctx context.Context, cancel context.CancelFunc) {
	oc.disconnectCtx = ctx
	oc.disconnectCancel = cancel
}

// ExportedClearActiveRoomsAndQueues clears in-flight rooms and pending queues (for logout).
func (oc *AIClient) ExportedClearActiveRoomsAndQueues() {
	oc.activeRoomsMu.Lock()
	clear(oc.activeRooms)
	oc.activeRoomsMu.Unlock()

	oc.pendingQueuesMu.Lock()
	clear(oc.pendingQueues)
	oc.pendingQueuesMu.Unlock()
}

// ExportedResolveServiceConfig returns the service config for the current login.
func (oc *AIClient) ExportedResolveServiceConfig() ServiceConfigMap {
	return oc.connector.resolveServiceConfig(loginMetadata(oc.UserLogin))
}

// ExportedBuildBasePrompt builds only the base prompt (system + history) for a portal.
func (oc *AIClient) ExportedBuildBasePrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata) ([]openai.ChatCompletionMessageParamUnion, error) {
	return oc.buildBasePrompt(ctx, portal, meta)
}

// ExportedDispatchInternalMessage dispatches an internal message to a portal.
func (oc *AIClient) ExportedDispatchInternalMessage(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, body, source string, excludeFromHistory bool) (id.EventID, bool, error) {
	return oc.dispatchInternalMessage(ctx, portal, meta, body, source, excludeFromHistory)
}

// ExportedDownloadAndEncodeMedia downloads an mxc URL and returns base64 + MIME type.
func (oc *AIClient) ExportedDownloadAndEncodeMedia(ctx context.Context, mxcURL string, encryptedFile *event.EncryptedFileInfo, maxSizeMB int) (string, string, error) {
	return oc.downloadAndEncodeMedia(ctx, mxcURL, encryptedFile, maxSizeMB)
}

// ExportedHasQueuedItems checks if a room has any pending queued items.
func (oc *AIClient) ExportedHasQueuedItems(roomID id.RoomID) (hasItems bool, droppedCount int) {
	q := oc.getQueueSnapshot(roomID)
	if q == nil {
		return false, 0
	}
	return len(q.items) > 0 || q.droppedCount > 0, q.droppedCount
}

// ExportedListAvailableModels lists all available models.
func (oc *AIClient) ExportedListAvailableModels(ctx context.Context, forceRefresh bool) ([]ModelInfo, error) {
	return oc.listAvailableModels(ctx, forceRefresh)
}

// ExportedModelIDForAPI returns the model ID suitable for API calls.
func (oc *AIClient) ExportedModelIDForAPI(modelID string) string {
	return oc.modelIDForAPI(modelID)
}

// ResponseFunc is the exported alias for the response handler type.
type ResponseFunc = responseFunc

// ExportedSelectResponseFn selects the appropriate response function and log label.
func (oc *AIClient) ExportedSelectResponseFn(meta *PortalMetadata, prompt []openai.ChatCompletionMessageParamUnion) (ResponseFunc, string) {
	return oc.selectResponseFn(meta, prompt)
}

// ExportedResponseWithRetryAndReasoningFallback runs response with retry and reasoning fallback.
func (oc *AIClient) ExportedResponseWithRetryAndReasoningFallback(
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

// ExportedStreamingResponseWithRetry runs a streaming response with retry logic.
func (oc *AIClient) ExportedStreamingResponseWithRetry(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	oc.streamingResponseWithRetry(ctx, evt, portal, meta, prompt)
}

// ExportedSendPlainAssistantMessageWithResult sends a plain assistant message and returns error.
func (oc *AIClient) ExportedSendPlainAssistantMessageWithResult(ctx context.Context, portal *bridgev2.Portal, text string) error {
	return oc.sendPlainAssistantMessageWithResult(ctx, portal, text)
}

// ExportedSetRoomSystemPrompt sets the room's system prompt and saves.
func (oc *AIClient) ExportedSetRoomSystemPrompt(ctx context.Context, portal *bridgev2.Portal, prompt string) error {
	return oc.setRoomSystemPrompt(ctx, portal, prompt)
}

// ExportedSetRoomSystemPromptNoSave sets the room's system prompt without saving.
func (oc *AIClient) ExportedSetRoomSystemPromptNoSave(ctx context.Context, portal *bridgev2.Portal, prompt string) error {
	return oc.setRoomSystemPromptNoSave(ctx, portal, prompt)
}

// ExportedSetRoomTopic sets the room topic.
func (oc *AIClient) ExportedSetRoomTopic(ctx context.Context, portal *bridgev2.Portal, topic string) error {
	return oc.setRoomTopic(ctx, portal, topic)
}

// ExportedSetRoomName sets the room name (with save).
func (oc *AIClient) ExportedSetRoomName(ctx context.Context, portal *bridgev2.Portal, name string) error {
	return oc.setRoomName(ctx, portal, name)
}

// ToolExecutor is the exported alias for the builtin tool executor signature.
type ToolExecutor = toolExecutor

// ============================================================================
// OpenAIConnector exported resolver methods
// ============================================================================

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

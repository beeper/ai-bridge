package connector

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/rs/zerolog"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	_ bridgev2.NetworkAPI                    = (*AIClient)(nil)
	_ bridgev2.IdentifierResolvingNetworkAPI = (*AIClient)(nil)
	_ bridgev2.ContactListingNetworkAPI      = (*AIClient)(nil)
	_ bridgev2.EditHandlingNetworkAPI        = (*AIClient)(nil)
)

var rejectAllMediaFileFeatures = &event.FileFeatures{
	MimeTypes: map[string]event.CapabilitySupportLevel{
		"*/*": event.CapLevelRejected,
	},
	Caption: event.CapLevelRejected,
}

func cloneRejectAllMediaFeatures() *event.FileFeatures {
	return rejectAllMediaFileFeatures.Clone()
}

// AI bridge capability constants
const (
	AIMaxTextLength        = 100000
	AIEditMaxAge           = 24 * time.Hour
	modelValidationTimeout = 5 * time.Second
)

func aiCapID() string {
	return "com.beeper.ai.capabilities.2025_01_31"
}

// aiBaseCaps defines the base capabilities for AI chat rooms
var aiBaseCaps = &event.RoomFeatures{
	ID: aiCapID(),
	Formatting: map[event.FormattingFeature]event.CapabilitySupportLevel{
		event.FmtBold:          event.CapLevelFullySupported,
		event.FmtItalic:        event.CapLevelFullySupported,
		event.FmtStrikethrough: event.CapLevelFullySupported,
		event.FmtInlineCode:    event.CapLevelFullySupported,
		event.FmtCodeBlock:     event.CapLevelFullySupported,
		event.FmtBlockquote:    event.CapLevelFullySupported,
		event.FmtUnorderedList: event.CapLevelFullySupported,
		event.FmtOrderedList:   event.CapLevelFullySupported,
		event.FmtInlineLink:    event.CapLevelFullySupported,
	},
	File: event.FileFeatureMap{
		event.MsgVideo:      cloneRejectAllMediaFeatures(),
		event.MsgAudio:      cloneRejectAllMediaFeatures(),
		event.MsgFile:       cloneRejectAllMediaFeatures(),
		event.CapMsgVoice:   cloneRejectAllMediaFeatures(),
		event.CapMsgGIF:     cloneRejectAllMediaFeatures(),
		event.CapMsgSticker: cloneRejectAllMediaFeatures(),
		event.MsgImage:      cloneRejectAllMediaFeatures(),
	},
	MaxTextLength:       AIMaxTextLength,
	Reply:               event.CapLevelFullySupported,
	Thread:              event.CapLevelFullySupported,
	Edit:                event.CapLevelFullySupported,
	EditMaxCount:        10,
	EditMaxAge:          ptr.Ptr(jsontime.S(AIEditMaxAge)),
	Delete:              event.CapLevelPartialSupport,
	DeleteMaxAge:        ptr.Ptr(jsontime.S(24 * time.Hour)),
	Reaction:            event.CapLevelRejected,
	ReadReceipts:        true,
	TypingNotifications: true,
}

// buildCapabilityID constructs a deterministic capability ID based on model modalities.
// Suffixes are sorted alphabetically to ensure the same capabilities produce the same ID.
func buildCapabilityID(caps ModelCapabilities) string {
	var suffixes []string

	// Add suffixes in alphabetical order for determinism
	if caps.SupportsAudio {
		suffixes = append(suffixes, "audio")
	}
	if caps.SupportsImageGen {
		suffixes = append(suffixes, "imagegen")
	}
	if caps.SupportsPDF {
		suffixes = append(suffixes, "pdf")
	}
	if caps.SupportsVideo {
		suffixes = append(suffixes, "video")
	}
	if caps.SupportsVision {
		suffixes = append(suffixes, "vision")
	}

	if len(suffixes) == 0 {
		return aiCapID()
	}
	return aiCapID() + "+" + strings.Join(suffixes, "+")
}

// visionFileFeatures returns FileFeatures for vision-capable models
func visionFileFeatures() *event.FileFeatures {
	return &event.FileFeatures{
		MimeTypes: map[string]event.CapabilitySupportLevel{
			"image/png":  event.CapLevelFullySupported,
			"image/jpeg": event.CapLevelFullySupported,
			"image/webp": event.CapLevelFullySupported,
			"image/gif":  event.CapLevelFullySupported,
		},
		Caption:          event.CapLevelFullySupported,
		MaxCaptionLength: AIMaxTextLength,
		MaxSize:          20 * 1024 * 1024, // 20MB
	}
}

// pdfFileFeatures returns FileFeatures for PDF-capable models
func pdfFileFeatures() *event.FileFeatures {
	return &event.FileFeatures{
		MimeTypes: map[string]event.CapabilitySupportLevel{
			"application/pdf": event.CapLevelFullySupported,
		},
		Caption:          event.CapLevelFullySupported,
		MaxCaptionLength: AIMaxTextLength,
		MaxSize:          50 * 1024 * 1024, // 50MB for PDFs
	}
}

// audioFileFeatures returns FileFeatures for audio-capable models
func audioFileFeatures() *event.FileFeatures {
	return &event.FileFeatures{
		MimeTypes: map[string]event.CapabilitySupportLevel{
			"audio/wav":   event.CapLevelFullySupported,
			"audio/mpeg":  event.CapLevelFullySupported, // mp3
			"audio/mp3":   event.CapLevelFullySupported,
			"audio/webm":  event.CapLevelFullySupported,
			"audio/ogg":   event.CapLevelFullySupported,
			"audio/flac":  event.CapLevelFullySupported,
			"audio/mp4":   event.CapLevelFullySupported, // m4a
			"audio/x-m4a": event.CapLevelFullySupported,
		},
		Caption:          event.CapLevelFullySupported,
		MaxCaptionLength: AIMaxTextLength,
		MaxSize:          25 * 1024 * 1024, // 25MB for audio
	}
}

// videoFileFeatures returns FileFeatures for video-capable models
func videoFileFeatures() *event.FileFeatures {
	return &event.FileFeatures{
		MimeTypes: map[string]event.CapabilitySupportLevel{
			"video/mp4":       event.CapLevelFullySupported,
			"video/webm":      event.CapLevelFullySupported,
			"video/mpeg":      event.CapLevelFullySupported,
			"video/quicktime": event.CapLevelFullySupported, // mov
			"video/x-msvideo": event.CapLevelFullySupported, // avi
		},
		Caption:          event.CapLevelFullySupported,
		MaxCaptionLength: AIMaxTextLength,
		MaxSize:          100 * 1024 * 1024, // 100MB for video
	}
}

// AIClient handles communication with AI providers
type AIClient struct {
	UserLogin *bridgev2.UserLogin
	connector *OpenAIConnector
	api       openai.Client
	apiKey    string
	log       zerolog.Logger

	// Provider abstraction layer - all providers use OpenAI SDK
	provider AIProvider

	loggedIn atomic.Bool
	chatLock sync.Mutex

	// Turn-based message queuing: only one response per room at a time
	activeRooms   map[id.RoomID]bool
	activeRoomsMu sync.Mutex

	// Pending message queue per room (for turn-based behavior)
	pendingMessages   map[id.RoomID][]pendingMessage
	pendingMessagesMu sync.Mutex
}

// pendingMessageType indicates what kind of pending message this is
type pendingMessageType string

const (
	pendingTypeText           pendingMessageType = "text"
	pendingTypeImage          pendingMessageType = "image"
	pendingTypePDF            pendingMessageType = "pdf"
	pendingTypeAudio          pendingMessageType = "audio"
	pendingTypeVideo          pendingMessageType = "video"
	pendingTypeRegenerate     pendingMessageType = "regenerate"
	pendingTypeEditRegenerate pendingMessageType = "edit_regenerate"
)

// pendingMessage represents a queued message waiting for AI processing
// Prompt is built fresh when processing starts to ensure up-to-date history
type pendingMessage struct {
	Event       *event.Event
	Portal      *bridgev2.Portal
	Meta        *PortalMetadata
	Type        pendingMessageType
	MessageBody string              // For text, regenerate, edit_regenerate (caption for media)
	ImageURL    string              // For image messages
	PDFURL      string              // For PDF messages
	AudioURL    string              // For audio messages
	VideoURL    string              // For video messages
	MimeType    string              // MIME type of the media
	TargetMsgID networkid.MessageID // For edit_regenerate
}

func newAIClient(login *bridgev2.UserLogin, connector *OpenAIConnector, apiKey string) (*AIClient, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("missing API key")
	}

	// Get per-user credentials from login metadata
	meta := login.Metadata.(*UserLoginMetadata)
	log := login.Log.With().Str("component", "ai-network").Str("provider", meta.Provider).Logger()

	// Create base client struct
	oc := &AIClient{
		UserLogin:       login,
		connector:       connector,
		apiKey:          key,
		log:             log,
		activeRooms:     make(map[id.RoomID]bool),
		pendingMessages: make(map[id.RoomID][]pendingMessage),
	}

	// Use per-user base_url if provided
	baseURL := strings.TrimSpace(meta.BaseURL)

	// Initialize provider based on login metadata
	// All providers use the OpenAI SDK with different base URLs
	switch meta.Provider {
	case ProviderBeeper:
		// Beeper mode: routes through Beeper's OpenRouter proxy
		beeperBaseURL := baseURL
		if beeperBaseURL == "" {
			return nil, fmt.Errorf("beeper base_url is required for Beeper provider")
		}

		// Get user ID for rate limiting
		userID := login.User.MXID.String()

		openrouterURL := beeperBaseURL + "/openrouter/v1"
		provider, err := NewOpenAIProviderWithUserID(key, openrouterURL, userID, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create Beeper provider: %w", err)
		}
		oc.provider = provider
		oc.api = provider.Client()

	case ProviderOpenRouter:
		// OpenRouter direct access
		openrouterURL := baseURL
		if openrouterURL == "" {
			openrouterURL = "https://openrouter.ai/api/v1"
		}
		provider, err := NewOpenAIProviderWithBaseURL(key, openrouterURL, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenRouter provider: %w", err)
		}
		oc.provider = provider
		oc.api = provider.Client()

	default:
		// OpenAI (default) or Custom OpenAI-compatible provider
		openaiURL := baseURL
		if openaiURL == "" {
			openaiURL = "https://api.openai.com/v1"
		}
		provider, err := NewOpenAIProviderWithBaseURL(key, openaiURL, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI provider: %w", err)
		}
		oc.provider = provider
		oc.api = provider.Client()
	}

	return oc, nil
}

func (oc *AIClient) acquireRoom(roomID id.RoomID) bool {
	oc.activeRoomsMu.Lock()
	defer oc.activeRoomsMu.Unlock()
	if oc.activeRooms[roomID] {
		return false // already processing
	}
	oc.activeRooms[roomID] = true
	return true
}

// releaseRoom releases a room after processing is complete.
func (oc *AIClient) releaseRoom(roomID id.RoomID) {
	oc.activeRoomsMu.Lock()
	defer oc.activeRoomsMu.Unlock()
	delete(oc.activeRooms, roomID)
}

// queuePendingMessage adds a message to the pending queue for later processing
func (oc *AIClient) queuePendingMessage(roomID id.RoomID, msg pendingMessage) {
	oc.pendingMessagesMu.Lock()
	defer oc.pendingMessagesMu.Unlock()
	oc.pendingMessages[roomID] = append(oc.pendingMessages[roomID], msg)
	oc.log.Debug().
		Str("room_id", roomID.String()).
		Int("queue_length", len(oc.pendingMessages[roomID])).
		Msg("Message queued for later processing")
}

// popNextPending removes and returns the next pending message for a room, or nil if none
func (oc *AIClient) popNextPending(roomID id.RoomID) *pendingMessage {
	oc.pendingMessagesMu.Lock()
	defer oc.pendingMessagesMu.Unlock()
	queue := oc.pendingMessages[roomID]
	if len(queue) == 0 {
		return nil
	}
	msg := queue[0]
	oc.pendingMessages[roomID] = queue[1:]
	if len(oc.pendingMessages[roomID]) == 0 {
		delete(oc.pendingMessages, roomID)
	}
	return &msg
}

// dispatchOrQueue handles the common room acquisition pattern for message processing.
// If the room is available, it dispatches the completion immediately and returns the userMessage for DB.
// If the room is busy, it queues the message and sends a PENDING status.
// Returns (shouldReturnDBMessage, isPending).
func (oc *AIClient) dispatchOrQueue(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	userMessage *database.Message,
	pending pendingMessage,
	promptMessages []openai.ChatCompletionMessageParamUnion,
) (dbMessage *database.Message, isPending bool) {
	if oc.acquireRoom(portal.MXID) {
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, evt, portal, meta, promptMessages)
		}()
		return userMessage, false
	}

	// Room busy - save message ourselves and queue for later
	if userMessage != nil {
		userMessage.MXID = evt.ID
		if err := oc.UserLogin.Bridge.DB.Message.Insert(ctx, userMessage); err != nil {
			oc.log.Err(err).Msg("Failed to save queued message to database")
		}
	}

	oc.queuePendingMessage(portal.MXID, pending)
	oc.sendPendingStatus(ctx, portal, evt, "Waiting for previous response")
	return nil, true
}

// dispatchOrQueueWithStatus is like dispatchOrQueue but sends SUCCESS status when room is acquired.
// Used for regenerate/edit operations where we need to acknowledge the command.
func (oc *AIClient) dispatchOrQueueWithStatus(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	pending pendingMessage,
	promptMessages []openai.ChatCompletionMessageParamUnion,
) {
	if oc.acquireRoom(portal.MXID) {
		oc.sendSuccessStatus(ctx, portal, evt)
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, evt, portal, meta, promptMessages)
		}()
		return
	}

	oc.queuePendingMessage(portal.MXID, pending)
	oc.sendPendingStatus(ctx, portal, evt, "Waiting for previous response")
}

// processNextPending processes the next pending message for a room if one exists
func (oc *AIClient) processNextPending(ctx context.Context, roomID id.RoomID) {
	pending := oc.popNextPending(roomID)
	if pending == nil {
		return
	}

	oc.log.Debug().
		Str("room_id", roomID.String()).
		Str("type", string(pending.Type)).
		Msg("Processing next pending message")

	// Re-acquire the room lock and process
	if !oc.acquireRoom(roomID) {
		// Room somehow got busy again, re-queue the message
		oc.queuePendingMessage(roomID, *pending)
		return
	}

	// Build prompt NOW with fresh history (includes previous AI responses)
	var promptMessages []openai.ChatCompletionMessageParamUnion
	var err error

	switch pending.Type {
	case pendingTypeText:
		promptMessages, err = oc.buildPrompt(ctx, pending.Portal, pending.Meta, pending.MessageBody)
	case pendingTypeImage:
		promptMessages, err = oc.buildPromptWithImage(ctx, pending.Portal, pending.Meta, pending.MessageBody, pending.ImageURL)
	case pendingTypePDF:
		promptMessages, err = oc.buildPromptWithPDF(ctx, pending.Portal, pending.Meta, pending.MessageBody, pending.PDFURL, pending.MimeType)
	case pendingTypeAudio:
		promptMessages, err = oc.buildPromptWithAudio(ctx, pending.Portal, pending.Meta, pending.MessageBody, pending.AudioURL, pending.MimeType)
	case pendingTypeVideo:
		promptMessages, err = oc.buildPromptWithVideo(ctx, pending.Portal, pending.Meta, pending.MessageBody, pending.VideoURL)
	case pendingTypeRegenerate:
		promptMessages, err = oc.buildPromptForRegenerate(ctx, pending.Portal, pending.Meta, pending.MessageBody)
	case pendingTypeEditRegenerate:
		promptMessages, err = oc.buildPromptUpToMessage(ctx, pending.Portal, pending.Meta, pending.TargetMsgID, pending.MessageBody)
	default:
		err = fmt.Errorf("unknown pending message type: %s", pending.Type)
	}

	if err != nil {
		oc.log.Err(err).Str("type", string(pending.Type)).Msg("Failed to build prompt for pending message")
		oc.releaseRoom(roomID)
		oc.processNextPending(oc.backgroundContext(ctx), roomID)
		return
	}

	// Send SUCCESS status synchronously - message is now being processed
	oc.sendSuccessStatus(ctx, pending.Portal, pending.Event)

	// Process in background, will release room when done
	go func() {
		defer func() {
			oc.releaseRoom(roomID)
			// Check for more pending messages
			oc.processNextPending(oc.backgroundContext(ctx), roomID)
		}()
		oc.dispatchCompletionInternal(ctx, pending.Event, pending.Portal, pending.Meta, promptMessages)
	}()
}

func (oc *AIClient) Connect(ctx context.Context) {
	// Trust the token - auth errors will be caught during actual API usage
	// OpenRouter and Beeper provider don't support the GET /v1/models/{model} endpoint
	oc.loggedIn.Store(true)
	oc.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateConnected,
		Message:    "Connected",
	})
}

func (oc *AIClient) Disconnect() {
	oc.loggedIn.Store(false)
}

func (oc *AIClient) IsLoggedIn() bool {
	return oc.loggedIn.Load()
}

func (oc *AIClient) LogoutRemote(ctx context.Context) {
	oc.Disconnect()
	oc.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateLoggedOut,
		Message:    "Disconnected by user",
	})
}

func (oc *AIClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return userID == humanUserID(oc.UserLogin.ID)
}

func (oc *AIClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	meta := portalMeta(portal)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = "ChatGPT"
		}
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: ptrIfNotEmpty(meta.SystemPrompt),
	}, nil
}

func (oc *AIClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	ghostID := string(ghost.ID)

	// Parse model from ghost ID (format: "model-{escaped-model-id}")
	if modelID := parseModelFromGhostID(ghostID); modelID != "" {
		caps := getModelCapabilities(modelID, nil)
		return &bridgev2.UserInfo{
			Name:         ptr.Ptr(FormatModelDisplayWithVision(modelID, caps)),
			IsBot:        ptr.Ptr(false),
			Identifiers:  []string{modelID},
			ExtraUpdates: updateGhostLastSync,
		}, nil
	}

	// Fallback for unknown ghost types
	return &bridgev2.UserInfo{
		Name:  ptr.Ptr("AI Assistant"),
		IsBot: ptr.Ptr(false),
	}, nil
}

// updateGhostLastSync updates the ghost's LastSync timestamp
func updateGhostLastSync(_ context.Context, ghost *bridgev2.Ghost) bool {
	meta, ok := ghost.Metadata.(*GhostMetadata)
	if !ok || meta == nil {
		ghost.Metadata = &GhostMetadata{LastSync: jsontime.U(time.Now())}
		return true
	}
	// Force save if last sync was more than 24 hours ago
	forceSave := time.Since(meta.LastSync.Time) > 24*time.Hour
	meta.LastSync = jsontime.U(time.Now())
	return forceSave
}

func (oc *AIClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	meta := portalMeta(portal)

	// Clone base capabilities
	caps := ptr.Clone(aiBaseCaps)

	// Build dynamic capability ID from modalities
	caps.ID = buildCapabilityID(meta.Capabilities)

	// Apply file capabilities based on modalities
	if meta.Capabilities.SupportsVision {
		caps.File[event.MsgImage] = visionFileFeatures()
	}
	if meta.Capabilities.SupportsPDF {
		caps.File[event.MsgFile] = pdfFileFeatures()
	}
	if meta.Capabilities.SupportsAudio {
		caps.File[event.MsgAudio] = audioFileFeatures()
	}
	if meta.Capabilities.SupportsVideo {
		caps.File[event.MsgVideo] = videoFileFeatures()
	}
	// Note: ImageGen is output capability - doesn't affect file upload features
	// Note: Reasoning is processing mode - doesn't affect room features

	return caps
}

// effectiveModel returns the full prefixed model ID (e.g., "openai/gpt-5.2")
// Used for routing and display purposes
func (oc *AIClient) effectiveModel(meta *PortalMetadata) string {
	if meta != nil && meta.Model != "" {
		return meta.Model
	}
	return oc.defaultModelForProvider()
}

// effectiveModelForAPI returns the actual model name to send to the API
// For OpenRouter/Beeper, returns the full model ID (e.g., "openai/gpt-5.2")
// For direct providers, strips the prefix (e.g., "openai/gpt-5.2" → "gpt-5.2")
func (oc *AIClient) effectiveModelForAPI(meta *PortalMetadata) string {
	modelID := oc.effectiveModel(meta)

	// OpenRouter and Beeper route through a gateway that expects the full model ID
	loginMeta := loginMetadata(oc.UserLogin)
	if loginMeta.Provider == ProviderOpenRouter || loginMeta.Provider == ProviderBeeper {
		return modelID
	}

	// Direct OpenAI provider needs the prefix stripped
	_, actualModel := ParseModelPrefix(modelID)
	return actualModel
}

// defaultModelForProvider returns the configured default model for this login's provider
func (oc *AIClient) defaultModelForProvider() string {
	loginMeta := loginMetadata(oc.UserLogin)
	providers := oc.connector.Config.Providers

	switch loginMeta.Provider {
	case ProviderOpenAI:
		if providers.OpenAI.DefaultModel != "" {
			return providers.OpenAI.DefaultModel
		}
		return DefaultModelOpenAI
	case ProviderOpenRouter:
		if providers.OpenRouter.DefaultModel != "" {
			return providers.OpenRouter.DefaultModel
		}
		return DefaultModelOpenRouter
	case ProviderBeeper:
		if providers.Beeper.DefaultModel != "" {
			return providers.Beeper.DefaultModel
		}
		return DefaultModelBeeper
	default:
		return DefaultModelOpenAI
	}
}

func (oc *AIClient) effectivePrompt(meta *PortalMetadata) string {
	if meta != nil && meta.SystemPrompt != "" {
		return meta.SystemPrompt
	}
	return oc.connector.Config.DefaultSystemPrompt
}

func (oc *AIClient) effectiveTemperature(meta *PortalMetadata) float64 {
	if meta != nil && meta.Temperature > 0 {
		return meta.Temperature
	}
	return defaultTemperature
}

func (oc *AIClient) historyLimit(meta *PortalMetadata) int {
	if meta != nil && meta.MaxContextMessages > 0 {
		return meta.MaxContextMessages
	}
	return defaultMaxContextMessages
}

func (oc *AIClient) effectiveMaxTokens(meta *PortalMetadata) int {
	if meta != nil && meta.MaxCompletionTokens > 0 {
		return meta.MaxCompletionTokens
	}
	return defaultMaxTokens
}

// validateModel checks if a model is available for this user
func (oc *AIClient) validateModel(ctx context.Context, modelID string) (bool, error) {
	if modelID == "" {
		return true, nil
	}

	// First check local model cache
	models, err := oc.listAvailableModels(ctx, false)
	if err == nil {
		for _, model := range models {
			if model.ID == modelID {
				return true, nil
			}
		}
	}

	// Check against OpenRouter's model list (cached)
	_, actualModel := ParseModelPrefix(modelID)
	if IsValidOpenRouterModel(actualModel) {
		return true, nil
	}

	// Try to validate by making a minimal API call as last resort
	timeoutCtx, cancel := context.WithTimeout(ctx, modelValidationTimeout)
	defer cancel()

	_, err = oc.api.Models.Get(timeoutCtx, actualModel)
	return err == nil, nil
}

// listAvailableModels fetches models from OpenAI API and caches them
// Returns ModelInfo list from the provider
func (oc *AIClient) listAvailableModels(ctx context.Context, forceRefresh bool) ([]ModelInfo, error) {
	meta := loginMetadata(oc.UserLogin)

	// Check cache (refresh every 6 hours unless forced)
	if !forceRefresh && meta.ModelCache != nil {
		age := time.Now().Unix() - meta.ModelCache.LastRefresh
		if age < meta.ModelCache.CacheDuration {
			return meta.ModelCache.Models, nil
		}
	}

	// Fetch models from provider
	oc.log.Debug().Msg("Fetching available models from provider")

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var allModels []ModelInfo

	// List models from the provider
	if oc.provider != nil {
		models, err := oc.provider.ListModels(timeoutCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to list models: %w", err)
		}
		allModels = models
	}

	// Update cache
	if meta.ModelCache == nil {
		meta.ModelCache = &ModelCache{
			CacheDuration: int64(oc.connector.Config.ModelCacheDuration.Seconds()),
		}
	}
	meta.ModelCache.Models = allModels
	meta.ModelCache.LastRefresh = time.Now().Unix()

	// Save metadata
	if err := oc.UserLogin.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save model cache")
	}

	oc.log.Info().Int("count", len(allModels)).Msg("Cached available models")
	return allModels, nil
}

// findModelInfo looks up ModelInfo from the user's model cache by ID
func (oc *AIClient) findModelInfo(modelID string) *ModelInfo {
	meta := loginMetadata(oc.UserLogin)
	if meta.ModelCache == nil {
		return nil
	}
	for i := range meta.ModelCache.Models {
		if meta.ModelCache.Models[i].ID == modelID {
			return &meta.ModelCache.Models[i]
		}
	}
	return nil
}

// buildBasePrompt builds the system prompt and history portion of a prompt.
// This is the common pattern used by buildPrompt and buildPromptWithImage.
func (oc *AIClient) buildBasePrompt(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion

	// Add system prompt
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}

	// Add history
	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}
		for i := len(history) - 1; i >= 0; i-- {
			msgMeta := messageMeta(history[i])
			if !shouldIncludeInHistory(msgMeta) {
				continue
			}
			switch msgMeta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(msgMeta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(msgMeta.Body))
			}
		}
	}

	return prompt, nil
}

// buildPrompt builds a prompt with the latest user message
func (oc *AIClient) buildPrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, latest string) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}
	prompt = append(prompt, openai.UserMessage(latest))
	return prompt, nil
}

// buildPromptWithImage builds a prompt that includes an image URL
func (oc *AIClient) buildPromptWithImage(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	caption string,
	imageURL string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}

	// Build the user message with image
	// Convert Matrix mxc:// URL to HTTP URL for download
	httpURL := oc.convertMxcToHttp(imageURL)

	imageContent := openai.ChatCompletionContentPartUnionParam{
		OfImageURL: &openai.ChatCompletionContentPartImageParam{
			ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
				URL:    httpURL,
				Detail: "auto",
			},
		},
	}

	textContent := openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{
			Text: caption,
		},
	}

	// Create user message with both image and text
	userMsg := openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
					textContent,
					imageContent,
				},
			},
		},
	}

	prompt = append(prompt, userMsg)
	return prompt, nil
}

// buildPromptWithPDF builds a prompt with a PDF file for document analysis
func (oc *AIClient) buildPromptWithPDF(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	caption string,
	pdfURL string,
	mimeType string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}

	// Download and base64 encode the PDF
	b64Data, actualMimeType, err := oc.downloadAndEncodeMedia(ctx, pdfURL, 50) // 50MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to download PDF: %w", err)
	}
	if actualMimeType == "" || actualMimeType == "application/octet-stream" {
		actualMimeType = mimeType
	}
	if actualMimeType == "" {
		actualMimeType = "application/pdf"
	}

	// Build data URL for the PDF
	dataURL := fmt.Sprintf("data:%s;base64,%s", actualMimeType, b64Data)

	// OpenRouter uses file content type for PDFs
	// Format: {"type": "file", "file": {"filename": "doc.pdf", "file_data": "data:application/pdf;base64,..."}}
	fileContent := openai.ChatCompletionContentPartUnionParam{
		OfFile: &openai.ChatCompletionContentPartFileParam{
			File: openai.ChatCompletionContentPartFileFileParam{
				FileData: openai.String(dataURL),
			},
		},
	}

	textContent := openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{
			Text: caption,
		},
	}

	userMsg := openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
					textContent,
					fileContent,
				},
			},
		},
	}

	prompt = append(prompt, userMsg)
	return prompt, nil
}

// buildPromptWithAudio builds a prompt with audio content for audio-capable models
func (oc *AIClient) buildPromptWithAudio(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	caption string,
	audioURL string,
	mimeType string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}

	// Download and base64 encode the audio
	b64Data, actualMimeType, err := oc.downloadAndEncodeMedia(ctx, audioURL, 25) // 25MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to download audio: %w", err)
	}
	if actualMimeType == "" || actualMimeType == "application/octet-stream" {
		actualMimeType = mimeType
	}

	// Get audio format for OpenRouter API
	audioFormat := getAudioFormat(actualMimeType)

	// OpenRouter uses input_audio content type
	// Format: {"type": "input_audio", "input_audio": {"data": "base64...", "format": "wav"}}
	audioContent := openai.ChatCompletionContentPartUnionParam{
		OfInputAudio: &openai.ChatCompletionContentPartInputAudioParam{
			InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
				Data:   b64Data,
				Format: audioFormat,
			},
		},
	}

	textContent := openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{
			Text: caption,
		},
	}

	userMsg := openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
					textContent,
					audioContent,
				},
			},
		},
	}

	prompt = append(prompt, userMsg)
	return prompt, nil
}

// buildPromptWithVideo builds a prompt with video content for video-capable models
func (oc *AIClient) buildPromptWithVideo(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	caption string,
	videoURL string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}

	// Convert mxc:// URL to HTTP URL for video passthrough
	httpURL := oc.convertMxcToHttp(videoURL)

	// OpenRouter expects video_url content type for video
	// We pass the URL directly rather than base64 encoding due to video sizes
	// Note: The OpenAI SDK may not have direct video support, so we use a generic approach
	// Format: {"type": "video_url", "video_url": {"url": "https://..."}}
	// Since the OpenAI Go SDK may not have native video support, we'll construct the message manually
	// For now, we'll use the image URL approach which some models accept for video

	// TODO: Once OpenRouter/OpenAI SDK adds proper video support, update this
	// For now, we use a text-based approach with the video URL
	videoPrompt := fmt.Sprintf("%s\n\nVideo URL: %s", caption, httpURL)

	userMsg := openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfString: openai.String(videoPrompt),
			},
		},
	}

	prompt = append(prompt, userMsg)
	return prompt, nil
}

// buildPromptUpToMessage builds a prompt including messages up to and including the specified message
func (oc *AIClient) buildPromptUpToMessage(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	targetMessageID networkid.MessageID,
	newBody string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion

	// Add system prompt
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}

	// Get history
	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}

		// Add messages up to the target message, replacing the target with newBody
		for i := len(history) - 1; i >= 0; i-- {
			msg := history[i]
			msgMeta := messageMeta(msg)

			// Stop after adding the target message
			if msg.ID == targetMessageID {
				// Use the new body for the edited message
				prompt = append(prompt, openai.UserMessage(newBody))
				break
			}

			// Skip commands and non-conversation messages
			if !shouldIncludeInHistory(msgMeta) {
				continue
			}

			// Skip assistant messages that came after the target (we're going backwards)
			switch msgMeta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(msgMeta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(msgMeta.Body))
			}
		}
	} else {
		// No history, just add the new message
		prompt = append(prompt, openai.UserMessage(newBody))
	}

	return prompt, nil
}

// convertMxcToHttp converts an mxc:// URL to an HTTP URL via the homeserver
func (oc *AIClient) convertMxcToHttp(mxcURL string) string {
	// mxc://server/mediaID -> https://homeserver/_matrix/media/v3/download/server/mediaID
	if !strings.HasPrefix(mxcURL, "mxc://") {
		return mxcURL // Already HTTP
	}

	// Get homeserver URL from bridge config
	homeserver := oc.UserLogin.Bridge.Matrix.ServerName()

	// Parse mxc URL
	parts := strings.SplitN(strings.TrimPrefix(mxcURL, "mxc://"), "/", 2)
	if len(parts) != 2 {
		return mxcURL
	}

	server := parts[0]
	mediaID := parts[1]

	return fmt.Sprintf("https://%s/_matrix/media/v3/download/%s/%s", homeserver, server, mediaID)
}

// downloadAndEncodeMedia downloads media from Matrix and returns base64-encoded data
// maxSizeMB limits the download size (0 = no limit)
// Returns (base64Data, mimeType, error)
func (oc *AIClient) downloadAndEncodeMedia(ctx context.Context, mxcURL string, maxSizeMB int) (string, string, error) {
	// Convert mxc:// to HTTP URL
	httpURL := oc.convertMxcToHttp(mxcURL)

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	// Use a client with timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Check content length if available
	if maxSizeMB > 0 && resp.ContentLength > 0 {
		maxBytes := int64(maxSizeMB * 1024 * 1024)
		if resp.ContentLength > maxBytes {
			return "", "", fmt.Errorf("media too large: %d bytes (max %d MB)", resp.ContentLength, maxSizeMB)
		}
	}

	// Read with size limit
	var reader io.Reader = resp.Body
	if maxSizeMB > 0 {
		maxBytes := int64(maxSizeMB * 1024 * 1024)
		reader = io.LimitReader(resp.Body, maxBytes+1) // +1 to detect overflow
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to read media: %w", err)
	}

	// Check if we hit the size limit
	if maxSizeMB > 0 {
		maxBytes := int64(maxSizeMB * 1024 * 1024)
		if int64(len(data)) > maxBytes {
			return "", "", fmt.Errorf("media too large (max %d MB)", maxSizeMB)
		}
	}

	// Get MIME type from response header
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Base64 encode
	b64Data := base64.StdEncoding.EncodeToString(data)

	return b64Data, mimeType, nil
}

// getAudioFormat extracts the audio format from a MIME type for OpenRouter API
func getAudioFormat(mimeType string) string {
	switch mimeType {
	case "audio/wav", "audio/x-wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/webm":
		return "webm"
	case "audio/ogg":
		return "ogg"
	case "audio/flac":
		return "flac"
	case "audio/mp4", "audio/x-m4a":
		return "mp4"
	default:
		// Default to mp3 for unknown formats
		return "mp3"
	}
}

// ensureGhostDisplayName ensures the ghost has its display name set before sending messages.
// This fixes the issue where ghosts appear with raw user IDs instead of formatted names.
func (oc *AIClient) ensureGhostDisplayName(ctx context.Context, modelID string) {
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, modelUserID(modelID))
	if err != nil || ghost == nil {
		return
	}
	// Only update if name is not already set
	if ghost.Name == "" || !ghost.NameSet {
		caps := getModelCapabilities(modelID, oc.findModelInfo(modelID))
		displayName := FormatModelDisplayWithVision(modelID, caps)
		ghost.UpdateInfo(ctx, &bridgev2.UserInfo{
			Name:  ptr.Ptr(displayName),
			IsBot: ptr.Ptr(false),
		})
		oc.log.Debug().Str("model", modelID).Str("name", displayName).Msg("Updated ghost display name")
	}
}

// getModelIntent returns the Matrix intent for the current model's ghost in the portal, or nil if unavailable.
// Uses the portal's model configuration to determine which model ghost to use.
func (oc *AIClient) getModelIntent(ctx context.Context, portal *bridgev2.Portal) bridgev2.MatrixAPI {
	meta := portalMeta(portal)
	modelID := oc.effectiveModel(meta)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, modelUserID(modelID))
	if err != nil {
		oc.log.Warn().Err(err).Str("model", modelID).Msg("Failed to get model ghost")
		return nil
	}
	return ghost.Intent
}

func (oc *AIClient) backgroundContext(_ context.Context) context.Context {
	// Always prefer BackgroundCtx for long-running operations that outlive request context
	if oc.UserLogin != nil && oc.UserLogin.Bridge != nil && oc.UserLogin.Bridge.BackgroundCtx != nil {
		return oc.UserLogin.Bridge.BackgroundCtx
	}
	return context.Background()
}

func ptrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return ptr.Ptr(value)
}

// supportsReasoning checks if a model supports reasoning/thinking capabilities
// These models can use reasoning_effort parameter and stream reasoning tokens via Responses API
func supportsReasoning(modelID string) bool {
	// O-series reasoning models
	if strings.HasPrefix(modelID, "o1") || strings.HasPrefix(modelID, "o3") || strings.HasPrefix(modelID, "o4") {
		return true
	}
	// GPT-5.x models with reasoning support
	if strings.HasPrefix(modelID, "gpt-5") {
		return true
	}
	return false
}

// getModelCapabilities computes capabilities for a model.
// If info is provided, it uses the ModelInfo fields for accurate capability detection.
// Otherwise, it falls back to heuristic detection based on modelID.
func getModelCapabilities(modelID string, info *ModelInfo) ModelCapabilities {
	caps := ModelCapabilities{
		SupportsVision:    detectVisionSupport(modelID),
		SupportsReasoning: supportsReasoning(modelID),
	}

	// Use ModelInfo if available (more accurate than heuristics)
	if info != nil {
		caps.SupportsVision = info.SupportsVision
		caps.SupportsPDF = info.SupportsPDF
		caps.SupportsImageGen = info.SupportsImageGen
		// Also override reasoning if the info has it
		if info.SupportsReasoning {
			caps.SupportsReasoning = true
		}
	}

	return caps
}

// detectVisionSupport checks if a model supports vision/images
func detectVisionSupport(modelID string) bool {
	// Known vision-capable models
	visionModels := map[string]bool{
		"gpt-4o":                    true,
		"gpt-4o-mini":               true,
		"gpt-4-turbo":               true,
		"gpt-4-turbo-2024-04-09":    true,
		"gpt-4-vision-preview":      true,
		"gpt-4-1106-vision-preview": true,
	}

	if visionModels[modelID] {
		return true
	}

	// Check by prefix/contains
	return strings.HasPrefix(modelID, "gpt-4o") ||
		strings.HasPrefix(modelID, "gpt-4-turbo") ||
		strings.Contains(modelID, "vision")
}

package connector

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
)

var (
	_ bridgev2.NetworkAPI                    = (*OpenAIClient)(nil)
	_ bridgev2.IdentifierResolvingNetworkAPI = (*OpenAIClient)(nil)
	_ bridgev2.ContactListingNetworkAPI      = (*OpenAIClient)(nil)
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

type OpenAIClient struct {
	UserLogin *bridgev2.UserLogin
	connector *OpenAIConnector
	api       openai.Client
	apiKey    string
	log       zerolog.Logger

	loggedIn atomic.Bool
	chatLock sync.Mutex
}

func newOpenAIClient(login *bridgev2.UserLogin, connector *OpenAIConnector, apiKey string) (*OpenAIClient, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("missing OpenAI API key")
	}
	opts := []option.RequestOption{
		option.WithAPIKey(key),
	}
	if connector.Config.OpenAI.OrganizationID != "" {
		opts = append(opts, option.WithOrganization(connector.Config.OpenAI.OrganizationID))
	}
	if connector.Config.OpenAI.ProjectID != "" {
		opts = append(opts, option.WithProject(connector.Config.OpenAI.ProjectID))
	}
	if connector.Config.OpenAI.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(connector.Config.OpenAI.BaseURL))
	}
	client := openai.NewClient(opts...)
	return &OpenAIClient{
		UserLogin: login,
		connector: connector,
		api:       client,
		apiKey:    key,
		log:       login.Log.With().Str("component", "openai-network").Logger(),
	}, nil
}

func (oc *OpenAIClient) Connect(ctx context.Context) {
	model := oc.connector.Config.OpenAI.DefaultModel
	timeoutCtx, cancel := context.WithTimeout(ctx, oc.connector.Config.OpenAI.RequestTimeout)
	defer cancel()
	_, err := oc.api.Models.Get(timeoutCtx, model)
	if err != nil {
		oc.log.Warn().Err(err).Str("model", model).Msg("Failed to validate OpenAI credentials")
		oc.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      "openai-auth-error",
			Message:    "Failed to validate OpenAI credentials",
			Info: map[string]any{
				"model": model,
				"error": err.Error(),
			},
		})
		return
	}
	oc.loggedIn.Store(true)
	oc.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateConnected,
		Message:    "Connected to OpenAI",
	})
}

func (oc *OpenAIClient) Disconnect() {
	oc.loggedIn.Store(false)
}

func (oc *OpenAIClient) IsLoggedIn() bool {
	return oc.loggedIn.Load()
}

func (oc *OpenAIClient) LogoutRemote(ctx context.Context) {
	oc.Disconnect()
	oc.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateLoggedOut,
		Message:    "Disconnected by user",
	})
}

func (oc *OpenAIClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return userID == humanUserID(oc.UserLogin.ID)
}

func (oc *OpenAIClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	meta := portalMeta(portal)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = "ChatGPT"
		}
	}
	prompt := meta.SystemPrompt
	if prompt == "" {
		prompt = oc.connector.Config.OpenAI.SystemPrompt
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: ptrIfNotEmpty(prompt),
	}, nil
}

func (oc *OpenAIClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	name := ptr.Ptr("ChatGPT")
	isBot := ptr.Ptr(true)
	return &bridgev2.UserInfo{
		Name:  name,
		IsBot: isBot,
	}, nil
}

func (oc *OpenAIClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	// Explicitly advertise the lack of media support so SDK clients can hide attachment actions while we test text-only flows.
	return &event.RoomFeatures{
		File: event.FileFeatureMap{
			event.MsgImage:      cloneRejectAllMediaFeatures(),
			event.MsgVideo:      cloneRejectAllMediaFeatures(),
			event.MsgAudio:      cloneRejectAllMediaFeatures(),
			event.MsgFile:       cloneRejectAllMediaFeatures(),
			event.CapMsgVoice:   cloneRejectAllMediaFeatures(),
			event.CapMsgGIF:     cloneRejectAllMediaFeatures(),
			event.CapMsgSticker: cloneRejectAllMediaFeatures(),
		},
		Reply:               event.CapLevelFullySupported,
		Thread:              event.CapLevelFullySupported,
		Edit:                event.CapLevelFullySupported,
		Delete:              event.CapLevelPartialSupport,
		TypingNotifications: oc.connector.Config.Bridge.TypingNotifications,
		ReadReceipts:        true,
	}
}

func (oc *OpenAIClient) GetContactList(ctx context.Context) ([]*bridgev2.ResolveIdentifierResponse, error) {
	oc.log.Debug().Msg("Contact list requested; ensuring default chat exists")
	_ = oc.ensureDefaultChat(ctx)
	portals, _ := oc.listAllChatPortals(ctx)
	var chat *bridgev2.CreateChatResponse
	if len(portals) > 0 {
		chat = &bridgev2.CreateChatResponse{
			PortalKey: portals[0].PortalKey,
			Portal:    portals[0],
		}
	}
	return []*bridgev2.ResolveIdentifierResponse{{
		UserID: assistantUserID(oc.UserLogin.ID),
		UserInfo: &bridgev2.UserInfo{
			Name:  ptr.Ptr("ChatGPT"),
			IsBot: ptr.Ptr(true),
		},
		Chat: chat,
	}}, nil
}

func (oc *OpenAIClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	title := strings.TrimSpace(identifier)
	if title == "" {
		oc.log.Debug().Msg("ResolveIdentifier called without title; using default naming")
	}
	var resp *bridgev2.CreateChatResponse
	var err error
	if createChat {
		oc.log.Info().Str("title", title).Msg("Creating new chat via ResolveIdentifier")
		resp, err = oc.createChat(ctx, title, "")
		if err != nil {
			oc.log.Err(err).Msg("Failed to create chat via ResolveIdentifier")
			return nil, err
		}
	}
	return &bridgev2.ResolveIdentifierResponse{
		UserID: assistantUserID(oc.UserLogin.ID),
		UserInfo: &bridgev2.UserInfo{
			Name:  ptr.Ptr("ChatGPT"),
			IsBot: ptr.Ptr(true),
		},
		Chat: resp,
	}, nil
}

func (oc *OpenAIClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if msg.Content == nil {
		return nil, fmt.Errorf("missing message content")
	}
	switch msg.Content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
	default:
		return nil, fmt.Errorf("%s messages are not supported", msg.Content.MsgType)
	}
	body := strings.TrimSpace(msg.Content.Body)
	if body == "" {
		return nil, fmt.Errorf("empty messages are not supported")
	}
	portal := msg.Portal
	meta := portalMeta(portal)
	promptMessages, err := oc.buildPrompt(ctx, portal, meta, body)
	if err != nil {
		return nil, err
	}
	userMessage := &database.Message{
		ID:       networkid.MessageID(fmt.Sprintf("mx:%s", string(msg.Event.ID))),
		Room:     portal.PortalKey,
		SenderID: humanUserID(oc.UserLogin.ID),
		Metadata: &MessageMetadata{
			Role: "user",
			Body: body,
		},
		Timestamp: time.Now(),
	}
	// Dispatch completion handling in the background so the Matrix send pipeline can ack immediately.
	go oc.dispatchCompletion(ctx, msg.Event, portal, meta, promptMessages)
	return &bridgev2.MatrixMessageResponse{
		DB: userMessage,
	}, nil
}

func (oc *OpenAIClient) requestCompletion(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, meta *PortalMetadata) (*openai.ChatCompletion, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("prompt had no messages")
	}
	params := openai.ChatCompletionNewParams{
		Model:               shared.ChatModel(oc.effectiveModel(meta)),
		Messages:            messages,
		Temperature:         openai.Float(oc.effectiveTemperature(meta)),
		MaxCompletionTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, oc.connector.Config.OpenAI.RequestTimeout)
	defer cancel()
	resp, err := oc.api.Chat.Completions.New(timeoutCtx, params)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (oc *OpenAIClient) buildPrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, latest string) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}
	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}
		for i := len(history) - 1; i >= 0; i-- {
			meta := messageMeta(history[i])
			if meta == nil || meta.Body == "" {
				continue
			}
			switch meta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(meta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(meta.Body))
			}
		}
	}
	prompt = append(prompt, openai.UserMessage(latest))
	return prompt, nil
}

func (oc *OpenAIClient) queueAssistantMessage(portal *bridgev2.Portal, completion *openai.ChatCompletion) {
	if completion == nil || len(completion.Choices) == 0 {
		return
	}
	choice := completion.Choices[0]
	body := strings.TrimSpace(choice.Message.Content)
	if body == "" {
		return
	}
	meta := &MessageMetadata{
		Role:             "assistant",
		Body:             body,
		CompletionID:     completion.ID,
		FinishReason:     choice.FinishReason,
		PromptTokens:     completion.Usage.PromptTokens,
		CompletionTokens: completion.Usage.CompletionTokens,
	}
	event := &OpenAIRemoteMessage{
		PortalKey: portal.PortalKey,
		ID:        networkid.MessageID(fmt.Sprintf("openai:%s", uuid.NewString())),
		Sender: bridgev2.EventSender{
			Sender:      assistantUserID(oc.UserLogin.ID),
			ForceDMUser: true,
			SenderLogin: oc.UserLogin.ID,
			IsFromMe:    false,
		},
		Content:   body,
		Timestamp: time.Unix(completion.Created, 0),
		Metadata:  meta,
	}
	oc.UserLogin.QueueRemoteEvent(event)
}

func (oc *OpenAIClient) dispatchCompletion(
	ctx context.Context,
	sourceEvent *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	runCtx := oc.backgroundContext(ctx)
	runCtx = oc.log.WithContext(runCtx)
	resp, err := oc.requestCompletion(runCtx, prompt, meta)
	if err != nil {
		oc.notifyMatrixSendFailure(runCtx, portal, sourceEvent, err)
		return
	}
	if resp != nil {
		oc.queueAssistantMessage(portal, resp)
	}
}

func (oc *OpenAIClient) notifyMatrixSendFailure(ctx context.Context, portal *bridgev2.Portal, evt *event.Event, err error) {
	if portal == nil || portal.Bridge == nil || evt == nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to send message via OpenAI")
		return
	}
	status := bridgev2.WrapErrorInStatus(err).
		WithStatus(event.MessageStatusRetriable).
		WithMessage("Failed to reach OpenAI").
		WithIsCertain(true).
		WithSendNotice(true)
	portal.Bridge.Matrix.SendMessageStatus(ctx, &status, bridgev2.StatusEventInfoFromEvent(evt))
}

func (oc *OpenAIClient) backgroundContext(ctx context.Context) context.Context {
	if oc.UserLogin != nil && oc.UserLogin.Bridge != nil && oc.UserLogin.Bridge.BackgroundCtx != nil {
		return oc.UserLogin.Bridge.BackgroundCtx
	}
	if ctx == nil || ctx.Err() != nil {
		return context.Background()
	}
	return ctx
}

func (oc *OpenAIClient) sendWelcomeMessage(ctx context.Context, portal *bridgev2.Portal) {
	meta := portalMeta(portal)
	if meta.WelcomeSent {
		return
	}
	body := "This chat was created automatically. Send a message to start talking to ChatGPT."
	event := &OpenAIRemoteMessage{
		PortalKey: portal.PortalKey,
		ID:        networkid.MessageID(fmt.Sprintf("openai:welcome:%s", uuid.NewString())),
		Sender: bridgev2.EventSender{
			Sender:      assistantUserID(oc.UserLogin.ID),
			ForceDMUser: true,
			SenderLogin: oc.UserLogin.ID,
			IsFromMe:    false,
		},
		Content:   body,
		Timestamp: time.Now(),
		Metadata: &MessageMetadata{
			Role: "assistant",
			Body: body,
		},
	}
	oc.UserLogin.QueueRemoteEvent(event)
	meta.WelcomeSent = true
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to persist welcome message state")
	}
}

func (oc *OpenAIClient) effectiveModel(meta *PortalMetadata) string {
	if meta != nil && meta.Model != "" {
		return meta.Model
	}
	return oc.connector.Config.OpenAI.DefaultModel
}

func (oc *OpenAIClient) effectivePrompt(meta *PortalMetadata) string {
	if meta != nil && meta.SystemPrompt != "" {
		return meta.SystemPrompt
	}
	return oc.connector.Config.OpenAI.SystemPrompt
}

func (oc *OpenAIClient) effectiveTemperature(meta *PortalMetadata) float64 {
	if meta != nil && meta.Temperature > 0 {
		return meta.Temperature
	}
	if oc.connector.Config.OpenAI.DefaultTemperature > 0 {
		return oc.connector.Config.OpenAI.DefaultTemperature
	}
	return defaultTemperature
}

func (oc *OpenAIClient) historyLimit(meta *PortalMetadata) int {
	if meta != nil && meta.MaxContextMessages > 0 {
		return meta.MaxContextMessages
	}
	if oc.connector.Config.OpenAI.MaxContextMessages > 0 {
		return oc.connector.Config.OpenAI.MaxContextMessages
	}
	return defaultMaxContextMessages
}

func (oc *OpenAIClient) effectiveMaxTokens(meta *PortalMetadata) int {
	if meta != nil && meta.MaxCompletionTokens > 0 {
		return meta.MaxCompletionTokens
	}
	if oc.connector.Config.OpenAI.MaxCompletionTokens > 0 {
		return oc.connector.Config.OpenAI.MaxCompletionTokens
	}
	return defaultMaxTokens
}

func (oc *OpenAIClient) scheduleBootstrap() {
	backgroundCtx := oc.UserLogin.Bridge.BackgroundCtx
	go oc.bootstrap(backgroundCtx)
}

func (oc *OpenAIClient) bootstrap(ctx context.Context) {
	logCtx := oc.log.With().Str("component", "openai-chat-bootstrap").Logger().WithContext(ctx)
	oc.waitForLoginPersisted(logCtx)
	if err := oc.syncChatCounter(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to sync chat counter")
		return
	}
	if err := oc.ensureDefaultChat(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to ensure default chat")
	}
}

func (oc *OpenAIClient) waitForLoginPersisted(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := oc.UserLogin.Bridge.DB.UserLogin.GetByID(ctx, oc.UserLogin.ID)
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (oc *OpenAIClient) syncChatCounter(ctx context.Context) error {
	meta := loginMetadata(oc.UserLogin)
	portals, err := oc.listAllChatPortals(ctx)
	if err != nil {
		return err
	}
	maxIdx := meta.NextChatIndex
	for _, portal := range portals {
		pm := portalMeta(portal)
		if idx := parseChatSlug(pm.Slug); idx > maxIdx {
			maxIdx = idx
		}
	}
	if maxIdx > meta.NextChatIndex {
		meta.NextChatIndex = maxIdx
		return oc.UserLogin.Save(ctx)
	}
	return nil
}

func (oc *OpenAIClient) ensureDefaultChat(ctx context.Context) error {
	oc.log.Debug().Msg("Ensuring default ChatGPT room exists")
	portals, err := oc.listAllChatPortals(ctx)
	if err != nil {
		oc.log.Err(err).Msg("Failed to list chat portals")
		return err
	}
	for _, portal := range portals {
		if portal.MXID != "" {
			oc.log.Debug().Stringer("portal", portal.PortalKey).Msg("Existing chat already has MXID")
			return nil
		}
	}
	if len(portals) > 0 {
		info := oc.chatInfoFromPortal(portals[0])
		oc.log.Info().Stringer("portal", portals[0].PortalKey).Msg("Existing portal missing MXID; creating Matrix room")
		err := portals[0].CreateMatrixRoom(ctx, oc.UserLogin, info)
		if err != nil {
			oc.log.Err(err).Msg("Failed to create Matrix room for existing portal")
		}
		oc.sendWelcomeMessage(ctx, portals[0])
		return err
	}
	resp, err := oc.createChat(ctx, "", oc.connector.Config.OpenAI.SystemPrompt)
	if err != nil {
		oc.log.Err(err).Msg("Failed to create default portal")
		return err
	}
	err = resp.Portal.CreateMatrixRoom(ctx, oc.UserLogin, resp.PortalInfo)
	if err != nil {
		oc.log.Err(err).Msg("Failed to create Matrix room for default chat")
		return err
	}
	oc.sendWelcomeMessage(ctx, resp.Portal)
	oc.log.Info().Stringer("portal", resp.PortalKey).Msg("Default ChatGPT room created")
	return nil
}

func (oc *OpenAIClient) listAllChatPortals(ctx context.Context) ([]*bridgev2.Portal, error) {
	dbPortals, err := oc.UserLogin.Bridge.DB.Portal.GetAllDMsWith(ctx, assistantUserID(oc.UserLogin.ID))
	if err != nil {
		return nil, err
	}
	portals := make([]*bridgev2.Portal, 0, len(dbPortals))
	for _, dbPortal := range dbPortals {
		portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, dbPortal.PortalKey)
		if err != nil {
			return nil, err
		}
		if portal != nil {
			portals = append(portals, portal)
		}
	}
	return portals, nil
}

func (oc *OpenAIClient) createChat(ctx context.Context, title, systemPrompt string) (*bridgev2.CreateChatResponse, error) {
	if strings.TrimSpace(title) == "" {
		meta := loginMetadata(oc.UserLogin)
		next := meta.NextChatIndex + 1
		title = fmt.Sprintf("ChatGPT %d", next)
	}
	portal, info, err := oc.spawnPortal(ctx, title, systemPrompt)
	if err != nil {
		return nil, err
	}
	return &bridgev2.CreateChatResponse{
		PortalKey:  portal.PortalKey,
		Portal:     portal,
		PortalInfo: info,
	}, nil
}

func (oc *OpenAIClient) spawnPortal(ctx context.Context, title, systemPrompt string) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	oc.chatLock.Lock()
	defer oc.chatLock.Unlock()
	oc.log.Debug().Str("title", title).Msg("Allocating portal for new chat")

	meta := loginMetadata(oc.UserLogin)
	meta.NextChatIndex++
	index := meta.NextChatIndex
	slug := formatChatSlug(index)
	if title == "" {
		title = fmt.Sprintf("ChatGPT %d", index)
	}
	if systemPrompt == "" {
		systemPrompt = oc.connector.Config.OpenAI.SystemPrompt
	}
	key := portalKeyForChat(oc.UserLogin.ID, slug)
	portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, key)
	if err != nil {
		meta.NextChatIndex--
		return nil, nil, err
	}
	pmeta := portalMeta(portal)
	pmeta.Slug = slug
	pmeta.Title = title
	if systemPrompt != "" {
		pmeta.SystemPrompt = systemPrompt
	}
	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = assistantUserID(oc.UserLogin.ID)
	portal.Name = title
	portal.NameSet = true
	portal.Topic = systemPrompt
	portal.TopicSet = systemPrompt != ""
	if err := portal.Save(ctx); err != nil {
		meta.NextChatIndex--
		return nil, nil, err
	}
	if err := oc.UserLogin.Save(ctx); err != nil {
		return nil, nil, err
	}
	info := oc.composeChatInfo(title, systemPrompt)
	return portal, info, nil
}

func (oc *OpenAIClient) chatInfoFromPortal(portal *bridgev2.Portal) *bridgev2.ChatInfo {
	meta := portalMeta(portal)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = "ChatGPT"
		}
	}
	prompt := meta.SystemPrompt
	if prompt == "" {
		prompt = oc.connector.Config.OpenAI.SystemPrompt
	}
	return oc.composeChatInfo(title, prompt)
}

func (oc *OpenAIClient) composeChatInfo(title, prompt string) *bridgev2.ChatInfo {
	if title == "" {
		title = "ChatGPT"
	}
	if prompt == "" {
		prompt = oc.connector.Config.OpenAI.SystemPrompt
	}
	members := bridgev2.ChatMemberMap{
		humanUserID(oc.UserLogin.ID): {
			EventSender: bridgev2.EventSender{
				IsFromMe:    true,
				SenderLogin: oc.UserLogin.ID,
			},
			Membership: event.MembershipJoin,
		},
		assistantUserID(oc.UserLogin.ID): {
			EventSender: bridgev2.EventSender{
				Sender:      assistantUserID(oc.UserLogin.ID),
				SenderLogin: oc.UserLogin.ID,
			},
			Membership: event.MembershipJoin,
			UserInfo: &bridgev2.UserInfo{
				Name:  ptr.Ptr("ChatGPT"),
				IsBot: ptr.Ptr(true),
			},
		},
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: ptrIfNotEmpty(prompt),
		Type:  ptr.Ptr(database.RoomTypeDM),
		Members: &bridgev2.ChatMemberList{
			IsFull:      true,
			OtherUserID: assistantUserID(oc.UserLogin.ID),
			MemberMap:   members,
		},
	}
}

func ptrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return ptr.Ptr(value)
}

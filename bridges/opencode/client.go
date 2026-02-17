package opencode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"

	"github.com/beeper/ai-bridge/bridges/opencode/opencodebridge"
)

var _ bridgev2.NetworkAPI = (*OpenCodeClient)(nil)
var _ bridgev2.BackfillingNetworkAPI = (*OpenCodeClient)(nil)
var _ bridgev2.DeleteChatHandlingNetworkAPI = (*OpenCodeClient)(nil)
var _ bridgev2.IdentifierResolvingNetworkAPI = (*OpenCodeClient)(nil)
var _ bridgev2.ContactListingNetworkAPI = (*OpenCodeClient)(nil)

type OpenCodeClient struct {
	UserLogin *bridgev2.UserLogin
	connector *OpenCodeConnector
	bridge    *opencodebridge.Bridge

	loggedIn atomic.Bool

	streamSeqMu sync.Mutex
	streamSeq   map[string]int
}

func newOpenCodeClient(login *bridgev2.UserLogin, connector *OpenCodeConnector) (*OpenCodeClient, error) {
	if login == nil {
		return nil, errors.New("missing login")
	}
	client := &OpenCodeClient{
		UserLogin: login,
		connector: connector,
		streamSeq: make(map[string]int),
	}
	client.bridge = opencodebridge.NewBridge(client)
	return client, nil
}

func (oc *OpenCodeClient) Connect(ctx context.Context) {
	if oc == nil || oc.UserLogin == nil {
		return
	}
	oc.loggedIn.Store(true)
	oc.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected, Message: "Connected"})
	if oc.bridge != nil {
		if err := oc.bridge.RestoreConnections(oc.BackgroundContext(ctx)); err != nil {
			oc.UserLogin.Log.Warn().Err(err).Msg("Failed to restore OpenCode connections")
		}
	}
}

func (oc *OpenCodeClient) Disconnect() {
	if oc == nil {
		return
	}
	oc.loggedIn.Store(false)
	if oc.bridge != nil {
		oc.bridge.DisconnectAll()
	}
	if oc.UserLogin != nil {
		oc.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateTransientDisconnect, Message: "Disconnected"})
	}
}

func (oc *OpenCodeClient) IsLoggedIn() bool {
	if oc == nil {
		return false
	}
	return oc.loggedIn.Load()
}

func (oc *OpenCodeClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if oc == nil || msg == nil || msg.Portal == nil {
		return nil, errors.New("missing portal context")
	}
	if oc.bridge == nil {
		return &bridgev2.MatrixMessageResponse{Pending: false}, nil
	}
	meta := portalMeta(msg.Portal)
	if !meta.IsOpenCodeRoom {
		return &bridgev2.MatrixMessageResponse{Pending: false}, nil
	}
	return oc.bridge.HandleMatrixMessage(ctx, msg, msg.Portal, oc.PortalMeta(msg.Portal))
}

func (oc *OpenCodeClient) HandleMatrixDeleteChat(ctx context.Context, msg *bridgev2.MatrixDeleteChat) error {
	if oc == nil || oc.bridge == nil {
		return nil
	}
	return oc.bridge.HandleMatrixDeleteChat(ctx, msg)
}

func (oc *OpenCodeClient) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	if oc == nil || oc.bridge == nil {
		return nil, nil
	}
	if params.Portal == nil {
		return nil, nil
	}
	meta := portalMeta(params.Portal)
	if !meta.IsOpenCodeRoom {
		return nil, nil
	}
	return oc.bridge.FetchMessages(ctx, params)
}

func openCodeFileFeatures() *event.FileFeatures {
	return &event.FileFeatures{
		MimeTypes: map[string]event.CapabilitySupportLevel{
			"*/*": event.CapLevelFullySupported,
		},
		Caption:          event.CapLevelFullySupported,
		MaxCaptionLength: 100000,
		MaxSize:          50 * 1024 * 1024,
	}
}

func (oc *OpenCodeClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	_ = ctx
	_ = portal
	return &event.RoomFeatures{
		ID: "com.beeper.ai.capabilities.2026_02_17+opencode",
		File: event.FileFeatureMap{
			event.MsgImage:      openCodeFileFeatures(),
			event.MsgVideo:      openCodeFileFeatures(),
			event.MsgAudio:      openCodeFileFeatures(),
			event.MsgFile:       openCodeFileFeatures(),
			event.CapMsgVoice:   openCodeFileFeatures(),
			event.CapMsgGIF:     openCodeFileFeatures(),
			event.CapMsgSticker: openCodeFileFeatures(),
		},
		MaxTextLength:       100000,
		Reply:               event.CapLevelFullySupported,
		Thread:              event.CapLevelFullySupported,
		Edit:                event.CapLevelRejected,
		Delete:              event.CapLevelRejected,
		Reaction:            event.CapLevelRejected,
		ReadReceipts:        true,
		TypingNotifications: true,
		DeleteChat:          true,
	}
}

func (oc *OpenCodeClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	_ = ctx
	if ghost == nil {
		return &bridgev2.UserInfo{Name: ptr.Ptr("OpenCode"), IsBot: ptr.Ptr(true)}, nil
	}
	instanceID, ok := opencodebridge.ParseOpenCodeGhostID(string(ghost.ID))
	if !ok {
		return &bridgev2.UserInfo{Name: ptr.Ptr("OpenCode"), IsBot: ptr.Ptr(true)}, nil
	}
	display := "OpenCode"
	if oc.bridge != nil {
		if name := strings.TrimSpace(oc.bridge.DisplayName(instanceID)); name != "" {
			display = name
		}
	}
	return &bridgev2.UserInfo{
		Name:        ptr.Ptr(display),
		IsBot:       ptr.Ptr(true),
		Identifiers: []string{"opencode:" + instanceID},
	}, nil
}

func (oc *OpenCodeClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	if oc == nil || oc.UserLogin == nil || oc.bridge == nil {
		return nil, errors.New("login unavailable")
	}
	instanceID, ok := opencodebridge.ParseOpenCodeIdentifier(identifier)
	if !ok {
		return nil, fmt.Errorf("unknown identifier: %s", identifier)
	}
	cfg := oc.bridge.InstanceConfig(instanceID)
	if cfg == nil {
		return nil, errors.New("OpenCode instance not found")
	}
	userID := opencodebridge.OpenCodeUserID(instanceID)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OpenCode ghost: %w", err)
	}
	oc.bridge.EnsureGhostDisplayName(ctx, instanceID)

	var chat *bridgev2.CreateChatResponse
	if createChat {
		chat, err = oc.bridge.CreateSessionChat(ctx, instanceID, "", true)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenCode chat: %w", err)
		}
	}

	displayName := oc.bridge.DisplayName(instanceID)
	if displayName == "" {
		displayName = "OpenCode"
	}
	return &bridgev2.ResolveIdentifierResponse{
		UserID: userID,
		UserInfo: &bridgev2.UserInfo{
			Name:        ptr.Ptr(displayName),
			IsBot:       ptr.Ptr(true),
			Identifiers: []string{"opencode:" + instanceID},
		},
		Ghost: ghost,
		Chat:  chat,
	}, nil
}

func (oc *OpenCodeClient) GetContactList(ctx context.Context) ([]*bridgev2.ResolveIdentifierResponse, error) {
	if oc == nil || oc.UserLogin == nil {
		return nil, nil
	}
	meta := loginMetadata(oc.UserLogin)
	if meta == nil || len(meta.OpenCodeInstances) == 0 {
		return nil, nil
	}
	out := make([]*bridgev2.ResolveIdentifierResponse, 0, len(meta.OpenCodeInstances))
	for instanceID := range meta.OpenCodeInstances {
		resp, err := oc.ResolveIdentifier(ctx, "opencode:"+instanceID, false)
		if err == nil && resp != nil {
			out = append(out, resp)
		}
	}
	return out, nil
}

func (oc *OpenCodeClient) LogoutRemote(ctx context.Context) {
	_ = ctx
	if oc == nil {
		return
	}
	oc.Disconnect()
	if oc.connector != nil && oc.UserLogin != nil {
		oc.connector.clientsMu.Lock()
		delete(oc.connector.clients, oc.UserLogin.ID)
		oc.connector.clientsMu.Unlock()
	}
}

func (oc *OpenCodeClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	_ = ctx
	if oc == nil || oc.UserLogin == nil {
		return false
	}
	return userID == humanUserID(oc.UserLogin.ID)
}

func (oc *OpenCodeClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	_ = ctx
	if portal == nil {
		return nil, nil
	}
	meta := portalMeta(portal)
	if !meta.IsOpenCodeRoom {
		return nil, nil
	}
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = "OpenCode"
	}
	return &bridgev2.ChatInfo{Name: ptr.Ptr(title)}, nil
}

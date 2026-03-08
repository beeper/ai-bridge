package openclaw

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/bridgeadapter"
	"github.com/beeper/ai-bridge/pkg/matrixevents"
	"github.com/beeper/ai-bridge/pkg/shared/streamtransport"
	"github.com/beeper/ai-bridge/pkg/shared/streamui"
)

var _ bridgev2.NetworkAPI = (*OpenClawClient)(nil)
var _ bridgev2.BackfillingNetworkAPI = (*OpenClawClient)(nil)

type OpenClawClient struct {
	UserLogin *bridgev2.UserLogin
	connector *OpenClawConnector

	manager *openClawManager

	loggedIn atomic.Bool

	streamMu                  sync.Mutex
	streamSessions            map[string]*streamtransport.StreamSession
	streamStates              map[string]*openClawStreamState
	streamFallbackToDebounced atomic.Bool
}

type openClawStreamState struct {
	portal           *bridgev2.Portal
	turnID           string
	agentID          string
	sessionKey       string
	targetEventID    string
	initialEventID   id.EventID
	networkMessageID networkid.MessageID
	sequenceNum      int
	accumulated      strings.Builder
	visible          strings.Builder
	ui               streamui.UIState
	lastVisibleText  string
	role             string
	runID            string
	sessionID        string
	finishReason     string
	errorText        string
	promptTokens     int64
	completionTokens int64
	reasoningTokens  int64
	startedAtMs      int64
	firstTokenAtMs   int64
	completedAtMs    int64
}

func newOpenClawClient(login *bridgev2.UserLogin, connector *OpenClawConnector) (*OpenClawClient, error) {
	if login == nil {
		return nil, errors.New("missing login")
	}
	client := &OpenClawClient{
		UserLogin:      login,
		connector:      connector,
		streamSessions: make(map[string]*streamtransport.StreamSession),
		streamStates:   make(map[string]*openClawStreamState),
	}
	client.manager = newOpenClawManager(client)
	return client, nil
}

func (oc *OpenClawClient) Connect(ctx context.Context) {
	oc.loggedIn.Store(true)
	oc.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting, Message: "Connecting"})
	go func() {
		if err := oc.manager.Start(oc.BackgroundContext(ctx)); err != nil {
			oc.loggedIn.Store(false)
			oc.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateBadCredentials, Message: err.Error()})
			return
		}
		oc.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected, Message: "Connected"})
	}()
}

func (oc *OpenClawClient) Disconnect() {
	oc.loggedIn.Store(false)
	if oc.manager != nil {
		oc.manager.Stop()
	}
	oc.streamMu.Lock()
	sessions := make([]*streamtransport.StreamSession, 0, len(oc.streamSessions))
	for _, s := range oc.streamSessions {
		if s != nil {
			sessions = append(sessions, s)
		}
	}
	oc.streamSessions = make(map[string]*streamtransport.StreamSession)
	oc.streamStates = make(map[string]*openClawStreamState)
	oc.streamMu.Unlock()
	for _, s := range sessions {
		s.End(context.Background(), streamtransport.EndReasonDisconnect)
	}
	if oc.UserLogin != nil {
		oc.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateTransientDisconnect, Message: "Disconnected"})
	}
}

func (oc *OpenClawClient) IsLoggedIn() bool { return oc.loggedIn.Load() }

func (oc *OpenClawClient) LogoutRemote(_ context.Context) {}

func (oc *OpenClawClient) IsThisUser(_ context.Context, userID networkid.UserID) bool {
	return userID == humanUserID(oc.UserLogin.ID)
}

func (oc *OpenClawClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if msg == nil || msg.Portal == nil {
		return nil, errors.New("missing portal context")
	}
	if handled, resp := oc.tryApprovalDecisionEvent(ctx, msg); handled {
		return resp, nil
	}
	meta := portalMeta(msg.Portal)
	if !meta.IsOpenClawRoom {
		return &bridgev2.MatrixMessageResponse{Pending: false}, nil
	}
	return oc.manager.HandleMatrixMessage(ctx, msg)
}

func (oc *OpenClawClient) tryApprovalDecisionEvent(ctx context.Context, msg *bridgev2.MatrixMessage) (bool, *bridgev2.MatrixMessageResponse) {
	if oc == nil || oc.manager == nil || msg == nil || msg.Event == nil || msg.Portal == nil {
		return false, nil
	}
	raw, ok := msg.Event.Content.Raw["com.beeper.ai.approval_decision"].(map[string]any)
	if !ok {
		return false, nil
	}
	decision, ok := bridgeadapter.ParseApprovalDecision(raw)
	if !ok {
		return true, &bridgev2.MatrixMessageResponse{Pending: false}
	}
	if err := oc.manager.ResolveApprovalDecision(ctx, msg.Portal, decision); err != nil {
		oc.sendSystemNoticeViaPortal(ctx, msg.Portal, bridgeadapter.ApprovalErrorToastText(err))
	}
	return true, &bridgev2.MatrixMessageResponse{Pending: false}
}

func (oc *OpenClawClient) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	if params.Portal == nil {
		return nil, nil
	}
	if !portalMeta(params.Portal).IsOpenClawRoom {
		return nil, nil
	}
	return oc.manager.FetchMessages(ctx, params)
}

func (oc *OpenClawClient) GetCapabilities(_ context.Context, _ *bridgev2.Portal) *event.RoomFeatures {
	return &event.RoomFeatures{
		ID: "com.beeper.ai.capabilities.2026_03_08+openclaw",
		File: event.FileFeatureMap{
			event.MsgImage:      openClawFileFeatures,
			event.MsgVideo:      openClawFileFeatures,
			event.MsgAudio:      openClawFileFeatures,
			event.MsgFile:       openClawFileFeatures,
			event.CapMsgVoice:   openClawFileFeatures,
			event.CapMsgGIF:     openClawFileFeatures,
			event.CapMsgSticker: openClawFileFeatures,
		},
		MaxTextLength:       100000,
		Reply:               event.CapLevelFullySupported,
		Thread:              event.CapLevelFullySupported,
		Edit:                event.CapLevelRejected,
		Delete:              event.CapLevelRejected,
		Reaction:            event.CapLevelRejected,
		ReadReceipts:        true,
		TypingNotifications: true,
	}
}

func (oc *OpenClawClient) GetChatInfo(_ context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	meta := portalMeta(portal)
	title := oc.displayNameForPortal(meta)
	return &bridgev2.ChatInfo{Name: ptr.Ptr(title)}, nil
}

func (oc *OpenClawClient) GetUserInfo(_ context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	if ghost == nil {
		return &bridgev2.UserInfo{Name: ptr.Ptr("OpenClaw"), IsBot: ptr.Ptr(true)}, nil
	}
	agentID, ok := parseOpenClawGhostID(string(ghost.ID))
	if !ok {
		return &bridgev2.UserInfo{Name: ptr.Ptr("OpenClaw"), IsBot: ptr.Ptr(true)}, nil
	}
	meta := ghostMeta(ghost)
	name := meta.OpenClawAgentName
	if name == "" {
		name = oc.displayNameForAgent(agentID)
	}
	return &bridgev2.UserInfo{
		Name:        ptr.Ptr(name),
		IsBot:       ptr.Ptr(true),
		Identifiers: []string{"openclaw:" + agentID},
	}, nil
}

func (oc *OpenClawClient) Log() *zerolog.Logger {
	if oc == nil || oc.UserLogin == nil {
		l := zerolog.Nop()
		return &l
	}
	l := oc.UserLogin.Log.With().Str("component", "openclaw").Logger()
	return &l
}

func (oc *OpenClawClient) BackgroundContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	if oc != nil && oc.UserLogin != nil && oc.UserLogin.Bridge != nil && oc.UserLogin.Bridge.BackgroundCtx != nil {
		return oc.UserLogin.Bridge.BackgroundCtx
	}
	return context.Background()
}

func (oc *OpenClawClient) gatewayID() string {
	meta := loginMetadata(oc.UserLogin)
	return openClawGatewayID(meta.GatewayURL, meta.GatewayLabel)
}

func (oc *OpenClawClient) portalKeyForSession(sessionKey string) networkid.PortalKey {
	return openClawPortalKey(oc.UserLogin.ID, oc.gatewayID(), sessionKey)
}

func (oc *OpenClawClient) displayNameForSession(session gatewaySessionRow) string {
	if strings.TrimSpace(session.DerivedTitle) != "" {
		return strings.TrimSpace(session.DerivedTitle)
	}
	if strings.TrimSpace(session.DisplayName) != "" {
		return strings.TrimSpace(session.DisplayName)
	}
	if strings.TrimSpace(session.Label) != "" {
		return strings.TrimSpace(session.Label)
	}
	if strings.TrimSpace(session.Key) != "" {
		return strings.TrimSpace(session.Key)
	}
	return "OpenClaw"
}

func (oc *OpenClawClient) displayNameForPortal(meta *PortalMetadata) string {
	if meta == nil {
		return "OpenClaw"
	}
	for _, value := range []string{meta.OpenClawDerivedTitle, meta.OpenClawDisplayName, meta.OpenClawSessionKey} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "OpenClaw"
}

func (oc *OpenClawClient) displayNameForAgent(agentID string) string {
	if strings.TrimSpace(agentID) == "" || strings.EqualFold(strings.TrimSpace(agentID), "gateway") {
		meta := loginMetadata(oc.UserLogin)
		if label := strings.TrimSpace(meta.GatewayLabel); label != "" {
			return label
		}
		return "OpenClaw"
	}
	return strings.TrimSpace(agentID)
}

func (oc *OpenClawClient) senderForAgent(agentID string, fromMe bool) bridgev2.EventSender {
	if fromMe {
		return bridgev2.EventSender{
			Sender:      humanUserID(oc.UserLogin.ID),
			SenderLogin: oc.UserLogin.ID,
			IsFromMe:    true,
		}
	}
	return bridgev2.EventSender{
		Sender:      openClawGhostUserID(agentID),
		SenderLogin: oc.UserLogin.ID,
		ForceDMUser: true,
	}
}

func (oc *OpenClawClient) sendSystemNoticeViaPortal(ctx context.Context, portal *bridgev2.Portal, msg string) {
	if portal == nil || strings.TrimSpace(msg) == "" {
		return
	}
	converted := &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{{
			ID:      networkid.PartID("0"),
			Type:    event.EventMessage,
			Content: &event.MessageEventContent{MsgType: event.MsgNotice, Body: msg},
			Extra:   map[string]any{"msgtype": event.MsgNotice, "body": msg, "m.mentions": map[string]any{}},
		}},
	}
	oc.UserLogin.QueueRemoteEvent(&OpenClawRemoteMessage{
		portal:    portal.PortalKey,
		id:        newOpenClawMessageID(),
		sender:    oc.senderForAgent("gateway", false),
		timestamp: time.Now(),
		preBuilt:  converted,
	})
}

func (oc *OpenClawClient) sendApprovalRequestFallbackEvent(ctx context.Context, portal *bridgev2.Portal, approvalID, body string) {
	uiMessage := map[string]any{
		"id":   approvalID,
		"role": "assistant",
		"metadata": map[string]any{
			"approvalId": approvalID,
		},
		"parts": []map[string]any{{
			"type":       "dynamic-tool",
			"toolName":   "exec",
			"toolCallId": approvalID,
			"state":      "approval-requested",
			"approval": map[string]any{
				"id": approvalID,
			},
		}},
	}
	converted := &bridgev2.ConvertedMessage{
		Parts: []*bridgev2.ConvertedMessagePart{{
			ID:      networkid.PartID("0"),
			Type:    event.EventMessage,
			Content: &event.MessageEventContent{MsgType: event.MsgNotice, Body: body},
			Extra: map[string]any{
				"msgtype":                event.MsgNotice,
				"body":                   body,
				"m.mentions":             map[string]any{},
				matrixevents.BeeperAIKey: uiMessage,
			},
			DBMetadata: &MessageMetadata{
				Role:               "assistant",
				ExcludeFromHistory: true,
				CanonicalSchema:    "ai-sdk-ui-message-v1",
				CanonicalUIMessage: uiMessage,
			},
		}},
	}
	oc.UserLogin.QueueRemoteEvent(&OpenClawRemoteMessage{
		portal:    portal.PortalKey,
		id:        newOpenClawMessageID(),
		sender:    oc.senderForAgent("gateway", false),
		timestamp: time.Now(),
		preBuilt:  converted,
	})
}

func (oc *OpenClawClient) DownloadAndEncodeMedia(ctx context.Context, mediaURL string, file *event.EncryptedFileInfo, maxMB int) (string, string, error) {
	if strings.TrimSpace(mediaURL) == "" {
		return "", "", errors.New("missing media URL")
	}
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.Bot == nil {
		return "", "", errors.New("bridge is unavailable")
	}
	maxBytes := int64(0)
	if maxMB > 0 {
		maxBytes = int64(maxMB) * 1024 * 1024
	}
	var encoded string
	err := oc.UserLogin.Bridge.Bot.DownloadMediaToFile(ctx, id.ContentURIString(mediaURL), file, false, func(f *os.File) error {
		var reader io.Reader = f
		if maxBytes > 0 {
			reader = io.LimitReader(f, maxBytes+1)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		if maxBytes > 0 && int64(len(data)) > maxBytes {
			return errors.New("media exceeds max size")
		}
		encoded = base64.StdEncoding.EncodeToString(data)
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return encoded, "application/octet-stream", nil
}

func stringsTrimDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

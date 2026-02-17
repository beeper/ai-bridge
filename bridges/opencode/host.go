package opencode

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/bridges/opencode/opencodebridge"
	"github.com/beeper/ai-bridge/pkg/matrixevents"
)

var _ opencodebridge.Host = (*OpenCodeClient)(nil)

func (oc *OpenCodeClient) Log() *zerolog.Logger {
	if oc == nil {
		logger := zerolog.Nop()
		return &logger
	}
	l := oc.UserLogin.Log.With().Str("component", "opencode").Logger()
	return &l
}

func (oc *OpenCodeClient) Login() *bridgev2.UserLogin {
	if oc == nil {
		return nil
	}
	return oc.UserLogin
}

func (oc *OpenCodeClient) BackgroundContext(ctx context.Context) context.Context {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.BackgroundCtx == nil {
		return ctx
	}
	if ctx == nil {
		return oc.UserLogin.Bridge.BackgroundCtx
	}
	return ctx
}

func (oc *OpenCodeClient) SendSystemNotice(ctx context.Context, portal *bridgev2.Portal, msg string) {
	if oc == nil || portal == nil || portal.MXID == "" || oc.UserLogin == nil || oc.UserLogin.Bridge == nil {
		return
	}
	content := &event.MessageEventContent{MsgType: event.MsgNotice, Body: msg, Mentions: &event.Mentions{}}
	_, _ = oc.UserLogin.Bridge.Bot.SendMessage(ctx, portal.MXID, event.EventMessage, &event.Content{Parsed: content}, nil)
}

func (oc *OpenCodeClient) SendPendingStatus(ctx context.Context, portal *bridgev2.Portal, evt *event.Event, msg string) {
	_ = ctx
	_ = portal
	_ = evt
	_ = msg
}

func (oc *OpenCodeClient) SendSuccessStatus(ctx context.Context, portal *bridgev2.Portal, evt *event.Event) {
	_ = ctx
	_ = portal
	_ = evt
}

func (oc *OpenCodeClient) EmitOpenCodeStreamEvent(ctx context.Context, portal *bridgev2.Portal, turnID, agentID, targetEventID string, part map[string]any) {
	_ = ctx
	_ = portal
	_ = turnID
	_ = agentID
	_ = targetEventID
	_ = part
}

func (oc *OpenCodeClient) FinishOpenCodeStream(turnID string) {
	if oc == nil || turnID == "" {
		return
	}
	oc.streamSeqMu.Lock()
	delete(oc.streamSeq, turnID)
	oc.streamSeqMu.Unlock()
}

func (oc *OpenCodeClient) DownloadAndEncodeMedia(ctx context.Context, mediaURL string, file *event.EncryptedFileInfo, maxMB int) (string, string, error) {
	_ = maxMB
	if strings.TrimSpace(mediaURL) == "" {
		return "", "", errors.New("missing media URL")
	}
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.Bot == nil {
		return "", "", errors.New("bridge is unavailable")
	}
	data, err := oc.UserLogin.Bridge.Bot.DownloadMedia(ctx, id.ContentURIString(mediaURL), file)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(data), "application/octet-stream", nil
}

func (oc *OpenCodeClient) SetRoomName(ctx context.Context, portal *bridgev2.Portal, name string) error {
	_ = ctx
	_ = portal
	_ = name
	return nil
}

func (oc *OpenCodeClient) SenderForOpenCode(instanceID string, fromMe bool) bridgev2.EventSender {
	if oc == nil || oc.UserLogin == nil {
		return bridgev2.EventSender{}
	}
	if fromMe {
		return bridgev2.EventSender{Sender: humanUserID(oc.UserLogin.ID), SenderLogin: oc.UserLogin.ID, IsFromMe: true}
	}
	return bridgev2.EventSender{
		Sender:      opencodebridge.OpenCodeUserID(instanceID),
		SenderLogin: oc.UserLogin.ID,
		IsFromMe:    false,
		ForceDMUser: true,
	}
}

func (oc *OpenCodeClient) CleanupPortal(ctx context.Context, portal *bridgev2.Portal, reason string) {
	if oc == nil || portal == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil {
		return
	}
	if portal.MXID != "" {
		if err := portal.Delete(ctx); err != nil {
			oc.UserLogin.Log.Warn().Err(err).Str("portal_id", string(portal.PortalKey.ID)).Str("reason", reason).Msg("Failed to delete portal room")
		}
	}
	if err := oc.UserLogin.Bridge.DB.Portal.Delete(ctx, portal.PortalKey); err != nil {
		oc.UserLogin.Log.Warn().Err(err).Str("portal_id", string(portal.PortalKey.ID)).Str("reason", reason).Msg("Failed to delete portal record")
	}
}

func (oc *OpenCodeClient) PortalMeta(portal *bridgev2.Portal) *opencodebridge.PortalMeta {
	if portal == nil {
		return nil
	}
	meta := portalMeta(portal)
	return &opencodebridge.PortalMeta{
		IsOpenCodeRoom: meta.IsOpenCodeRoom,
		InstanceID:     meta.OpenCodeInstanceID,
		SessionID:      meta.OpenCodeSessionID,
		ReadOnly:       meta.OpenCodeReadOnly,
		TitlePending:   meta.OpenCodeTitlePending,
		Title:          meta.Title,
		TitleGenerated: meta.TitleGenerated,
		AgentID:        meta.AgentID,
		VerboseLevel:   meta.VerboseLevel,
	}
}

func (oc *OpenCodeClient) SetPortalMeta(portal *bridgev2.Portal, meta *opencodebridge.PortalMeta) {
	if portal == nil {
		return
	}
	existing := portalMeta(portal)
	if meta == nil {
		portal.Metadata = existing
		return
	}
	existing.IsOpenCodeRoom = meta.IsOpenCodeRoom
	existing.OpenCodeInstanceID = meta.InstanceID
	existing.OpenCodeSessionID = meta.SessionID
	existing.OpenCodeReadOnly = meta.ReadOnly
	existing.OpenCodeTitlePending = meta.TitlePending
	existing.Title = meta.Title
	existing.TitleGenerated = meta.TitleGenerated
	existing.AgentID = meta.AgentID
	existing.VerboseLevel = meta.VerboseLevel
	portal.Metadata = existing
}

func (oc *OpenCodeClient) SavePortal(ctx context.Context, portal *bridgev2.Portal) error {
	if portal == nil {
		return nil
	}
	return portal.Save(ctx)
}

func (oc *OpenCodeClient) DefaultAgentID() string {
	return "opencode"
}

func (oc *OpenCodeClient) OpenCodeInstances() map[string]*opencodebridge.OpenCodeInstance {
	if oc == nil || oc.UserLogin == nil {
		return nil
	}
	return loginMetadata(oc.UserLogin).OpenCodeInstances
}

func (oc *OpenCodeClient) SaveOpenCodeInstances(ctx context.Context, instances map[string]*opencodebridge.OpenCodeInstance) error {
	if oc == nil || oc.UserLogin == nil {
		return nil
	}
	meta := loginMetadata(oc.UserLogin)
	meta.OpenCodeInstances = instances
	return oc.UserLogin.Save(ctx)
}

func (oc *OpenCodeClient) HumanUserID(loginID networkid.UserLoginID) networkid.UserID {
	return humanUserID(loginID)
}

func (oc *OpenCodeClient) RoomCapabilitiesEventType() event.Type {
	return matrixevents.RoomCapabilitiesEventType
}

func (oc *OpenCodeClient) RoomSettingsEventType() event.Type {
	return matrixevents.RoomSettingsEventType
}

package bridgeadapter

import (
	"context"
	"fmt"
	"time"

	"github.com/beeper/ai-bridge/pkg/matrixtransport"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Adapter is a thin wrapper that makes a bridgev2 MatrixAPI look like a matrixtransport.Transport.
//
// This will be expanded as connector logic is migrated into pkg/airuntime.
type Adapter struct {
	Intent bridgev2.MatrixAPI
	Bot    bridgev2.MatrixAPI // optional; used for APIs that only exist on the bot in bridgev2
}

var _ matrixtransport.Transport = (*Adapter)(nil)

type ephemeralSender interface {
	SendEphemeralEvent(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, txnID string) (*mautrix.RespSendEvent, error)
}

func (a *Adapter) SendMessage(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, txnID string) (id.EventID, error) {
	if a == nil || a.Intent == nil {
		return "", fmt.Errorf("missing intent")
	}
	resp, err := a.Intent.SendMessage(ctx, roomID, eventType, content, nil)
	if err != nil {
		return "", err
	}
	return resp.EventID, nil
}

func (a *Adapter) EditMessage(ctx context.Context, roomID id.RoomID, targetEventID id.EventID, newContent *event.Content, txnID string) (id.EventID, error) {
	// Bridgev2 edit handling differs per connector; for now callers should send
	// m.replace content themselves and use SendMessage.
	return "", fmt.Errorf("EditMessage not implemented: send m.replace via SendMessage")
}

func (a *Adapter) SendEphemeral(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, txnID string) error {
	if a == nil || a.Intent == nil {
		return fmt.Errorf("missing intent")
	}
	es, ok := a.Intent.(ephemeralSender)
	if !ok {
		return fmt.Errorf("intent does not support ephemeral events")
	}
	_, err := es.SendEphemeralEvent(ctx, roomID, eventType, content, txnID)
	return err
}

func (a *Adapter) SendMessageStatus(ctx context.Context, roomID id.RoomID, targetEventID id.EventID, statusPayload map[string]any) error {
	// Status events are bridge-internal; connector will continue to use bridgev2 directly
	// until we extract a proper abstraction.
	return nil
}

func (a *Adapter) MarkTyping(ctx context.Context, roomID id.RoomID, typingType string, timeout time.Duration) error {
	if a == nil || a.Intent == nil {
		return nil
	}
	return a.Intent.MarkTyping(ctx, roomID, bridgev2.TypingTypeText, timeout)
}

func (a *Adapter) UploadMedia(ctx context.Context, data []byte, mimeType, filename string) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	return "", nil, fmt.Errorf("UploadMedia not implemented")
}

func (a *Adapter) DownloadMedia(ctx context.Context, uri id.ContentURIString, encryptedFile *event.EncryptedFileInfo) ([]byte, string, error) {
	return nil, "", fmt.Errorf("DownloadMedia not implemented")
}

func (a *Adapter) GetRoomState(ctx context.Context, roomID id.RoomID, eventType event.Type, stateKey string) (*event.Event, error) {
	// State access isn't available on MatrixAPI; bridge adapter will need a bridgev2.MatrixConnector.
	return nil, fmt.Errorf("GetRoomState not implemented")
}

func (a *Adapter) SetRoomState(ctx context.Context, roomID id.RoomID, eventType event.Type, stateKey string, content *event.Content) (id.EventID, error) {
	if a == nil || a.Intent == nil {
		return "", fmt.Errorf("missing intent")
	}
	resp, err := a.Intent.SendState(ctx, roomID, eventType, stateKey, content, time.Now())
	if err != nil {
		return "", err
	}
	return resp.EventID, nil
}

func (a *Adapter) GetMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error) {
	if a == nil || a.Intent == nil {
		return nil, fmt.Errorf("missing intent")
	}
	// MatrixAPI doesn't expose member list; connector has direct access via bridge Matrix connector.
	return nil, fmt.Errorf("GetMembers not implemented")
}

func (a *Adapter) GetMemberProfile(ctx context.Context, roomID id.RoomID, userID id.UserID) (*event.MemberEventContent, error) {
	return nil, fmt.Errorf("GetMemberProfile not implemented")
}

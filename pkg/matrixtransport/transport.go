package matrixtransport

import (
	"context"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Transport abstracts Matrix IO for shared runtime logic.
//
// Implementations:
// - bridge adapter (bridgev2 portal/intent)
// - bot adapter (mautrix client + crypto)
type Transport interface {
	SendMessage(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, txnID string) (id.EventID, error)
	EditMessage(ctx context.Context, roomID id.RoomID, targetEventID id.EventID, newContent *event.Content, txnID string) (id.EventID, error)
	SendEphemeral(ctx context.Context, roomID id.RoomID, eventType event.Type, content *event.Content, txnID string) error

	SendMessageStatus(ctx context.Context, roomID id.RoomID, targetEventID id.EventID, statusPayload map[string]any) error

	MarkTyping(ctx context.Context, roomID id.RoomID, typingType string, timeout time.Duration) error

	UploadMedia(ctx context.Context, data []byte, mimeType, filename string) (mxc id.ContentURIString, encryptedFile *event.EncryptedFileInfo, err error)
	DownloadMedia(ctx context.Context, uri id.ContentURIString, encryptedFile *event.EncryptedFileInfo) (data []byte, mimeType string, err error)

	GetRoomState(ctx context.Context, roomID id.RoomID, eventType event.Type, stateKey string) (*event.Event, error)
	SetRoomState(ctx context.Context, roomID id.RoomID, eventType event.Type, stateKey string, content *event.Content) (id.EventID, error)

	GetMembers(ctx context.Context, roomID id.RoomID) ([]id.UserID, error)
	GetMemberProfile(ctx context.Context, roomID id.RoomID, userID id.UserID) (*event.MemberEventContent, error)
}

// EventLoop receives decrypted/normalized events from the transport layer.
// Shared runtime implementations subscribe here to drive behavior.
type EventLoop interface {
	OnEvent(ctx context.Context, evt *event.Event) error
}


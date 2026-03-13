package sdk

import (
	"context"
	"time"

	"go.mau.fi/util/configupgrade"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
)

// MessageType identifies the kind of message.
type MessageType string

const (
	MessageText  MessageType = "text"
	MessageImage MessageType = "image"
	MessageAudio MessageType = "audio"
	MessageVideo MessageType = "video"
	MessageFile  MessageType = "file"
)

// Message represents an incoming user message.
type Message struct {
	ID        string
	Text      string
	HTML      string
	MediaURL  string      // MXC URL for media messages
	MediaType string      // MIME type
	MsgType   MessageType // Text, Image, Audio, Video, File
	Sender    string
	ReplyTo   string // event ID being replied to
	Timestamp time.Time
	Metadata  map[string]any

	// Escape hatches for power users.
	RawEvent *event.Event
	RawMsg   *bridgev2.MatrixMessage
}

// MessageEdit represents an edit to a previously sent message.
type MessageEdit struct {
	OriginalID string
	NewText    string
	NewHTML    string
	RawEdit    *bridgev2.MatrixEdit
}

// Reaction represents a user reaction on a message.
type Reaction struct {
	MessageID string
	Emoji     string
	Sender    string
	RawMsg    *bridgev2.MatrixReaction
}

// LoginInfo contains information about a bridge login.
type LoginInfo struct {
	UserID   string
	Domain   string
	Login    *bridgev2.UserLogin // escape hatch
	Metadata map[string]any
}

// UserInfo describes a user/agent/model for search results.
type UserInfo struct {
	ID       string
	Name     string
	Avatar   string
	Metadata map[string]any
}

// ChatInfo describes a chat/portal.
type ChatInfo struct {
	ID       string
	Name     string
	Topic    string
	Metadata map[string]any
}

// CreateChatParams contains parameters for creating a new chat.
type CreateChatParams struct {
	UserID   string
	Name     string
	Metadata map[string]any
}

// ToolApprovalResponse is the user's decision on a tool approval request.
type ToolApprovalResponse struct {
	Approved bool
	Always   bool   // "always allow this tool"
	Reason   string // allow_once, allow_always, deny, timeout, expired
}

// Command defines a slash command that users can invoke.
type Command struct {
	Name        string
	Description string
	Args        string // e.g. "<query>", "[options...]"
	Handler     func(conv *Conversation, args string) error
}

// RoomFeatures describes what a room supports.
type RoomFeatures struct {
	MaxTextLength        int
	SupportsImages       bool
	SupportsAudio        bool
	SupportsVideo        bool
	SupportsFiles        bool
	SupportsReply        bool
	SupportsEdit         bool
	SupportsDelete       bool
	SupportsReactions    bool
	SupportsTyping       bool
	SupportsReadReceipts bool
	SupportsDeleteChat   bool
	CustomCapabilityID   string             // for dynamic capability IDs
	Custom               *event.RoomFeatures // escape hatch: override everything
}


// ModelInfo describes an AI model.
type ModelInfo struct {
	ID           string
	Name         string
	Provider     string
	Capabilities []string
}

// Config configures the SDK bridge.
type Config struct {
	// Required
	Name        string
	Description string

	// Agent identity (optional, used for ghost sender)
	Agent *AgentMember

	// Message handling (required)
	// session is the value returned by OnConnect; conv is the conversation;
	// msg is the incoming message; turn is the pre-created Turn for streaming responses.
	OnMessage func(session any, conv *Conversation, msg *Message, turn *Turn) error

	// Event hooks (optional)
	OnConnect    func(ctx context.Context, login *LoginInfo) (any, error) // returns session state
	OnDisconnect func(session any)
	OnReaction   func(session any, conv *Conversation, reaction *Reaction) error
	OnTyping     func(session any, conv *Conversation, typing bool)
	OnEdit       func(session any, conv *Conversation, edit *MessageEdit) error
	OnDelete     func(session any, conv *Conversation, msgID string) error
	OnRoomName   func(session any, conv *Conversation, name string) (bool, error)
	OnRoomTopic  func(session any, conv *Conversation, topic string) (bool, error)

	// Turn management (optional)
	TurnManagement *TurnConfig

	// Capabilities (optional, dynamic per-conversation)
	GetCapabilities func(session any, conv *Conversation) *RoomFeatures

	// Search & chat ops (optional)
	SearchUsers       func(query string) ([]*UserInfo, error)
	GetContactList    func() ([]*UserInfo, error)
	ResolveIdentifier func(id string) (*UserInfo, error)
	CreateChat        func(params *CreateChatParams) (*ChatInfo, error)
	DeleteChat        func(conv *Conversation) error
	GetChatInfo       func(conv *Conversation) (*bridgev2.ChatInfo, error)
	GetUserInfo       func(ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error)
	IsThisUser        func(userID string) bool

	// Commands
	Commands []Command

	// Room features (static default; overridden by GetCapabilities if set)
	RoomFeatures *RoomFeatures // nil = AI agent defaults

	// Login — use bridgev2 types directly.
	LoginFlows  []bridgev2.LoginFlow                                                                    // nil = single auto-login
	CreateLogin func(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) // nil = auto-login

	// Backfill — use bridgev2 types directly.
	FetchMessages func(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) // nil = no backfill

	// Import turns for backfill (optional, session-aware)
	ImportTurns func(session any, conv *Conversation, params BackfillParams) ([]*ImportedTurn, error)

	// Advanced
	ProtocolID     string                     // default: "sdk-<Name>"
	Port           int                        // default: 29400
	DBName         string                     // default: "<Name>.db"
	ConfigPath     string                     // default: auto-discover
	DBMeta         func() database.MetaTypes  // nil = default
	ExampleConfig  string                     // YAML
	ConfigData     any                        // config struct pointer
	ConfigUpgrader configupgrade.Upgrader
}

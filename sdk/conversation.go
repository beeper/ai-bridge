package sdk

import (
	"context"
	"fmt"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

// Conversation represents a chat room the agent is participating in.
type Conversation struct {
	ID    string
	Title string

	ctx    context.Context
	portal *bridgev2.Portal
	login  *bridgev2.UserLogin
	sender bridgev2.EventSender
	client *sdkClient
}

func newConversation(ctx context.Context, portal *bridgev2.Portal, login *bridgev2.UserLogin, sender bridgev2.EventSender, client *sdkClient) *Conversation {
	id := ""
	if portal != nil {
		id = string(portal.ID)
	}
	return &Conversation{
		ID:     id,
		ctx:    ctx,
		portal: portal,
		login:  login,
		sender: sender,
		client: client,
	}
}

func (c *Conversation) getIntent(ctx context.Context) (bridgev2.MatrixAPI, error) {
	if c.portal == nil || c.login == nil {
		return nil, fmt.Errorf("no portal or login")
	}
	intent, ok := c.portal.GetIntentFor(ctx, c.sender, c.login, bridgev2.RemoteEventMessage)
	if !ok || intent == nil {
		return nil, fmt.Errorf("failed to get intent")
	}
	return intent, nil
}

// Send sends a complete text message.
func (c *Conversation) Send(ctx context.Context, text string) error {
	return c.SendHTML(ctx, text, "")
}

// SendHTML sends a message with both plaintext and HTML body.
func (c *Conversation) SendHTML(ctx context.Context, text, html string) error {
	intent, err := c.getIntent(ctx)
	if err != nil {
		return err
	}
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
	}
	if html != "" {
		content.Format = event.FormatHTML
		content.FormattedBody = html
	}
	wrappedContent := &event.Content{Parsed: content}
	_, err = intent.SendMessage(ctx, c.portal.MXID, event.EventMessage, wrappedContent, nil)
	return err
}

// SendMedia sends a media message.
func (c *Conversation) SendMedia(ctx context.Context, data []byte, mediaType, filename string) error {
	intent, err := c.getIntent(ctx)
	if err != nil {
		return err
	}
	mxcURL, encFile, err := intent.UploadMedia(ctx, c.portal.MXID, data, filename, mediaType)
	if err != nil {
		return err
	}
	msgType := event.MsgFile
	switch {
	case len(mediaType) > 5 && mediaType[:6] == "image/":
		msgType = event.MsgImage
	case len(mediaType) > 5 && mediaType[:6] == "audio/":
		msgType = event.MsgAudio
	case len(mediaType) > 5 && mediaType[:6] == "video/":
		msgType = event.MsgVideo
	}
	content := &event.MessageEventContent{
		MsgType: msgType,
		Body:    filename,
		Info: &event.FileInfo{
			MimeType: mediaType,
			Size:     len(data),
		},
	}
	if encFile != nil {
		content.File = encFile
	} else {
		content.URL = mxcURL
	}
	wrappedContent := &event.Content{Parsed: content}
	_, err = intent.SendMessage(ctx, c.portal.MXID, event.EventMessage, wrappedContent, nil)
	return err
}

// SendNotice sends a notice message.
func (c *Conversation) SendNotice(ctx context.Context, text string) error {
	intent, err := c.getIntent(ctx)
	if err != nil {
		return err
	}
	content := &event.MessageEventContent{
		MsgType: event.MsgNotice,
		Body:    text,
	}
	wrappedContent := &event.Content{Parsed: content}
	_, err = intent.SendMessage(ctx, c.portal.MXID, event.EventMessage, wrappedContent, nil)
	return err
}

// Stream starts a new streaming response in this conversation.
func (c *Conversation) Stream(ctx context.Context) *Stream {
	return newStream(ctx, c)
}

// StartTurn creates a new Turn for this conversation with the default agent.
func (c *Conversation) StartTurn(ctx context.Context) *Turn {
	return newTurn(ctx, c, nil)
}

// StartTurnWithAgent creates a new Turn for this conversation with a specific agent.
func (c *Conversation) StartTurnWithAgent(ctx context.Context, agent *AgentMember) *Turn {
	return newTurn(ctx, c, agent)
}

// Session returns the session state from the client, if available.
func (c *Conversation) Session() any {
	if c.client == nil {
		return nil
	}
	return c.client.getSession()
}

// Context returns the conversation's context.
func (c *Conversation) Context() context.Context {
	return c.ctx
}

// SetTyping sets the typing indicator for this conversation.
func (c *Conversation) SetTyping(ctx context.Context, typing bool) error {
	intent, err := c.getIntent(ctx)
	if err != nil {
		return err
	}
	timeout := 30 * time.Second
	if !typing {
		timeout = 0
	}
	return intent.MarkTyping(ctx, c.portal.MXID, bridgev2.TypingTypeText, timeout)
}

// SetRoomName sets the room name.
func (c *Conversation) SetRoomName(ctx context.Context, name string) error {
	intent, err := c.getIntent(ctx)
	if err != nil {
		return err
	}
	content := &event.Content{Parsed: &event.RoomNameEventContent{Name: name}}
	_, err = intent.SendState(ctx, c.portal.MXID, event.StateRoomName, "", content, time.Time{})
	return err
}

// SetRoomTopic sets the room topic.
func (c *Conversation) SetRoomTopic(ctx context.Context, topic string) error {
	intent, err := c.getIntent(ctx)
	if err != nil {
		return err
	}
	content := &event.Content{Parsed: &event.TopicEventContent{Topic: topic}}
	_, err = intent.SendState(ctx, c.portal.MXID, event.StateTopic, "", content, time.Time{})
	return err
}

// BroadcastCapabilities sends room capability state events.
func (c *Conversation) BroadcastCapabilities(ctx context.Context, features *RoomFeatures) error {
	if features == nil {
		return nil
	}
	intent, err := c.getIntent(ctx)
	if err != nil {
		return err
	}
	rf := convertRoomFeatures(features)
	content := &event.Content{Parsed: rf}
	_, err = intent.SendState(ctx, c.portal.MXID, event.StateBeeperRoomFeatures, "", content, time.Time{})
	return err
}

// Portal returns the underlying bridgev2.Portal.
func (c *Conversation) Portal() *bridgev2.Portal { return c.portal }

// Login returns the underlying bridgev2.UserLogin.
func (c *Conversation) Login() *bridgev2.UserLogin { return c.login }

// Sender returns the event sender for this conversation.
func (c *Conversation) Sender() bridgev2.EventSender { return c.sender }

// QueueRemoteEvent queues a remote event for processing.
func (c *Conversation) QueueRemoteEvent(evt bridgev2.RemoteEvent) {
	if c.login != nil {
		c.login.Bridge.QueueRemoteEvent(c.login, evt)
	}
}

// Intent returns the Matrix API intent for sending events.
func (c *Conversation) Intent(ctx context.Context) (bridgev2.MatrixAPI, error) {
	return c.getIntent(ctx)
}

package sdk

import (
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"

	"github.com/beeper/agentremote"
)

// ImportedTurn represents a historical turn for backfill.
type ImportedTurn struct {
	ID           string
	Role         string // "user", "assistant", "system"
	Text         string
	HTML         string
	Reasoning    string
	ToolCalls    []ImportedToolCall
	Citations    []ImportedCitation
	Files        []ImportedFile
	Agent        *AgentMember
	Sender       bridgev2.EventSender
	Timestamp    time.Time
	Metadata     map[string]any
	FinishReason string
}

// ImportedToolCall represents a tool call in a historical turn.
type ImportedToolCall struct {
	ID     string
	Name   string
	Input  string
	Output string
}

// ImportedCitation represents a citation in a historical turn.
type ImportedCitation struct {
	URL   string
	Title string
}

// ImportedFile represents a file attachment in a historical turn.
type ImportedFile struct {
	URL       string
	MediaType string
}

// BackfillParams configures a backfill request.
type BackfillParams struct {
	Forward         bool
	Count           int
	AnchorTimestamp time.Time
}

// ConvertImportedTurns converts imported turns into bridgev2.BackfillMessage values.
func ConvertImportedTurns(turns []*ImportedTurn, idPrefix string) []*bridgev2.BackfillMessage {
	if len(turns) == 0 {
		return nil
	}
	messages := make([]*bridgev2.BackfillMessage, 0, len(turns))
	for _, turn := range turns {
		if turn == nil {
			continue
		}
		msg := convertImportedTurn(turn, idPrefix)
		if msg != nil {
			messages = append(messages, msg)
		}
	}
	return messages
}

func convertImportedTurn(turn *ImportedTurn, idPrefix string) *bridgev2.BackfillMessage {
	msgID := turn.ID
	if msgID == "" {
		msgID = string(agentremote.NewMessageID(idPrefix))
	}

	body := turn.Text
	htmlBody := turn.HTML
	if htmlBody == "" && body != "" {
		rendered := format.RenderMarkdown(body, true, true)
		htmlBody = rendered.FormattedBody
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    body,
	}
	if htmlBody != "" {
		content.Format = event.FormatHTML
		content.FormattedBody = htmlBody
	}

	// Build metadata.
	meta := &agentremote.BaseMessageMetadata{
		Role:         turn.Role,
		Body:         body,
		FinishReason: turn.FinishReason,
		TurnID:       turn.ID,
	}
	if turn.Reasoning != "" {
		meta.ThinkingContent = turn.Reasoning
	}
	if turn.Agent != nil {
		meta.AgentID = turn.Agent.ID
	}

	// Convert tool calls.
	if len(turn.ToolCalls) > 0 {
		meta.ToolCalls = make([]agentremote.ToolCallMetadata, len(turn.ToolCalls))
		for i, tc := range turn.ToolCalls {
			meta.ToolCalls[i] = agentremote.ToolCallMetadata{
				CallID:   tc.ID,
				ToolName: tc.Name,
				Status:   "completed",
			}
		}
	}

	ts := turn.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	return &bridgev2.BackfillMessage{
		ConvertedMessage: &bridgev2.ConvertedMessage{
			Parts: []*bridgev2.ConvertedMessagePart{{
				ID:         networkid.PartID("0"),
				Type:       event.EventMessage,
				Content:    content,
				DBMetadata: meta,
			}},
		},
		Sender:    turn.Sender,
		Timestamp: ts,
		ID:        networkid.MessageID(msgID),
	}
}

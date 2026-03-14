package sdk

import (
	"time"

	"maunium.net/go/mautrix/bridgev2"
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
	Agent        *Agent
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

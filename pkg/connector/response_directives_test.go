package connector

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestParseResponseDirectives_Silent(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSilent bool
	}{
		{"exact match", "NO_REPLY", true},
		{"with trailing space", "NO_REPLY ", true},
		{"with leading space", " NO_REPLY", true},
		{"at start with text", "NO_REPLY some extra", true},
		{"at end", "some text NO_REPLY", true},
		{"in middle", "before NO_REPLY after", false},
		{"not present", "Hello world", false},
		{"empty", "", true},
		{"partial match", "NO_REPL", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseResponseDirectives(tt.input, "")
			if result.IsSilent != tt.wantSilent {
				t.Errorf("ParseResponseDirectives(%q).IsSilent = %v, want %v", tt.input, result.IsSilent, tt.wantSilent)
			}
		})
	}
}

func TestParseResponseDirectives_ReplyTags(t *testing.T) {
	sourceEventID := id.EventID("$source123")

	tests := []struct {
		name           string
		input          string
		sourceEventID  id.EventID
		wantReplyTo    id.EventID
		wantCurrent    bool
		wantHasTag     bool
		wantCleanedLen int
	}{
		{
			name:           "reply_to_current",
			input:          "Hello [[reply_to_current]] world",
			sourceEventID:  sourceEventID,
			wantReplyTo:    sourceEventID,
			wantCurrent:    true,
			wantHasTag:     true,
			wantCleanedLen: len("Hello world"),
		},
		{
			name:          "reply_to_current with whitespace",
			input:         "Hello [[ reply_to_current ]] world",
			sourceEventID: sourceEventID,
			wantReplyTo:   sourceEventID,
			wantCurrent:   true,
			wantHasTag:    true,
		},
		{
			name:          "explicit reply_to",
			input:         "Reply [[reply_to:$abc123]] here",
			sourceEventID: sourceEventID,
			wantReplyTo:   id.EventID("$abc123"),
			wantCurrent:   false,
			wantHasTag:    true,
		},
		{
			name:          "explicit takes precedence",
			input:         "[[reply_to_current]] and [[reply_to:$explicit]]",
			sourceEventID: sourceEventID,
			wantReplyTo:   id.EventID("$explicit"),
			wantCurrent:   false, // sawCurrent is true but explicit overrides
			wantHasTag:    true,
		},
		{
			name:          "no reply tag",
			input:         "Just plain text",
			sourceEventID: sourceEventID,
			wantReplyTo:   "",
			wantCurrent:   false,
			wantHasTag:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseResponseDirectives(tt.input, tt.sourceEventID)
			if result.ReplyToEventID != tt.wantReplyTo {
				t.Errorf("ReplyToEventID = %q, want %q", result.ReplyToEventID, tt.wantReplyTo)
			}
			if result.HasReplyTag != tt.wantHasTag {
				t.Errorf("HasReplyTag = %v, want %v", result.HasReplyTag, tt.wantHasTag)
			}
		})
	}
}

// Note: Reaction tags are NOT supported - reactions are handled via the message tool.
// This matches OpenClaw's approach where reactions require tool calls.

func TestParseResponseDirectives_CleanedText(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCleaned string
	}{
		{
			name:        "strips reply tag",
			input:       "Hello [[reply_to_current]] world",
			wantCleaned: "Hello world",
		},
		{
			name:        "strips multiple reply tags",
			input:       "[[reply_to_current]] Hello [[reply_to:$abc]] world",
			wantCleaned: "Hello world",
		},
		{
			name:        "normalizes whitespace",
			input:       "Hello    world",
			wantCleaned: "Hello world",
		},
		{
			name:        "preserves reaction-like text (not a directive)",
			input:       "Great! [[react:👍]]",
			wantCleaned: "Great! [[react:👍]]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseResponseDirectives(tt.input, "")
			if result.Text != tt.wantCleaned {
				t.Errorf("Text = %q, want %q", result.Text, tt.wantCleaned)
			}
		})
	}
}

func TestIsSilentReplyText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact", "NO_REPLY", true},
		{"prefix", "NO_REPLY -- reason", true},
		{"suffix", "some context NO_REPLY", true},
		{"middle - not silent", "before NO_REPLY after", false},
		{"not present", "Hello", false},
		{"empty", "", false},
		{"partial", "NO_REPL", false},
		{"with newline suffix", "NO_REPLY\n", true},
		// Markup handling (clawdbot parity)
		{"bold markdown", "**NO_REPLY**", true},
		{"italic markdown", "*NO_REPLY*", true},
		{"backtick markdown", "`NO_REPLY`", true},
		{"html bold", "<b>NO_REPLY</b>", true},
		{"html italic", "<i>NO_REPLY</i>", true},
		{"html span", "<span>NO_REPLY</span>", true},
		{"mixed formatting", "**`NO_REPLY`**", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSilentReplyText(tt.input); got != tt.want {
				t.Errorf("isSilentReplyText(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

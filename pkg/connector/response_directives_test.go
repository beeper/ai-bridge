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

func TestParseResponseDirectives_Reactions(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantReactions int
		wantEmoji     string
		wantEventID   id.EventID
	}{
		{
			name:          "single reaction",
			input:         "Great job! [[react:👍]]",
			wantReactions: 1,
			wantEmoji:     "👍",
			wantEventID:   "",
		},
		{
			name:          "reaction with event ID",
			input:         "[[react:🎉:$event123]]",
			wantReactions: 1,
			wantEmoji:     "🎉",
			wantEventID:   id.EventID("$event123"),
		},
		{
			name:          "multiple reactions",
			input:         "[[react:👍]] [[react:❤️]]",
			wantReactions: 2,
			wantEmoji:     "👍", // First reaction
			wantEventID:   "",
		},
		{
			name:          "no reactions",
			input:         "Just text",
			wantReactions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseResponseDirectives(tt.input, "")
			if len(result.Reactions) != tt.wantReactions {
				t.Errorf("len(Reactions) = %d, want %d", len(result.Reactions), tt.wantReactions)
			}
			if tt.wantReactions > 0 && len(result.Reactions) > 0 {
				if result.Reactions[0].Emoji != tt.wantEmoji {
					t.Errorf("Reactions[0].Emoji = %q, want %q", result.Reactions[0].Emoji, tt.wantEmoji)
				}
				if result.Reactions[0].EventID != tt.wantEventID {
					t.Errorf("Reactions[0].EventID = %q, want %q", result.Reactions[0].EventID, tt.wantEventID)
				}
			}
		})
	}
}

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
			name:        "strips reaction tag",
			input:       "Great! [[react:👍]]",
			wantCleaned: "Great!",
		},
		{
			name:        "strips multiple tags",
			input:       "[[reply_to_current]] Hello [[react:👍]] world [[react:❤️]]",
			wantCleaned: "Hello world",
		},
		{
			name:        "normalizes whitespace",
			input:       "Hello    world",
			wantCleaned: "Hello world",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSilentReplyText(tt.input); got != tt.want {
				t.Errorf("isSilentReplyText(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

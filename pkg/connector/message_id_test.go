package connector

import "testing"

func TestStripMessageIDHintLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "removes message_id line",
			input: "hello\n[message_id: $abc:example.com]\nworld",
			want:  "hello\nworld",
		},
		{
			name:  "removes matrix event id line",
			input: "hello\n[matrix event id: $abc:example.com room: !room:example.com]\nworld",
			want:  "hello\nworld",
		},
		{
			name:  "keeps inline hint text",
			input: "hello [message_id: $abc:example.com] world",
			want:  "hello [message_id: $abc:example.com] world",
		},
		{
			name:  "keeps text without hints",
			input: "hello world",
			want:  "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripMessageIDHintLines(tt.input)
			if got != tt.want {
				t.Fatalf("stripMessageIDHintLines(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeMessageID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "message_id line",
			input: "[message_id: $abc:example.com]",
			want:  "$abc:example.com",
		},
		{
			name:  "message_id inline",
			input: "reply to [message_id: $abc:example.com] please",
			want:  "$abc:example.com",
		},
		{
			name:  "matrix event id line",
			input: "[matrix event id: $abc:example.com room: !room:example.com]",
			want:  "$abc:example.com",
		},
		{
			name:  "quoted message_id hint value",
			input: "[message_id: `$abc:example.com`]",
			want:  "$abc:example.com",
		},
		{
			name:  "sample payload body with trailing hint line",
			input: "**Ping #8!**\n\n8 down, 7 to go. Cruising.\n[message_id: $3wY5J8g8XTHBWK2a8NMwLGqDKnSXb5ckBNOPFXe1SHQmfGZ7CepYYcXabOG0XBPQ:.local-ai.localhost]",
			want:  "$3wY5J8g8XTHBWK2a8NMwLGqDKnSXb5ckBNOPFXe1SHQmfGZ7CepYYcXabOG0XBPQ:.local-ai.localhost",
		},
		{
			name:  "invalid hint with spaces in id",
			input: "[message_id: $abc:example.com extra]",
			want:  "",
		},
		{
			name:  "invalid hint returns empty",
			input: "text [message_id: bad id]",
			want:  "",
		},
		{
			name:  "plain id remains unchanged",
			input: "desktop-msg-123",
			want:  "desktop-msg-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeMessageID(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeMessageID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeMessageArgsCanonicalMessageID(t *testing.T) {
	args := map[string]any{
		"action":     "reply",
		"message_id": "[message_id: $abc:example.com]",
	}
	normalizeMessageArgs(args)
	got, _ := args["message_id"].(string)
	if got != "$abc:example.com" {
		t.Fatalf("normalized message_id = %q, want %q", got, "$abc:example.com")
	}
}

func TestNormalizeMessageArgsIgnoresLegacyAliasFields(t *testing.T) {
	args := map[string]any{
		"action":    "reply",
		"messageId": "[message_id: $abc:example.com]",
		"replyTo":   "[message_id: $def:example.com]",
	}
	normalizeMessageArgs(args)
	got, _ := args["message_id"].(string)
	if got != "" {
		t.Fatalf("expected legacy alias fields to be ignored, got message_id=%q", got)
	}
}

package agents

import "testing"

func TestParseIdentityMarkdownIgnoresPlaceholders(t *testing.T) {
	content := `
- **Name:** (pick something you like)
- **Emoji:** (your signature — pick one that feels right)
`
	identity := ParseIdentityMarkdown(content)
	if IdentityHasValues(identity) {
		t.Fatalf("expected placeholders to be ignored, got %+v", identity)
	}
}

func TestParseIdentityMarkdownParsesValues(t *testing.T) {
	content := `
- **Name:** OpenClaw
- **Emoji:** 🦞
- **Vibe:** warm and sharp
`
	identity := ParseIdentityMarkdown(content)
	if identity.Name != "OpenClaw" {
		t.Fatalf("expected name OpenClaw, got %q", identity.Name)
	}
	if identity.Emoji != "🦞" {
		t.Fatalf("expected emoji 🦞, got %q", identity.Emoji)
	}
	if identity.Vibe != "warm and sharp" {
		t.Fatalf("expected vibe 'warm and sharp', got %q", identity.Vibe)
	}
}

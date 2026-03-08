package openclaw

import "testing"

func TestOpenClawAgentIDFromSessionKey(t *testing.T) {
	if got := openClawAgentIDFromSessionKey("agent:main:discord:channel:123"); got != "main" {
		t.Fatalf("expected main, got %q", got)
	}
	if got := openClawAgentIDFromSessionKey("main"); got != "" {
		t.Fatalf("expected empty agent id, got %q", got)
	}
}

func TestExtractMessageTextOpenResponsesParts(t *testing.T) {
	msg := map[string]any{
		"content": []any{
			map[string]any{"type": "input_text", "text": "hello"},
			map[string]any{"type": "output_text", "text": "world"},
		},
	}
	if got := extractMessageText(msg); got != "hello\n\nworld" {
		t.Fatalf("unexpected extracted text: %q", got)
	}
}

func TestOpenClawAttachmentSourceFromBlock(t *testing.T) {
	block := map[string]any{
		"type": "input_file",
		"source": map[string]any{
			"type":       "base64",
			"media_type": "image/png",
			"data":       "Zm9v",
			"filename":   "dot.png",
		},
	}
	source := openClawAttachmentSourceFromBlock(block)
	if source == nil {
		t.Fatal("expected source")
	}
	if source.Kind != "base64" || source.FileName != "dot.png" || source.MimeType != "image/png" {
		t.Fatalf("unexpected source: %#v", source)
	}
}

func TestIsOpenClawAttachmentBlock(t *testing.T) {
	if isOpenClawAttachmentBlock(map[string]any{"type": "output_text", "text": "hello"}) {
		t.Fatal("output_text should not be treated as attachment")
	}
	if isOpenClawAttachmentBlock(map[string]any{"type": "toolCall", "id": "call-1"}) {
		t.Fatal("toolCall should not be treated as attachment")
	}
	if !isOpenClawAttachmentBlock(map[string]any{
		"type":   "input_file",
		"source": map[string]any{"type": "url", "url": "https://example.com/file.txt"},
	}) {
		t.Fatal("input_file should be treated as attachment")
	}
}

func TestOpenClawHistoryUIPartsToolCall(t *testing.T) {
	parts := openClawHistoryUIParts(map[string]any{
		"content": []any{
			map[string]any{
				"type":      "toolCall",
				"id":        "call-1",
				"name":      "bash",
				"arguments": map[string]any{"cmd": "ls"},
			},
		},
	}, "assistant")
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0]["type"] != "dynamic-tool" || parts[0]["toolCallId"] != "call-1" {
		t.Fatalf("unexpected part: %#v", parts[0])
	}
}

func TestOpenClawHistoryUIPartsToolResult(t *testing.T) {
	parts := openClawHistoryUIParts(map[string]any{
		"toolCallId": "call-1",
		"toolName":   "bash",
		"isError":    false,
		"details":    map[string]any{"stdout": "ok"},
		"content":    []any{map[string]any{"type": "text", "text": "ok"}},
	}, "toolresult")
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0]["state"] != "output-available" {
		t.Fatalf("unexpected tool result part: %#v", parts[0])
	}
}

package openclaw

import (
	"context"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
)

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

func TestOpenClawHistoryUIPartsReasoningAndApproval(t *testing.T) {
	parts := openClawHistoryUIParts(map[string]any{
		"content": []any{
			map[string]any{"type": "reasoning", "text": "checking context"},
			map[string]any{
				"type":       "toolCall",
				"id":         "call-9",
				"name":       "exec",
				"arguments":  map[string]any{"cmd": "pwd"},
				"approvalId": "approval-1",
			},
		},
	}, "assistant")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0]["type"] != "reasoning" || parts[0]["text"] != "checking context" {
		t.Fatalf("unexpected reasoning part: %#v", parts[0])
	}
	if parts[1]["type"] != "dynamic-tool" || parts[1]["state"] != "approval-requested" {
		t.Fatalf("unexpected tool approval part: %#v", parts[1])
	}
}

func TestConvertHistoryToCanonicalUIMetadata(t *testing.T) {
	meta := &PortalMetadata{
		OpenClawSessionID:  "sess-1",
		OpenClawSessionKey: "agent:main:matrix-dm",
		Model:              "gpt-5",
	}
	parts, metadata := convertHistoryToCanonicalUI(map[string]any{
		"role":         "assistant",
		"runId":        "run-1",
		"finishReason": "completed",
		"usage": map[string]any{
			"inputTokens":     int64(4),
			"outputTokens":    int64(6),
			"reasoningTokens": int64(2),
			"totalTokens":     int64(12),
		},
		"content": []any{map[string]any{"type": "text", "text": "hello"}},
	}, "assistant", meta)
	if len(parts) != 1 || parts[0]["type"] != "text" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
	if metadata["session_id"] != "sess-1" || metadata["session_key"] != "agent:main:matrix-dm" {
		t.Fatalf("unexpected session metadata: %#v", metadata)
	}
	usage, ok := metadata["usage"].(map[string]any)
	if !ok {
		t.Fatalf("expected usage metadata, got %#v", metadata["usage"])
	}
	if usage["prompt_tokens"] != int64(4) || usage["completion_tokens"] != int64(6) || usage["reasoning_tokens"] != int64(2) || usage["total_tokens"] != int64(12) {
		t.Fatalf("unexpected usage metadata: %#v", usage)
	}
}

func TestNormalizeOpenClawUsage(t *testing.T) {
	usage := normalizeOpenClawUsage(map[string]any{
		"input":           float64(10),
		"outputTokens":    int64(4),
		"reasoningTokens": int64(2),
		"total":           int64(16),
	})
	if usage["prompt_tokens"] != int64(10) {
		t.Fatalf("expected prompt_tokens=10, got %#v", usage["prompt_tokens"])
	}
	if usage["completion_tokens"] != int64(4) {
		t.Fatalf("expected completion_tokens=4, got %#v", usage["completion_tokens"])
	}
	if usage["reasoning_tokens"] != int64(2) {
		t.Fatalf("expected reasoning_tokens=2, got %#v", usage["reasoning_tokens"])
	}
	if usage["total_tokens"] != int64(16) {
		t.Fatalf("expected total_tokens=16, got %#v", usage["total_tokens"])
	}
}

func TestOpenClawAttachmentSourceFromNestedFileMap(t *testing.T) {
	block := map[string]any{
		"type": "file",
		"file": map[string]any{
			"url":      "https://example.com/doc.txt",
			"mimeType": "text/plain",
			"name":     "doc.txt",
		},
	}
	source := openClawAttachmentSourceFromBlock(block)
	if source == nil {
		t.Fatal("expected source")
	}
	if source.Kind != "url" || source.URL != "https://example.com/doc.txt" || source.FileName != "doc.txt" {
		t.Fatalf("unexpected source: %#v", source)
	}
}

func TestOpenClawAttachmentSourceFromNestedAssetSource(t *testing.T) {
	block := map[string]any{
		"type": "image",
		"asset": map[string]any{
			"source": map[string]any{
				"url":         "https://example.com/image.png",
				"contentType": "image/png",
				"fileName":    "image.png",
			},
		},
	}
	source := openClawAttachmentSourceFromBlock(block)
	if source == nil {
		t.Fatal("expected source")
	}
	if source.Kind != "url" || source.URL != "https://example.com/image.png" || source.MimeType != "image/png" || source.FileName != "image.png" {
		t.Fatalf("unexpected source: %#v", source)
	}
}

func TestDownloadOpenClawAttachmentURLRejectsLocalFiles(t *testing.T) {
	if _, _, err := downloadOpenClawAttachmentURL(context.Background(), "file:///tmp/test.txt", "", 1024, 1); err == nil {
		t.Fatal("expected local file URL to be rejected")
	}
	if _, _, err := downloadOpenClawAttachmentURL(context.Background(), "/tmp/test.txt", "", 1024, 1); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}

func TestTopicForPortal(t *testing.T) {
	oc := &OpenClawClient{}
	topic := oc.topicForPortal(&PortalMetadata{
		OpenClawChatType:           "channel",
		OpenClawChannel:            "discord",
		OpenClawSubject:            "Support",
		OpenClawSpace:              "Acme",
		OpenClawGroupChannel:       "support",
		ModelProvider:              "openai",
		Model:                      "gpt-5",
		OpenClawLastMessagePreview: "hello there",
		HistoryMode:                "recent_only",
	})
	want := "channel | discord | Acme#support | openai | gpt-5 | Recent: hello there | History: recent_only"
	if topic != want {
		t.Fatalf("unexpected topic: %q", topic)
	}
}

func TestTopicForPortalWithPreviewAndCatalogCounts(t *testing.T) {
	oc := &OpenClawClient{}
	topic := oc.topicForPortal(&PortalMetadata{
		OpenClawChatType:        "group",
		OpenClawChannel:         "discord",
		OpenClawOrigin:          "{\"provider\":\"discord\",\"channel\":\"123\"}",
		OpenClawPreviewSnippet:  "preview text",
		HistoryMode:             "recent_only",
		OpenClawToolProfile:     "default",
		OpenClawToolCount:       3,
		OpenClawKnownModelCount: 7,
	})
	want := "group | discord | Origin: {\"provider\":\"discord\",\"channel\":\"123\"} | Recent: preview text | History: recent_only | Tools: 3 (default) | Models: 7"
	if topic != want {
		t.Fatalf("unexpected topic: %q", topic)
	}
}

func TestOpenClawRoomType(t *testing.T) {
	tests := []struct {
		name string
		meta PortalMetadata
		want database.RoomType
	}{
		{
			name: "direct chat type stays dm",
			meta: PortalMetadata{OpenClawChatType: "direct"},
			want: database.RoomTypeDM,
		},
		{
			name: "group chat type becomes default room",
			meta: PortalMetadata{OpenClawChatType: "group"},
			want: database.RoomTypeDefault,
		},
		{
			name: "channel chat type becomes default room",
			meta: PortalMetadata{OpenClawChatType: "channel"},
			want: database.RoomTypeDefault,
		},
		{
			name: "group channel metadata becomes default room",
			meta: PortalMetadata{OpenClawGroupChannel: "alerts"},
			want: database.RoomTypeDefault,
		},
		{
			name: "synthetic dm stays dm",
			meta: PortalMetadata{OpenClawSessionKey: openClawDMAgentSessionKey("main")},
			want: database.RoomTypeDM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openClawRoomType(&tt.meta); got != tt.want {
				t.Fatalf("unexpected room type: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestDisplayNameForSessionUsesSourceLabel(t *testing.T) {
	oc := &OpenClawClient{}
	got := oc.displayNameForSession(gatewaySessionRow{
		Space:        "Acme",
		GroupChannel: "support",
		Channel:      "discord",
	})
	if got != "Acme#support" {
		t.Fatalf("unexpected display name: %q", got)
	}
}

func TestOpenClawGetCapabilitiesUsesSelectedModelModalities(t *testing.T) {
	oc := &OpenClawClient{
		modelCatalog: []gatewayModelChoice{
			{
				ID:        "gpt-5",
				Provider:  "openai",
				Reasoning: true,
				Input:     []string{"text", "image"},
			},
		},
	}
	portal := &bridgev2.Portal{
		Portal: &database.Portal{
			Metadata: &PortalMetadata{
				IsOpenClawRoom: true,
				ModelProvider:  "openai",
				Model:          "gpt-5",
			},
		},
	}

	caps := oc.GetCapabilities(context.Background(), portal)
	if caps.ID != openClawCapabilityBaseID+"+reasoning+vision" {
		t.Fatalf("unexpected capability id: %q", caps.ID)
	}
	if caps.File[event.MsgImage].MimeTypes["*/*"] != event.CapLevelFullySupported {
		t.Fatalf("expected images to be supported, got %#v", caps.File[event.MsgImage])
	}
	if caps.File[event.CapMsgGIF].MimeTypes["*/*"] != event.CapLevelFullySupported {
		t.Fatalf("expected GIFs to be supported, got %#v", caps.File[event.CapMsgGIF])
	}
	if caps.File[event.MsgAudio].MimeTypes["*/*"] != event.CapLevelRejected {
		t.Fatalf("expected audio to stay rejected, got %#v", caps.File[event.MsgAudio])
	}
}

func TestOpenClawGetCapabilitiesRejectsUnsupportedMediaWhenKnown(t *testing.T) {
	oc := &OpenClawClient{
		modelCatalog: []gatewayModelChoice{
			{
				ID:       "gpt-5-mini",
				Provider: "openai",
				Input:    []string{"text"},
			},
		},
	}
	portal := &bridgev2.Portal{
		Portal: &database.Portal{
			Metadata: &PortalMetadata{
				IsOpenClawRoom: true,
				ModelProvider:  "openai",
				Model:          "gpt-5-mini",
			},
		},
	}

	caps := oc.GetCapabilities(context.Background(), portal)
	if caps.ID != openClawCapabilityBaseID {
		t.Fatalf("unexpected capability id: %q", caps.ID)
	}
	if caps.File[event.MsgFile].MimeTypes["*/*"] != event.CapLevelFullySupported {
		t.Fatalf("expected generic files to stay supported, got %#v", caps.File[event.MsgFile])
	}
	if caps.File[event.MsgImage].MimeTypes["*/*"] != event.CapLevelRejected {
		t.Fatalf("expected images to be rejected, got %#v", caps.File[event.MsgImage])
	}
	if caps.File[event.MsgVideo].MimeTypes["*/*"] != event.CapLevelRejected {
		t.Fatalf("expected video to be rejected, got %#v", caps.File[event.MsgVideo])
	}
}

func TestOpenClawGetCapabilitiesFallsBackWhenModelSupportUnknown(t *testing.T) {
	oc := &OpenClawClient{}
	portal := &bridgev2.Portal{
		Portal: &database.Portal{
			Metadata: &PortalMetadata{
				IsOpenClawRoom: true,
				ModelProvider:  "openai",
				Model:          "unknown-model",
			},
		},
	}

	caps := oc.GetCapabilities(context.Background(), portal)
	if caps.ID != openClawCapabilityBaseID+"+fallbackmedia" {
		t.Fatalf("unexpected capability id: %q", caps.ID)
	}
	for _, msgType := range []event.MessageType{
		event.MsgImage,
		event.MsgVideo,
		event.MsgAudio,
		event.MsgFile,
		event.CapMsgVoice,
		event.CapMsgGIF,
		event.CapMsgSticker,
	} {
		if caps.File[msgType].MimeTypes["*/*"] != event.CapLevelFullySupported {
			t.Fatalf("expected %s to use fallback support, got %#v", msgType, caps.File[msgType])
		}
	}
}

func TestOpenClawApprovalResolvedText(t *testing.T) {
	if got := openClawApprovalResolvedText("deny"); got != "Tool approval denied" {
		t.Fatalf("unexpected deny text: %q", got)
	}
}

func TestRecoverRunTextEmptyWithoutGateway(t *testing.T) {
	mgr := &openClawManager{}
	if text := mgr.recoverRunText(context.Background(), "", "turn-1"); text != "" {
		t.Fatalf("expected empty text, got %q", text)
	}
}

package openclaw

import (
	"testing"

	"github.com/beeper/agentremote/pkg/shared/streamui"
)

func TestComputeVisibleDeltaTracksPrefixOnly(t *testing.T) {
	oc := &OpenClawClient{
		streamStates: map[string]*openClawStreamState{
			"turn-1": {turnID: "turn-1"},
		},
	}

	if got := oc.computeVisibleDelta("turn-1", "hello"); got != "hello" {
		t.Fatalf("expected first delta to be full text, got %q", got)
	}
	if got := oc.computeVisibleDelta("turn-1", "hello world"); got != " world" {
		t.Fatalf("expected suffix delta, got %q", got)
	}
	if got := oc.computeVisibleDelta("turn-1", "hello world"); got != "" {
		t.Fatalf("expected no delta for unchanged text, got %q", got)
	}
}

func TestIsStreamActiveReflectsStatePresence(t *testing.T) {
	oc := &OpenClawClient{
		streamStates: map[string]*openClawStreamState{
			"turn-2": {turnID: "turn-2"},
		},
	}
	if !oc.isStreamActive("turn-2") {
		t.Fatal("expected active stream state")
	}
	if oc.isStreamActive("missing") {
		t.Fatal("did not expect missing stream state to be active")
	}
}

func TestBuildStreamDBMetadataIncludesToolCalls(t *testing.T) {
	oc := &OpenClawClient{}
	state := &openClawStreamState{
		turnID:     "turn-3",
		agentID:    "main",
		sessionID:  "sess-1",
		sessionKey: "agent:main:matrix-dm",
		role:       "assistant",
	}
	state.visible.WriteString("running")
	streamui.ApplyChunk(&state.ui, map[string]any{
		"type": "reasoning-start",
		"id":   "reasoning-1",
	})
	streamui.ApplyChunk(&state.ui, map[string]any{
		"type":  "reasoning-delta",
		"id":    "reasoning-1",
		"delta": "thinking",
	})
	streamui.ApplyChunk(&state.ui, map[string]any{
		"type": "reasoning-end",
		"id":   "reasoning-1",
	})
	streamui.ApplyChunk(&state.ui, map[string]any{
		"type":       "tool-input-available",
		"toolCallId": "call-1",
		"toolName":   "bash",
		"input":      map[string]any{"cmd": "pwd"},
	})
	streamui.ApplyChunk(&state.ui, map[string]any{
		"type":       "tool-output-available",
		"toolCallId": "call-1",
		"output":     map[string]any{"stdout": "/tmp"},
	})
	streamui.ApplyChunk(&state.ui, map[string]any{
		"type":      "file",
		"url":       "mxc://example.org/out",
		"mediaType": "image/png",
	})

	meta := oc.buildStreamDBMetadata(state)
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if meta.ThinkingContent != "thinking" {
		t.Fatalf("unexpected thinking content: %q", meta.ThinkingContent)
	}
	if len(meta.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", meta.ToolCalls)
	}
	call := meta.ToolCalls[0]
	if call.CallID != "call-1" || call.ToolName != "bash" || call.ToolType != "openclaw" {
		t.Fatalf("unexpected tool call metadata: %#v", call)
	}
	if call.Status != "output-available" || call.ResultStatus != "completed" {
		t.Fatalf("unexpected tool call status: %#v", call)
	}
	if call.Input["cmd"] != "pwd" {
		t.Fatalf("unexpected tool input: %#v", call.Input)
	}
	if call.Output["stdout"] != "/tmp" {
		t.Fatalf("unexpected tool output: %#v", call.Output)
	}
	if len(meta.GeneratedFiles) != 1 {
		t.Fatalf("expected 1 generated file, got %#v", meta.GeneratedFiles)
	}
	if meta.GeneratedFiles[0].URL != "mxc://example.org/out" || meta.GeneratedFiles[0].MimeType != "image/png" {
		t.Fatalf("unexpected generated files: %#v", meta.GeneratedFiles)
	}
}

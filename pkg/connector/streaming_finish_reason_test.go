package connector

import "testing"

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "tool_calls", input: "tool_calls", expect: "tool-calls"},
		{name: "tool_use", input: "tool_use", expect: "tool-calls"},
		{name: "tool-use", input: "tool-use", expect: "tool-calls"},
		{name: "toolUse", input: "toolUse", expect: "tool-calls"},
		{name: "end_turn", input: "end_turn", expect: "stop"},
		{name: "end-turn", input: "end-turn", expect: "stop"},
		{name: "stop", input: "stop", expect: "stop"},
		{name: "other", input: "weird", expect: "other"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapFinishReason(tc.input)
			if got != tc.expect {
				t.Fatalf("mapFinishReason(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}

func TestShouldContinueChatToolLoop(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		toolCalls  int
		shouldLoop bool
	}{
		{name: "no_tool_calls", reason: "tool_calls", toolCalls: 0, shouldLoop: false},
		{name: "tool_calls", reason: "tool_calls", toolCalls: 1, shouldLoop: true},
		{name: "tool_use", reason: "tool_use", toolCalls: 1, shouldLoop: true},
		{name: "tool-use", reason: "tool-use", toolCalls: 1, shouldLoop: true},
		{name: "toolUse", reason: "toolUse", toolCalls: 1, shouldLoop: true},
		{name: "empty_reason", reason: "", toolCalls: 1, shouldLoop: true},
		{name: "stop_reason", reason: "stop", toolCalls: 1, shouldLoop: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldContinueChatToolLoop(tc.reason, tc.toolCalls)
			if got != tc.shouldLoop {
				t.Fatalf(
					"shouldContinueChatToolLoop(%q, %d) = %v, want %v",
					tc.reason,
					tc.toolCalls,
					got,
					tc.shouldLoop,
				)
			}
		})
	}
}

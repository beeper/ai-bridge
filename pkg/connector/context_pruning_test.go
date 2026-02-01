package connector

import (
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestSmartTruncatePrompt(t *testing.T) {
	// Helper to create user messages
	userMsg := func(content string) openai.ChatCompletionMessageParamUnion {
		return openai.UserMessage(content)
	}

	// Helper to create assistant messages
	assistantMsg := func(content string) openai.ChatCompletionMessageParamUnion {
		return openai.AssistantMessage(content)
	}

	// Helper to create system messages
	systemMsg := func(content string) openai.ChatCompletionMessageParamUnion {
		return openai.SystemMessage(content)
	}

	// Helper to create tool result messages
	toolResultMsg := func(content, toolCallID string) openai.ChatCompletionMessageParamUnion {
		return openai.ToolMessage(content, toolCallID)
	}

	t.Run("preserves system and latest user", func(t *testing.T) {
		prompt := []openai.ChatCompletionMessageParamUnion{
			systemMsg("You are a helpful assistant."),
			userMsg("Hello"),
			assistantMsg("Hi there!"),
			userMsg("How are you?"),
			assistantMsg("I'm doing well!"),
			userMsg("Tell me a joke"), // Latest user message
		}

		result := smartTruncatePrompt(prompt, 0.5)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}

		// Should always have system message first
		if result[0].OfSystem == nil {
			t.Error("First message should be system")
		}

		// Should always have latest user message last
		lastMsg := result[len(result)-1]
		if lastMsg.OfUser == nil || lastMsg.OfUser.Content.OfString.Value != "Tell me a joke" {
			t.Error("Last message should be the latest user message")
		}
	})

	t.Run("truncates large tool results", func(t *testing.T) {
		largeContent := strings.Repeat("x", 10000) // 10k chars

		prompt := []openai.ChatCompletionMessageParamUnion{
			systemMsg("System"),
			userMsg("Run code"),
			assistantMsg("Running..."), // Would have tool calls in real scenario
			toolResultMsg(largeContent, "call_123"),
			userMsg("What happened?"),
		}

		result := smartTruncatePrompt(prompt, 0.5)

		if result == nil {
			t.Fatal("Expected non-nil result")
		}

		// Find the tool result message
		var toolContent string
		for _, msg := range result {
			if msg.OfTool != nil {
				toolContent = extractToolContent(msg.OfTool.Content)
				break
			}
		}

		if toolContent == "" {
			t.Fatal("Tool result message should be preserved")
		}

		// Should be truncated to head + tail + omission notice
		if len(toolContent) >= len(largeContent) {
			t.Errorf("Tool content should be truncated, got %d chars", len(toolContent))
		}

		// Should contain the omission notice
		if !strings.Contains(toolContent, "characters omitted") {
			t.Error("Truncated content should contain omission notice")
		}
	})

	t.Run("returns nil when cannot truncate further", func(t *testing.T) {
		prompt := []openai.ChatCompletionMessageParamUnion{
			systemMsg("System"),
			userMsg("Hello"),
		}

		result := smartTruncatePrompt(prompt, 0.5)

		if result != nil {
			t.Error("Expected nil when prompt has only 2 messages")
		}
	})

	t.Run("removes oldest messages first", func(t *testing.T) {
		prompt := []openai.ChatCompletionMessageParamUnion{
			systemMsg("System"),
			userMsg("First message - oldest"),
			assistantMsg("Response 1"),
			userMsg("Second message"),
			assistantMsg("Response 2"),
			userMsg("Third message"),
			assistantMsg("Response 3"),
			userMsg("Latest message"),
		}

		result := smartTruncatePrompt(prompt, 0.7) // Aggressive pruning

		if result == nil {
			t.Fatal("Expected non-nil result")
		}

		// Should not contain "First message - oldest"
		for _, msg := range result {
			if msg.OfUser != nil && strings.Contains(msg.OfUser.Content.OfString.Value, "First message") {
				t.Error("Oldest message should be removed")
			}
		}

		// Should still have system and latest user
		if result[0].OfSystem == nil {
			t.Error("System message should be preserved")
		}

		lastMsg := result[len(result)-1]
		if lastMsg.OfUser == nil || lastMsg.OfUser.Content.OfString.Value != "Latest message" {
			t.Error("Latest user message should be preserved")
		}
	})
}

func TestTruncateToolResult(t *testing.T) {
	t.Run("does not truncate small content", func(t *testing.T) {
		content := "Small content"
		result := truncateToolResult(content)
		if result != content {
			t.Errorf("Small content should not be truncated")
		}
	})

	t.Run("truncates large content with head and tail", func(t *testing.T) {
		// Create content larger than head + tail threshold
		content := strings.Repeat("H", toolResultHeadChars) +
			strings.Repeat("M", 5000) + // Middle content to be removed
			strings.Repeat("T", toolResultTailChars)

		result := truncateToolResult(content)

		// Should be smaller than original
		if len(result) >= len(content) {
			t.Errorf("Large content should be truncated")
		}

		// Should start with head content
		if !strings.HasPrefix(result, strings.Repeat("H", 100)) {
			t.Error("Should preserve head content")
		}

		// Should end with tail content
		if !strings.HasSuffix(result, strings.Repeat("T", 100)) {
			t.Error("Should preserve tail content")
		}

		// Should have omission notice
		if !strings.Contains(result, "characters omitted") {
			t.Error("Should contain omission notice")
		}
	})
}

func TestAnalyzeMessage(t *testing.T) {
	t.Run("analyzes system message", func(t *testing.T) {
		msg := openai.SystemMessage("System prompt")
		info := analyzeMessage(msg, 0)

		if info.role != "system" {
			t.Errorf("Expected role 'system', got '%s'", info.role)
		}
		if info.prunable {
			t.Error("System messages should not be prunable")
		}
	})

	t.Run("analyzes user message", func(t *testing.T) {
		msg := openai.UserMessage("Hello world")
		info := analyzeMessage(msg, 1)

		if info.role != "user" {
			t.Errorf("Expected role 'user', got '%s'", info.role)
		}
		if !info.prunable {
			t.Error("User messages should be prunable")
		}
		if info.charCount != 11 {
			t.Errorf("Expected charCount 11, got %d", info.charCount)
		}
	})

	t.Run("analyzes tool result message", func(t *testing.T) {
		msg := openai.ToolMessage("Result content", "call_123")
		info := analyzeMessage(msg, 2)

		if info.role != "tool" {
			t.Errorf("Expected role 'tool', got '%s'", info.role)
		}
		if !info.isToolResult {
			t.Error("Should be marked as tool result")
		}
		if info.toolCallID != "call_123" {
			t.Errorf("Expected toolCallID 'call_123', got '%s'", info.toolCallID)
		}
	})

	t.Run("marks large tool results as prunable", func(t *testing.T) {
		largeContent := strings.Repeat("x", largeToolResultChars+100)
		msg := openai.ToolMessage(largeContent, "call_456")
		info := analyzeMessage(msg, 3)

		if !info.prunable {
			t.Error("Large tool results should be prunable")
		}
	})
}

package connector

import (
	"fmt"

	"github.com/openai/openai-go/v3"
)

// Context pruning settings - can be made configurable later
const (
	// Approximate characters per token for estimation
	charsPerToken = 4

	// When pruning tool results, keep this many chars from head
	toolResultHeadChars = 2000

	// When pruning tool results, keep this many chars from tail
	toolResultTailChars = 1000

	// Minimum messages to keep (besides system + latest user)
	minHistoryMessages = 2

	// Threshold for considering a tool result "large" and prunable
	largeToolResultChars = 4000
)

// messageInfo holds metadata about a message for pruning decisions
type messageInfo struct {
	index        int
	role         string
	charCount    int
	tokenCount   int
	isToolCall   bool // assistant message with tool calls
	isToolResult bool // tool result message
	toolCallID   string
	prunable     bool
	pruned       bool
}

// estimateTokens estimates token count from character count
func estimateTokens(chars int) int {
	return (chars + charsPerToken - 1) / charsPerToken
}

// analyzeMessage extracts metadata from a message for pruning decisions
func analyzeMessage(msg openai.ChatCompletionMessageParamUnion, index int) messageInfo {
	info := messageInfo{
		index: index,
	}

	switch {
	case msg.OfSystem != nil:
		info.role = "system"
		content := extractSystemContent(msg.OfSystem.Content)
		info.charCount = len(content)
		// System messages are never prunable
		info.prunable = false

	case msg.OfUser != nil:
		info.role = "user"
		content := extractUserContent(msg.OfUser.Content)
		info.charCount = len(content)
		// Add image token estimates
		for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
			if part.OfImageURL != nil {
				// Images consume significant tokens (~85 tokens for low detail, 765+ for high)
				info.charCount += 3000 // Conservative estimate
			}
		}
		info.prunable = true

	case msg.OfAssistant != nil:
		info.role = "assistant"
		content := extractAssistantContent(msg.OfAssistant.Content)
		info.charCount = len(content)
		// Check for tool calls
		if len(msg.OfAssistant.ToolCalls) > 0 {
			info.isToolCall = true
			for _, tc := range msg.OfAssistant.ToolCalls {
				if tc.OfFunction != nil {
					info.charCount += len(tc.OfFunction.Function.Name) + len(tc.OfFunction.Function.Arguments)
				}
			}
		}
		info.prunable = true

	case msg.OfTool != nil:
		info.role = "tool"
		info.isToolResult = true
		info.toolCallID = msg.OfTool.ToolCallID
		content := extractToolContent(msg.OfTool.Content)
		info.charCount = len(content)
		// Large tool results are prunable (can be truncated)
		info.prunable = info.charCount > largeToolResultChars
	}

	info.tokenCount = estimateTokens(info.charCount)
	return info
}

// truncateToolResult truncates a large tool result keeping head and tail
func truncateToolResult(content string) string {
	if len(content) <= toolResultHeadChars+toolResultTailChars+100 {
		return content
	}

	head := content[:toolResultHeadChars]
	tail := content[len(content)-toolResultTailChars:]
	omitted := len(content) - toolResultHeadChars - toolResultTailChars

	return head + fmt.Sprintf("\n\n[... %d characters omitted ...]\n\n", omitted) + tail
}

// smartTruncatePrompt intelligently prunes messages while preserving conversation coherence.
// Strategy:
// 1. Never remove system prompt or latest user message
// 2. First, truncate large tool results (keep head + tail)
// 3. Then remove oldest messages, keeping tool call/result pairs together
// 4. Preserve recent context with higher priority
func smartTruncatePrompt(
	prompt []openai.ChatCompletionMessageParamUnion,
	targetReduction float64, // 0.5 = reduce by 50%
) []openai.ChatCompletionMessageParamUnion {
	if len(prompt) <= 2 {
		return nil // Can't truncate further
	}

	// Analyze all messages
	messages := make([]messageInfo, len(prompt))
	totalTokens := 0
	for i, msg := range prompt {
		messages[i] = analyzeMessage(msg, i)
		totalTokens += messages[i].tokenCount
	}

	targetTokens := int(float64(totalTokens) * (1 - targetReduction))
	currentTokens := totalTokens

	// Phase 1: Truncate large tool results first
	for i := range messages {
		if currentTokens <= targetTokens {
			break
		}
		if messages[i].role == "tool" && messages[i].charCount > largeToolResultChars {
			// Truncate this tool result
			oldChars := messages[i].charCount
			newChars := toolResultHeadChars + toolResultTailChars + 100
			savedTokens := estimateTokens(oldChars - newChars)
			currentTokens -= savedTokens
			messages[i].pruned = true
			messages[i].charCount = newChars
			messages[i].tokenCount = estimateTokens(newChars)
		}
	}

	// Phase 2: Remove oldest messages if still over target
	// Find boundaries: system prompt (index 0 if present), latest user message
	systemIdx := -1
	latestUserIdx := -1
	if len(messages) > 0 && messages[0].role == "system" {
		systemIdx = 0
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].role == "user" {
			latestUserIdx = i
			break
		}
	}

	// Mark messages for removal from oldest to newest
	startIdx := 0
	if systemIdx == 0 {
		startIdx = 1
	}

	removedCount := 0
	for i := startIdx; i < len(messages) && currentTokens > targetTokens; i++ {
		// Don't remove the latest user message
		if i == latestUserIdx {
			continue
		}

		// Don't remove if it would leave fewer than minHistoryMessages
		remainingHistory := len(messages) - removedCount - 2 // -2 for system and latest user
		if remainingHistory <= minHistoryMessages {
			break
		}

		// If this is a tool call, also mark its results for removal
		if messages[i].isToolCall {
			currentTokens -= messages[i].tokenCount
			messages[i].pruned = true
			removedCount++

			// Find and remove associated tool results
			for j := i + 1; j < len(messages); j++ {
				if messages[j].isToolResult {
					currentTokens -= messages[j].tokenCount
					messages[j].pruned = true
					removedCount++
				} else if messages[j].role == "assistant" || messages[j].role == "user" {
					break // Stop at next non-tool message
				}
			}
			continue
		}

		// If this is a tool result without its call being removed, skip it
		// (we don't want orphaned tool results)
		if messages[i].isToolResult {
			continue
		}

		// Remove this message
		currentTokens -= messages[i].tokenCount
		messages[i].pruned = true
		removedCount++
	}

	// Build result, applying truncation to tool results and skipping removed messages
	var result []openai.ChatCompletionMessageParamUnion
	for i, info := range messages {
		if info.pruned && info.role != "tool" {
			continue // Skip removed messages (except tool results which get truncated)
		}

		msg := prompt[i]

		// Apply truncation to large tool results
		if info.role == "tool" && info.pruned {
			if msg.OfTool != nil && msg.OfTool.Content.OfString.Value != "" {
				truncated := truncateToolResult(msg.OfTool.Content.OfString.Value)
				msg = openai.ToolMessage(truncated, msg.OfTool.ToolCallID)
			}
		}

		result = append(result, msg)
	}

	// Sanity check: if we removed too much, return at least system + latest user
	if len(result) < 2 {
		return nil
	}

	return result
}

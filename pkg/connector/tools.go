package connector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

// ToolDefinition defines a tool that can be used by the AI
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(ctx context.Context, args map[string]any) (string, error)
}

// BridgeToolContext provides bridge-specific context for tool execution
type BridgeToolContext struct {
	Client        *AIClient
	Portal        *bridgev2.Portal
	Meta          *PortalMetadata
	SourceEventID id.EventID // The triggering message's event ID (for reactions/replies)
}

// bridgeToolContextKey is the context key for BridgeToolContext
type bridgeToolContextKey struct{}

// WithBridgeToolContext adds bridge context to a context
func WithBridgeToolContext(ctx context.Context, btc *BridgeToolContext) context.Context {
	return context.WithValue(ctx, bridgeToolContextKey{}, btc)
}

// GetBridgeToolContext retrieves bridge context from a context
func GetBridgeToolContext(ctx context.Context) *BridgeToolContext {
	if v := ctx.Value(bridgeToolContextKey{}); v != nil {
		return v.(*BridgeToolContext)
	}
	return nil
}

// BuiltinTools returns the list of available builtin tools
func BuiltinTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "calculator",
			Description: "Perform basic arithmetic calculations. Supports addition, subtraction, multiplication, division, and modulo operations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{
						"type":        "string",
						"description": "A mathematical expression to evaluate, e.g. '2 + 3 * 4' or '100 / 5'",
					},
				},
				"required": []string{"expression"},
			},
			Execute: executeCalculator,
		},
		{
			Name:        "web_search",
			Description: "Search the web for information. Returns a summary of search results.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query",
					},
				},
				"required": []string{"query"},
			},
			Execute: executeWebSearch,
		},
		{
			Name:        ToolNameSetChatInfo,
			Description: "Patch the current chat's title and/or description (omit fields to keep them unchanged).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{
						"type":        "string",
						"description": "Optional. The new title for the chat",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional. The new description/topic for the chat (empty string clears it)",
					},
				},
				"minProperties":        1,
				"additionalProperties": false,
			},
			Execute: executeSetChatInfo,
		},
		{
			Name:        ToolNameMessage,
			Description: "Send messages and perform channel actions in the current chat. Supports: send, react, reactions, edit, delete, reply, pin, unpin, list-pins, thread-reply, search, read, member-info, channel-info.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"send", "react", "reactions", "edit", "delete", "reply", "pin", "unpin", "list-pins", "thread-reply", "search", "read", "member-info", "channel-info"},
						"description": "The action to perform",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "For send/edit/reply/thread-reply: the message text",
					},
					"message_id": map[string]any{
						"type":        "string",
						"description": "Target message ID for react/reactions/edit/delete/reply/pin/unpin/thread-reply/read",
					},
					"emoji": map[string]any{
						"type":        "string",
						"description": "For action=react: the emoji to react with (empty to remove all reactions)",
					},
					"remove": map[string]any{
						"type":        "boolean",
						"description": "For action=react: set true to remove the reaction instead of adding",
					},
					"user_id": map[string]any{
						"type":        "string",
						"description": "For action=member-info: the Matrix user ID to look up (e.g., @user:server.com)",
					},
					"thread_id": map[string]any{
						"type":        "string",
						"description": "For action=thread-reply: the thread root message ID",
					},
					"query": map[string]any{
						"type":        "string",
						"description": "For action=search: search query to find messages",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "For action=search: max results to return (default: 20)",
					},
				},
				"required": []string{"action"},
			},
			Execute: executeMessage,
		},
		{
			Name:        ToolNameTTS,
			Description: "Convert text to speech audio. Returns audio that will be sent as a voice message. Only available on Beeper and OpenAI providers.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "The text to convert to speech (max 4096 characters)",
					},
					"voice": map[string]any{
						"type":        "string",
						"enum":        []string{"alloy", "ash", "coral", "echo", "fable", "onyx", "nova", "sage", "shimmer"},
						"description": "The voice to use for speech synthesis (default: alloy)",
					},
				},
				"required": []string{"text"},
			},
			Execute: executeTTS,
		},
		{
			Name:        ToolNameWebFetch,
			Description: "Fetch a web page and extract its readable content as text or markdown.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to fetch (must be http or https)",
					},
					"max_chars": map[string]any{
						"type":        "number",
						"description": "Maximum characters to return (default: 50000)",
					},
				},
				"required": []string{"url"},
			},
			Execute: executeWebFetch,
		},
		{
			Name:        ToolNameImage,
			Description: "Generate an image from a text prompt using AI image generation.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "The text prompt describing the image to generate",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "Image model to use (default: google/gemini-3-pro-image-preview)",
					},
				},
				"required": []string{"prompt"},
			},
			Execute: executeImageGeneration,
		},
		{
			Name:        ToolNameAnalyzeImage,
			Description: "Analyze an image with a custom prompt. Use this to examine image details, read text from images (OCR), identify objects, or get specific information about visual content.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"image_url": map[string]any{
						"type":        "string",
						"description": "URL of the image to analyze (http/https URL, mxc:// Matrix URL, or data: URI with base64)",
					},
					"prompt": map[string]any{
						"type":        "string",
						"description": "What to analyze or look for in the image (e.g., 'describe this image', 'read the text', 'what objects are visible')",
					},
				},
				"required": []string{"image_url", "prompt"},
			},
			Execute: executeAnalyzeImage,
		},
		{
			Name:        ToolNameSessionStatus,
			Description: "Get current session status including time, date, model info, and context usage. Use this tool when asked about current time, date, day of week, or what model is being used.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"set_model": map[string]any{
						"type":        "string",
						"description": "Optional: change the model for this session (e.g., 'gpt-4o', 'claude-sonnet-4-20250514')",
					},
				},
			},
			Execute: executeSessionStatus,
		},
		// Memory tools (matching OpenClaw interface)
		{
			Name:        ToolNameMemorySearch,
			Description: "Search your memory for relevant information. Use this to recall facts, preferences, decisions, or context from previous conversations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query to find relevant memories",
					},
					"maxResults": map[string]any{
						"type":        "number",
						"description": "Maximum number of results to return (default: 6)",
					},
					"minScore": map[string]any{
						"type":        "number",
						"description": "Minimum relevance score threshold (0-1, default: 0.35)",
					},
				},
				"required": []string{"query"},
			},
			Execute: executeMemorySearch,
		},
		{
			Name:        ToolNameMemoryGet,
			Description: "Retrieve the full content of a specific memory by its path.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The memory path (e.g., 'agent:myagent/fact:abc123' or 'global/fact:xyz789')",
					},
				},
				"required": []string{"path"},
			},
			Execute: executeMemoryGet,
		},
		{
			Name:        ToolNameMemoryStore,
			Description: "Store a new memory for later recall. Use this to remember important facts, user preferences, decisions, or context that should persist across conversations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "The content to store in memory",
					},
					"importance": map[string]any{
						"type":        "number",
						"description": "Importance score from 0 to 1 (default: 0.5). Higher values make the memory more likely to surface in searches.",
					},
					"category": map[string]any{
						"type":        "string",
						"enum":        []string{"preference", "decision", "entity", "fact", "other"},
						"description": "Category of memory (default: 'other')",
					},
					"scope": map[string]any{
						"type":        "string",
						"enum":        []string{"agent", "global"},
						"description": "Where to store the memory: 'agent' for this agent only, 'global' for all agents (default: 'agent')",
					},
				},
				"required": []string{"content"},
			},
			Execute: executeMemoryStore,
		},
		{
			Name:        ToolNameMemoryForget,
			Description: "Remove a memory by its ID/path. Use this to delete outdated or incorrect information.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "string",
						"description": "The memory ID or path to forget",
					},
				},
				"required": []string{"id"},
			},
			Execute: executeMemoryForget,
		},
	}
}

// ToolNameMessage is the name of the message tool.
const ToolNameMessage = "message"

// ToolNameTTS is the name of the text-to-speech tool.
const ToolNameTTS = "tts"

// ToolNameWebFetch is the name of the web fetch tool.
const ToolNameWebFetch = "web_fetch"

// ToolNameImage is the name of the image generation tool.
const ToolNameImage = "image"

// ToolNameAnalyzeImage is the name of the image analysis tool.
const ToolNameAnalyzeImage = "analyze_image"

// ToolNameSessionStatus is the name of the session status tool.
const ToolNameSessionStatus = "session_status"

// Memory tool names (matching OpenClaw interface)
const (
	ToolNameMemorySearch = "memory_search"
	ToolNameMemoryGet    = "memory_get"
	ToolNameMemoryStore  = "memory_store"
	ToolNameMemoryForget = "memory_forget"
)

// ImageResultPrefix is the prefix used to identify image results that need media sending.
const ImageResultPrefix = "IMAGE:"

// DefaultImageModel is the default model for image generation.
const DefaultImageModel = "google/gemini-3-pro-image-preview"

// TTSResultPrefix is the prefix used to identify TTS results that need audio sending.
const TTSResultPrefix = "AUDIO:"

// executeMessage handles the message tool for sending messages and channel actions.
// Matches OpenClaw's message tool pattern with full action support.
func executeMessage(ctx context.Context, args map[string]any) (string, error) {
	action, ok := args["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("missing or invalid 'action' argument")
	}

	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("message tool requires bridge context")
	}

	switch action {
	case "send":
		return executeMessageSend(ctx, args, btc)
	case "react":
		return executeMessageReact(ctx, args, btc)
	case "reactions":
		return executeMessageReactions(ctx, args, btc)
	case "edit":
		return executeMessageEdit(ctx, args, btc)
	case "delete":
		return executeMessageDelete(ctx, args, btc)
	case "reply":
		return executeMessageReply(ctx, args, btc)
	case "pin":
		return executeMessagePin(ctx, args, btc, true)
	case "unpin":
		return executeMessagePin(ctx, args, btc, false)
	case "list-pins":
		return executeMessageListPins(ctx, btc)
	case "thread-reply":
		return executeMessageThreadReply(ctx, args, btc)
	case "search":
		return executeMessageSearch(ctx, args, btc)
	case "read":
		return executeMessageRead(ctx, args, btc)
	case "member-info":
		return executeMessageMemberInfo(ctx, args, btc)
	case "channel-info":
		return executeMessageChannelInfo(ctx, args, btc)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// executeMessageReact handles the react action of the message tool.
// Supports adding reactions (with emoji) and removing reactions (with remove:true or empty emoji).
func executeMessageReact(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	emoji, _ := args["emoji"].(string)
	remove, _ := args["remove"].(bool)

	// Check if this is a removal request (remove:true or empty emoji)
	if remove || emoji == "" {
		return executeMessageReactRemove(ctx, args, btc)
	}

	// Get target message ID (optional - defaults to triggering message)
	var targetEventID id.EventID
	if msgID, ok := args["message_id"].(string); ok && msgID != "" {
		targetEventID = id.EventID(msgID)
	} else if btc.SourceEventID != "" {
		// Default to the triggering message (like clawdbot's currentMessageId)
		targetEventID = btc.SourceEventID
	}

	// If no target available, return error
	if targetEventID == "" {
		return "", fmt.Errorf("action=react requires 'message_id' parameter (no triggering message available)")
	}

	// Send reaction
	btc.Client.sendReaction(ctx, btc.Portal, targetEventID, emoji)

	return fmt.Sprintf(`{"action":"react","emoji":%q,"message_id":%q,"status":"sent"}`, emoji, targetEventID), nil
}

// executeMessageSend handles the send action of the message tool.
func executeMessageSend(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("action=send requires 'message' parameter")
	}

	// Get the model intent for sending
	intent := btc.Client.getModelIntent(ctx, btc.Portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	// Send the message
	rendered := format.RenderMarkdown(message, true, true)
	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype":        event.MsgText,
			"body":           rendered.Body,
			"format":         rendered.Format,
			"formatted_body": rendered.FormattedBody,
		},
	}

	resp, err := intent.SendMessage(ctx, btc.Portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return fmt.Sprintf(`{"action":"send","event_id":%q,"status":"sent"}`, resp.EventID), nil
}

// executeMessageEdit handles the edit action - edits an existing message.
func executeMessageEdit(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	messageID, ok := args["message_id"].(string)
	if !ok || messageID == "" {
		return "", fmt.Errorf("action=edit requires 'message_id' parameter")
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("action=edit requires 'message' parameter")
	}

	intent := btc.Client.getModelIntent(ctx, btc.Portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	targetEventID := id.EventID(messageID)
	rendered := format.RenderMarkdown(message, true, true)

	// Send edit with m.replace relation
	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype":        event.MsgText,
			"body":           "* " + rendered.Body,
			"format":         rendered.Format,
			"formatted_body": "* " + rendered.FormattedBody,
			"m.new_content": map[string]any{
				"msgtype":        event.MsgText,
				"body":           rendered.Body,
				"format":         rendered.Format,
				"formatted_body": rendered.FormattedBody,
			},
			"m.relates_to": map[string]any{
				"rel_type": RelReplace,
				"event_id": targetEventID.String(),
			},
		},
	}

	resp, err := intent.SendMessage(ctx, btc.Portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		return "", fmt.Errorf("failed to edit message: %w", err)
	}

	return fmt.Sprintf(`{"action":"edit","event_id":%q,"edited_id":%q,"status":"sent"}`, resp.EventID, targetEventID), nil
}

// executeMessageDelete handles the delete action - redacts a message.
func executeMessageDelete(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	messageID, ok := args["message_id"].(string)
	if !ok || messageID == "" {
		return "", fmt.Errorf("action=delete requires 'message_id' parameter")
	}

	intent := btc.Client.getModelIntent(ctx, btc.Portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	targetEventID := id.EventID(messageID)

	// Send redaction event
	_, err := intent.SendMessage(ctx, btc.Portal.MXID, event.EventRedaction, &event.Content{
		Parsed: &event.RedactionEventContent{
			Redacts: targetEventID,
		},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to delete message: %w", err)
	}

	return fmt.Sprintf(`{"action":"delete","deleted_id":%q,"status":"deleted"}`, targetEventID), nil
}

// executeMessageReply handles the reply action - sends a message as a reply to another.
func executeMessageReply(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	messageID, ok := args["message_id"].(string)
	if !ok || messageID == "" {
		return "", fmt.Errorf("action=reply requires 'message_id' parameter")
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("action=reply requires 'message' parameter")
	}

	intent := btc.Client.getModelIntent(ctx, btc.Portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	targetEventID := id.EventID(messageID)
	rendered := format.RenderMarkdown(message, true, true)

	// Send message with m.in_reply_to relation
	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype":        event.MsgText,
			"body":           rendered.Body,
			"format":         rendered.Format,
			"formatted_body": rendered.FormattedBody,
			"m.relates_to": map[string]any{
				"m.in_reply_to": map[string]any{
					"event_id": targetEventID.String(),
				},
			},
		},
	}

	resp, err := intent.SendMessage(ctx, btc.Portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		return "", fmt.Errorf("failed to send reply: %w", err)
	}

	return fmt.Sprintf(`{"action":"reply","event_id":%q,"reply_to":%q,"status":"sent"}`, resp.EventID, targetEventID), nil
}

// executeMessagePin handles pin/unpin actions - updates room pinned events.
func executeMessagePin(ctx context.Context, args map[string]any, btc *BridgeToolContext, pin bool) (string, error) {
	messageID, ok := args["message_id"].(string)
	if !ok || messageID == "" {
		action := "pin"
		if !pin {
			action = "unpin"
		}
		return "", fmt.Errorf("action=%s requires 'message_id' parameter", action)
	}

	targetEventID := id.EventID(messageID)
	bot := btc.Client.UserLogin.Bridge.Bot

	// Get current pinned events using the matrix connection
	var pinnedEvents []string
	matrixConn, ok := btc.Client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if ok {
		stateEvent, err := matrixConn.GetStateEvent(ctx, btc.Portal.MXID, event.StatePinnedEvents, "")
		if err == nil && stateEvent != nil {
			if content, ok := stateEvent.Content.Parsed.(*event.PinnedEventsEventContent); ok {
				for _, evtID := range content.Pinned {
					pinnedEvents = append(pinnedEvents, evtID.String())
				}
			}
		}
	}

	// Modify pinned events
	if pin {
		// Add to pinned if not already there
		found := false
		for _, evtID := range pinnedEvents {
			if evtID == targetEventID.String() {
				found = true
				break
			}
		}
		if !found {
			pinnedEvents = append(pinnedEvents, targetEventID.String())
		}
	} else {
		// Remove from pinned
		var newPinned []string
		for _, evtID := range pinnedEvents {
			if evtID != targetEventID.String() {
				newPinned = append(newPinned, evtID)
			}
		}
		pinnedEvents = newPinned
	}

	// Convert to id.EventID slice
	pinnedIDs := make([]id.EventID, len(pinnedEvents))
	for i, evtID := range pinnedEvents {
		pinnedIDs[i] = id.EventID(evtID)
	}

	// Update pinned events state
	_, err := bot.SendState(ctx, btc.Portal.MXID, event.StatePinnedEvents, "", &event.Content{
		Parsed: &event.PinnedEventsEventContent{
			Pinned: pinnedIDs,
		},
	}, time.Time{})
	if err != nil {
		action := "pin"
		if !pin {
			action = "unpin"
		}
		return "", fmt.Errorf("failed to %s message: %w", action, err)
	}

	action := "pin"
	if !pin {
		action = "unpin"
	}
	return fmt.Sprintf(`{"action":%q,"message_id":%q,"status":"ok","pinned_count":%d}`, action, targetEventID, len(pinnedEvents)), nil
}

// executeMessageListPins handles list-pins action - returns currently pinned messages.
func executeMessageListPins(ctx context.Context, btc *BridgeToolContext) (string, error) {
	// Get current pinned events using the matrix connection
	var pinnedEvents []string
	matrixConn, ok := btc.Client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if ok {
		stateEvent, err := matrixConn.GetStateEvent(ctx, btc.Portal.MXID, event.StatePinnedEvents, "")
		if err == nil && stateEvent != nil {
			if content, ok := stateEvent.Content.Parsed.(*event.PinnedEventsEventContent); ok {
				for _, evtID := range content.Pinned {
					pinnedEvents = append(pinnedEvents, evtID.String())
				}
			}
		}
	}

	// Build JSON response
	pinnedJSON, _ := json.Marshal(pinnedEvents)
	return fmt.Sprintf(`{"action":"list-pins","pinned":%s,"count":%d}`, string(pinnedJSON), len(pinnedEvents)), nil
}

// executeMessageThreadReply handles thread-reply action - sends a message in a thread.
func executeMessageThreadReply(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	// thread_id is the root message of the thread
	threadID, ok := args["thread_id"].(string)
	if !ok || threadID == "" {
		// Fall back to message_id for thread root
		threadID, ok = args["message_id"].(string)
		if !ok || threadID == "" {
			return "", fmt.Errorf("action=thread-reply requires 'thread_id' or 'message_id' parameter")
		}
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("action=thread-reply requires 'message' parameter")
	}

	intent := btc.Client.getModelIntent(ctx, btc.Portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	threadRootID := id.EventID(threadID)
	rendered := format.RenderMarkdown(message, true, true)

	// Send message with m.thread relation
	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype":        event.MsgText,
			"body":           rendered.Body,
			"format":         rendered.Format,
			"formatted_body": rendered.FormattedBody,
			"m.relates_to": map[string]any{
				"rel_type": "m.thread",
				"event_id": threadRootID.String(),
			},
		},
	}

	resp, err := intent.SendMessage(ctx, btc.Portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		return "", fmt.Errorf("failed to send thread reply: %w", err)
	}

	return fmt.Sprintf(`{"action":"thread-reply","event_id":%q,"thread_id":%q,"status":"sent"}`, resp.EventID, threadRootID), nil
}

// executeMessageSearch searches messages in the current chat.
func executeMessageSearch(ctx context.Context, args map[string]any, btc *BridgeToolContext) (string, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("action=search requires 'query' parameter")
	}

	// Get limit (default 20)
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100 // Cap at 100 results
		}
	}

	// Get messages from database
	// Fetch more than needed since we'll filter
	messages, err := btc.Client.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, btc.Portal.PortalKey, 1000)
	if err != nil {
		return "", fmt.Errorf("failed to get messages: %w", err)
	}

	// Search through messages
	queryLower := strings.ToLower(query)
	var results []map[string]any

	for _, msg := range messages {
		if len(results) >= limit {
			break
		}

		// Get message body from metadata
		msgMeta, ok := msg.Metadata.(*MessageMetadata)
		if ok && msgMeta != nil {
			body := msgMeta.Body
			if body != "" && strings.Contains(strings.ToLower(body), queryLower) {
				results = append(results, map[string]any{
					"message_id": msg.MXID.String(),
					"role":       msgMeta.Role,
					"content":    truncateString(body, 200),
					"timestamp":  msg.Timestamp.Unix(),
				})
			}
		}
	}

	// Build JSON response
	resultsJSON, _ := json.Marshal(results)
	return fmt.Sprintf(`{"action":"search","query":%q,"results":%s,"count":%d}`, query, string(resultsJSON), len(results)), nil
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// executeWebFetch fetches a web page and extracts readable content.
func executeWebFetch(ctx context.Context, args map[string]any) (string, error) {
	urlStr, ok := args["url"].(string)
	if !ok || urlStr == "" {
		return "", fmt.Errorf("missing or invalid 'url' argument")
	}

	// Parse and validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("URL must use http or https scheme")
	}

	// Get max chars (default 50000)
	maxChars := 50000
	if mc, ok := args["max_chars"].(float64); ok && mc > 0 {
		maxChars = int(mc)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; BeeperAI/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Read body with limit
	limitedReader := io.LimitReader(resp.Body, int64(maxChars*2)) // Read extra for HTML overhead
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Extract text content (simple approach - strip HTML tags)
	content := extractTextFromHTML(string(bodyBytes))

	// Truncate to max chars
	if len(content) > maxChars {
		content = content[:maxChars] + "...[truncated]"
	}

	return fmt.Sprintf(`{"url":%q,"content":%q,"length":%d}`, urlStr, content, len(content)), nil
}

// extractTextFromHTML does a simple extraction of text from HTML.
// This is a basic implementation - a full readability parser would be better.
func extractTextFromHTML(html string) string {
	// Remove script and style elements
	html = removeHTMLElement(html, "script")
	html = removeHTMLElement(html, "style")
	html = removeHTMLElement(html, "noscript")

	// Remove all HTML tags
	var result strings.Builder
	inTag := false
	lastWasSpace := false

	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			// Add space after tags to separate words
			if !lastWasSpace {
				result.WriteRune(' ')
				lastWasSpace = true
			}
			continue
		}
		if !inTag {
			// Normalize whitespace
			if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
				if !lastWasSpace {
					result.WriteRune(' ')
					lastWasSpace = true
				}
			} else {
				result.WriteRune(r)
				lastWasSpace = false
			}
		}
	}

	// Decode common HTML entities
	text := result.String()
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	// Collapse multiple spaces
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return strings.TrimSpace(text)
}

// removeHTMLElement removes all instances of an HTML element and its content.
func removeHTMLElement(html, tag string) string {
	result := html
	lowerTag := strings.ToLower(tag)

	for {
		startIdx := strings.Index(strings.ToLower(result), "<"+lowerTag)
		if startIdx == -1 {
			break
		}

		// Find the end of this element
		endTag := "</" + lowerTag + ">"
		endIdx := strings.Index(strings.ToLower(result[startIdx:]), endTag)
		if endIdx == -1 {
			// Self-closing or malformed - just remove to next >
			closeIdx := strings.Index(result[startIdx:], ">")
			if closeIdx == -1 {
				break
			}
			result = result[:startIdx] + result[startIdx+closeIdx+1:]
		} else {
			result = result[:startIdx] + result[startIdx+endIdx+len(endTag):]
		}
	}

	return result
}

// executeImageGeneration generates an image using OpenRouter's image generation API.
func executeImageGeneration(ctx context.Context, args map[string]any) (string, error) {
	prompt, ok := args["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("missing or invalid 'prompt' argument")
	}

	// Get model (default to stable diffusion)
	model := DefaultImageModel
	if m, ok := args["model"].(string); ok && m != "" {
		model = m
	}

	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("image generation requires bridge context")
	}

	// Get the provider to check if we can use OpenRouter
	provider, ok := btc.Client.provider.(*OpenAIProvider)
	if !ok {
		return "", fmt.Errorf("image generation requires OpenAI-compatible provider")
	}

	// Check if using OpenRouter
	baseURL := strings.ToLower(provider.baseURL)
	if !strings.Contains(baseURL, "openrouter") {
		return "", fmt.Errorf("image generation requires OpenRouter provider")
	}

	// Call OpenRouter image generation
	imageData, err := callOpenRouterImageGen(ctx, btc.Client.apiKey, provider.baseURL, prompt, model)
	if err != nil {
		return "", fmt.Errorf("image generation failed: %w", err)
	}

	// Return image as base64 with IMAGE: prefix for connector to handle
	return ImageResultPrefix + imageData, nil
}

// callOpenRouterImageGen calls OpenRouter's image generation endpoint.
func callOpenRouterImageGen(ctx context.Context, apiKey, baseURL, prompt, model string) (string, error) {
	// OpenRouter uses chat completions with image models
	// The response will contain a URL or base64 image

	// Normalize base URL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Build request for image generation via chat completions
	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens": 1,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://beeper.com")
	req.Header.Set("X-Title", "Beeper AI")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response to extract image URL or data
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for direct image data (DALL-E style response)
	if len(result.Data) > 0 {
		if result.Data[0].B64JSON != "" {
			return result.Data[0].B64JSON, nil
		}
		if result.Data[0].URL != "" {
			// Fetch the image from URL and convert to base64
			return fetchImageAsBase64(ctx, result.Data[0].URL)
		}
	}

	// Check for chat completion response with image URL
	if len(result.Choices) > 0 && result.Choices[0].Message.Content != "" {
		content := result.Choices[0].Message.Content
		// If content looks like a URL, fetch it
		if strings.HasPrefix(content, "http") {
			return fetchImageAsBase64(ctx, content)
		}
		// If it's already base64, return it
		if _, err := base64.StdEncoding.DecodeString(content); err == nil {
			return content, nil
		}
		return "", fmt.Errorf("unexpected response format: %s", content[:min(100, len(content))])
	}

	return "", fmt.Errorf("no image data in response")
}

// fetchImageAsBase64 fetches an image URL and returns it as base64.
func fetchImageAsBase64(ctx context.Context, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch image: %d", resp.StatusCode)
	}

	// Limit to 10MB
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// executeTTS converts text to speech.
// Supports: macOS 'say' command, Beeper provider, OpenAI provider.
func executeTTS(ctx context.Context, args map[string]any) (string, error) {
	text, ok := args["text"].(string)
	if !ok || text == "" {
		return "", fmt.Errorf("missing or invalid 'text' argument")
	}

	// Limit text length
	const maxTextLen = 4096
	if len(text) > maxTextLen {
		return "", fmt.Errorf("text too long: %d characters (max %d)", len(text), maxTextLen)
	}

	// Get voice (default to "alloy" for OpenAI, "Samantha" for macOS)
	voice := ""
	if v, ok := args["voice"].(string); ok && v != "" {
		voice = v
	}

	btc := GetBridgeToolContext(ctx)

	// Try provider-based TTS first (Beeper/OpenAI)
	if btc != nil {
		if provider, ok := btc.Client.provider.(*OpenAIProvider); ok {
			baseURL := strings.ToLower(provider.baseURL)
			isBeeperProvider := strings.Contains(baseURL, "beeper")
			isOpenAIProvider := baseURL == "" || strings.Contains(baseURL, "openai.com")

			if isBeeperProvider || isOpenAIProvider {
				// Use OpenAI voice if not specified
				if voice == "" {
					voice = "alloy"
				}

				// Validate OpenAI voice
				validVoices := map[string]bool{
					"alloy": true, "ash": true, "coral": true, "echo": true,
					"fable": true, "onyx": true, "nova": true, "sage": true, "shimmer": true,
				}
				if !validVoices[voice] {
					voice = "alloy" // Fall back to default
				}

				// Call OpenAI TTS API
				audioData, err := callOpenAITTS(ctx, btc.Client.apiKey, provider.baseURL, text, voice)
				if err == nil {
					return TTSResultPrefix + audioData, nil
				}
				// Fall through to macOS say if API fails
			}
		}
	}

	// Try macOS 'say' command as fallback
	if isTTSMacOSAvailable() {
		if voice == "" {
			voice = "Samantha" // Default macOS voice
		}
		audioData, err := callMacOSSay(ctx, text, voice)
		if err != nil {
			return "", fmt.Errorf("macOS TTS failed: %w", err)
		}
		return TTSResultPrefix + audioData, nil
	}

	return "", fmt.Errorf("TTS not available: requires Beeper/OpenAI provider or macOS")
}

// isTTSMacOSAvailable checks if macOS 'say' command is available.
func isTTSMacOSAvailable() bool {
	return runtime.GOOS == "darwin"
}

// callMacOSSay uses macOS 'say' command to generate speech.
func callMacOSSay(ctx context.Context, text, voice string) (string, error) {
	// Create temp file for output
	tmpFile, err := os.CreateTemp("", "tts-*.aiff")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Run say command
	args := []string{"-o", tmpPath}
	if voice != "" {
		args = append(args, "-v", voice)
	}
	args = append(args, text)

	cmd := exec.CommandContext(ctx, "say", args...)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("say command failed: %w", err)
	}

	// Read the generated audio file
	audioData, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read audio file: %w", err)
	}

	// Return as base64
	return base64.StdEncoding.EncodeToString(audioData), nil
}

// callOpenAITTS calls OpenAI's /v1/audio/speech endpoint
func callOpenAITTS(ctx context.Context, apiKey, baseURL, text, voice string) (string, error) {
	// Determine endpoint URL
	endpoint := "https://api.openai.com/v1/audio/speech"
	if baseURL != "" {
		endpoint = strings.TrimSuffix(baseURL, "/") + "/audio/speech"
	}

	// Build request body
	reqBody := map[string]any{
		"model":           "tts-1",
		"input":           text,
		"voice":           voice,
		"response_format": "mp3",
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("TTS API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Read audio data
	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read audio response: %w", err)
	}

	// Return base64 encoded audio
	return base64.StdEncoding.EncodeToString(audioBytes), nil
}

// executeCalculator evaluates a simple arithmetic expression
func executeCalculator(ctx context.Context, args map[string]any) (string, error) {
	expr, ok := args["expression"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'expression' argument")
	}

	result, err := evalExpression(expr)
	if err != nil {
		return "", fmt.Errorf("calculation error: %w", err)
	}

	return fmt.Sprintf("%.6g", result), nil
}

// evalExpression evaluates a simple arithmetic expression
// Supports: +, -, *, /, %, and parentheses
func evalExpression(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	// Simple recursive descent parser for basic arithmetic
	pos := 0
	return parseExpression(expr, &pos)
}

func parseExpression(expr string, pos *int) (float64, error) {
	result, err := parseTerm(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) {
		op := expr[*pos]
		if op != '+' && op != '-' {
			break
		}
		*pos++
		right, err := parseTerm(expr, pos)
		if err != nil {
			return 0, err
		}
		if op == '+' {
			result += right
		} else {
			result -= right
		}
	}
	return result, nil
}

func parseTerm(expr string, pos *int) (float64, error) {
	result, err := parseFactor(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) {
		op := expr[*pos]
		if op != '*' && op != '/' && op != '%' {
			break
		}
		*pos++
		right, err := parseFactor(expr, pos)
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			result *= right
		case '/':
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			result /= right
		case '%':
			if right == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			result = math.Mod(result, right)
		}
	}
	return result, nil
}

func parseFactor(expr string, pos *int) (float64, error) {
	if *pos >= len(expr) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	// Handle parentheses
	if expr[*pos] == '(' {
		*pos++
		result, err := parseExpression(expr, pos)
		if err != nil {
			return 0, err
		}
		if *pos >= len(expr) || expr[*pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		*pos++
		return result, nil
	}

	// Handle negative numbers
	negative := false
	if expr[*pos] == '-' {
		negative = true
		*pos++
	}

	// Parse number
	start := *pos
	for *pos < len(expr) && (isDigit(expr[*pos]) || expr[*pos] == '.') {
		*pos++
	}

	if start == *pos {
		return 0, fmt.Errorf("expected number at position %d", start)
	}

	num, err := strconv.ParseFloat(expr[start:*pos], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", expr[start:*pos])
	}

	if negative {
		num = -num
	}
	return num, nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// executeWebSearch performs a web search (placeholder implementation)
func executeWebSearch(ctx context.Context, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'query' argument")
	}

	// Use DuckDuckGo instant answer API (no API key required)
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("web search failed: status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Abstract      string `json:"Abstract"`
		AbstractText  string `json:"AbstractText"`
		Answer        string `json:"Answer"`
		AnswerType    string `json:"AnswerType"`
		Definition    string `json:"Definition"`
		Heading       string `json:"Heading"`
		RelatedTopics []struct {
			Text string `json:"Text"`
		} `json:"RelatedTopics"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse search results: %w", err)
	}

	// Build response from available data
	var response strings.Builder
	response.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	if result.Answer != "" {
		response.WriteString(fmt.Sprintf("Answer: %s\n", result.Answer))
	}
	if result.AbstractText != "" {
		response.WriteString(fmt.Sprintf("Summary: %s\n", result.AbstractText))
	}
	if result.Definition != "" {
		response.WriteString(fmt.Sprintf("Definition: %s\n", result.Definition))
	}

	// Add related topics if no direct answer
	if result.Answer == "" && result.AbstractText == "" && len(result.RelatedTopics) > 0 {
		response.WriteString("Related information:\n")
		count := 0
		for _, topic := range result.RelatedTopics {
			if topic.Text == "" {
				continue
			}
			response.WriteString(fmt.Sprintf("- %s\n", topic.Text))
			count++
			if count >= 3 {
				break
			}
		}
	}

	if response.Len() == len(fmt.Sprintf("Search results for: %s\n\n", query)) {
		return fmt.Sprintf("No direct results found for '%s'. Try rephrasing your query.", query), nil
	}

	return response.String(), nil
}

// executeSetChatInfo patches the room title and/or description using bridge context.
func executeSetChatInfo(ctx context.Context, args map[string]any) (string, error) {
	rawTitle, hasTitle := args["title"]
	rawDesc, hasDesc := args["description"]
	if !hasTitle && !hasDesc {
		return "", fmt.Errorf("missing 'title' or 'description' argument")
	}

	var title string
	if hasTitle {
		if s, ok := rawTitle.(string); ok {
			title = strings.TrimSpace(s)
		} else {
			return "", fmt.Errorf("invalid 'title' argument")
		}
		if title == "" {
			return "", fmt.Errorf("title cannot be empty")
		}
	}

	var description string
	if hasDesc {
		if s, ok := rawDesc.(string); ok {
			description = strings.TrimSpace(s)
		} else {
			return "", fmt.Errorf("invalid 'description' argument")
		}
	}

	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("bridge context not available")
	}
	if btc.Portal == nil {
		return "", fmt.Errorf("portal not available")
	}

	var updates []string
	if hasTitle {
		if err := btc.Client.setRoomName(ctx, btc.Portal, title); err != nil {
			return "", fmt.Errorf("failed to set room title: %w", err)
		}
		updates = append(updates, fmt.Sprintf("title=%s", title))
	}
	if hasDesc {
		if err := btc.Client.setRoomTopic(ctx, btc.Portal, description); err != nil {
			return "", fmt.Errorf("failed to set room description: %w", err)
		}
		if description == "" {
			updates = append(updates, "description=cleared")
		} else {
			updates = append(updates, fmt.Sprintf("description=%s", description))
		}
	}

	return fmt.Sprintf("Chat info updated: %s", strings.Join(updates, ", ")), nil
}

// executeSessionStatus returns current session status including time, model, and usage info.
// Similar to OpenClaw's session_status tool.
func executeSessionStatus(ctx context.Context, args map[string]any) (string, error) {
	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("session_status tool requires bridge context")
	}

	meta := portalMeta(btc.Portal)
	if meta == nil {
		return "", fmt.Errorf("failed to get portal metadata")
	}

	// Get current time info
	now := time.Now()
	timezone := "UTC"
	if tz := os.Getenv("TZ"); tz != "" {
		timezone = tz
	}
	timeStr := now.Format("2006-01-02 15:04:05")
	dayOfWeek := now.Weekday().String()

	// Get model info
	model := meta.Model
	if model == "" {
		model = btc.Client.effectiveModel(meta)
	}

	// Parse provider from model string (format: "provider/model" or just "model")
	provider := "unknown"
	modelName := model
	if parts := strings.SplitN(model, "/", 2); len(parts) == 2 {
		provider = parts[0]
		modelName = parts[1]
	}

	// Get context/token info from metadata
	maxContext := meta.MaxContextMessages
	if maxContext == 0 {
		maxContext = 12 // default
	}
	maxTokens := meta.MaxCompletionTokens
	if maxTokens == 0 {
		maxTokens = 512 // default
	}

	// Build session info
	sessionID := string(btc.Portal.PortalKey.ID)
	title := meta.Title
	if title == "" {
		title = meta.Slug
	}
	if title == "" {
		title = "Untitled"
	}

	// Handle model change if requested
	var modelChanged string
	if newModel, ok := args["set_model"].(string); ok && newModel != "" {
		// Update the model in metadata
		meta.Model = newModel
		meta.Capabilities = getModelCapabilities(newModel, btc.Client.findModelInfo(newModel))
		// Save portal metadata
		if err := btc.Portal.Save(ctx); err != nil {
			return "", fmt.Errorf("failed to save model change: %w", err)
		}
		btc.Portal.UpdateBridgeInfo(ctx)
		btc.Client.ensureGhostDisplayName(ctx, newModel)
		modelChanged = fmt.Sprintf("\n\nModel changed to: %s", newModel)
		model = newModel
		if parts := strings.SplitN(newModel, "/", 2); len(parts) == 2 {
			provider = parts[0]
			modelName = parts[1]
		} else {
			modelName = newModel
		}
	}

	// Get agent info if available
	agentInfo := ""
	if meta.AgentID != "" {
		agentInfo = fmt.Sprintf("\nAgent: %s", meta.AgentID)
	}

	// Build status card similar to OpenClaw
	status := fmt.Sprintf(`Session Status
==============
Time: %s %s (%s)
Day: %s

Model: %s
Provider: %s
Max Context: %d messages
Max Tokens: %d

Session: %s
Chat: %s%s%s`,
		timeStr, timezone, now.Format("MST"),
		dayOfWeek,
		modelName,
		provider,
		maxContext,
		maxTokens,
		sessionID,
		title,
		agentInfo,
		modelChanged,
	)

	return status, nil
}

// GetBuiltinTool returns a builtin tool by name, or nil if not found
func GetBuiltinTool(name string) *ToolDefinition {
	for _, tool := range BuiltinTools() {
		if tool.Name == name {
			return &tool
		}
	}
	return nil
}

// GetEnabledBuiltinTools returns the list of enabled builtin tools based on config
func GetEnabledBuiltinTools(isToolEnabled func(string) bool) []ToolDefinition {
	var enabled []ToolDefinition
	for _, tool := range BuiltinTools() {
		if isToolEnabled(tool.Name) {
			enabled = append(enabled, tool)
		}
	}
	return enabled
}

// executeMemorySearch handles the memory_search tool
func executeMemorySearch(ctx context.Context, args map[string]any) (string, error) {
	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("memory_search requires bridge context")
	}

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("missing or invalid 'query' argument")
	}

	var input MemorySearchInput
	input.Query = query

	if maxResults, ok := args["maxResults"].(float64); ok {
		max := int(maxResults)
		input.MaxResults = &max
	}
	if minScore, ok := args["minScore"].(float64); ok {
		input.MinScore = &minScore
	}

	memStore := NewMemoryStore(btc.Client)
	results, err := memStore.Search(ctx, btc.Portal, input)
	if err != nil {
		return "", fmt.Errorf("memory search failed: %w", err)
	}

	// Format as JSON
	output, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("failed to format results: %w", err)
	}

	return string(output), nil
}

// executeMemoryGet handles the memory_get tool
func executeMemoryGet(ctx context.Context, args map[string]any) (string, error) {
	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("memory_get requires bridge context")
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing or invalid 'path' argument")
	}

	input := MemoryGetInput{Path: path}

	memStore := NewMemoryStore(btc.Client)
	result, err := memStore.Get(ctx, btc.Portal, input)
	if err != nil {
		return "", fmt.Errorf("memory get failed: %w", err)
	}

	// Format as JSON
	output, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return string(output), nil
}

// executeMemoryStore handles the memory_store tool
func executeMemoryStore(ctx context.Context, args map[string]any) (string, error) {
	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("memory_store requires bridge context")
	}

	content, ok := args["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("missing or invalid 'content' argument")
	}

	var input MemoryStoreInput
	input.Content = content

	if importance, ok := args["importance"].(float64); ok {
		input.Importance = &importance
	}
	if category, ok := args["category"].(string); ok {
		input.Category = &category
	}
	if scope, ok := args["scope"].(string); ok {
		input.Scope = &scope
	}

	memStore := NewMemoryStore(btc.Client)
	result, err := memStore.Store(ctx, btc.Portal, input)
	if err != nil {
		return "", fmt.Errorf("memory store failed: %w", err)
	}

	// Format as JSON
	output, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return string(output), nil
}

// executeMemoryForget handles the memory_forget tool
func executeMemoryForget(ctx context.Context, args map[string]any) (string, error) {
	btc := GetBridgeToolContext(ctx)
	if btc == nil {
		return "", fmt.Errorf("memory_forget requires bridge context")
	}

	id, ok := args["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("missing or invalid 'id' argument")
	}

	input := MemoryForgetInput{ID: id}

	memStore := NewMemoryStore(btc.Client)
	result, err := memStore.Forget(ctx, btc.Portal, input)
	if err != nil {
		return "", fmt.Errorf("memory forget failed: %w", err)
	}

	// Format as JSON
	output, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to format result: %w", err)
	}

	return string(output), nil
}

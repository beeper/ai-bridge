package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const (
	// MCPRequestTimeout is the default timeout for MCP requests
	MCPRequestTimeout = 30 * time.Second
	// MCPProtocolVersion is the MCP protocol version we support
	MCPProtocolVersion = "2024-11-05"
)

// MCPMatrixClient handles MCP communication over Matrix
type MCPMatrixClient struct {
	client *AIClient
	log    zerolog.Logger

	// pendingRequests maps request IDs to response channels
	pendingRequests map[string]chan *MCPJSONRPCBase
	mu              sync.Mutex
}

// NewMCPMatrixClient creates a new MCP client for Matrix transport
func NewMCPMatrixClient(client *AIClient) *MCPMatrixClient {
	return &MCPMatrixClient{
		client:          client,
		log:             client.log.With().Str("component", "mcp-matrix-client").Logger(),
		pendingRequests: make(map[string]chan *MCPJSONRPCBase),
	}
}

// Call sends an MCP JSON-RPC request and waits for a response
func (c *MCPMatrixClient) Call(ctx context.Context, roomID id.RoomID, deviceID string, method string, params map[string]any) (*MCPJSONRPCBase, error) {
	requestID := NewCallID()

	request := &MCPRequestContent{
		DeviceID: deviceID,
		MCP: MCPJSONRPCBase{
			JSONRPC: "2.0",
			ID:      requestID,
			Method:  method,
			Params:  params,
		},
	}

	// Create response channel
	respChan := make(chan *MCPJSONRPCBase, 1)
	c.mu.Lock()
	c.pendingRequests[requestID] = respChan
	c.mu.Unlock()

	// Clean up on exit
	defer func() {
		c.mu.Lock()
		delete(c.pendingRequests, requestID)
		c.mu.Unlock()
	}()

	// Send request via Matrix
	bot := c.client.UserLogin.Bridge.Bot
	_, err := bot.SendMessage(ctx, roomID, MCPRequestEventType, &event.Content{
		Parsed: request,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send MCP request: %w", err)
	}

	c.log.Debug().
		Str("request_id", requestID).
		Str("method", method).
		Stringer("room_id", roomID).
		Msg("Sent MCP request")

	// Wait for response with timeout
	timeout := MCPRequestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("MCP request timed out after %v", timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendNotification sends an MCP notification (no response expected)
func (c *MCPMatrixClient) SendNotification(ctx context.Context, roomID id.RoomID, deviceID string, method string, params map[string]any) error {
	notification := &MCPNotificationContent{
		DeviceID: deviceID,
		MCP: MCPJSONRPCBase{
			JSONRPC: "2.0",
			Method:  method,
			Params:  params,
		},
	}

	bot := c.client.UserLogin.Bridge.Bot
	_, err := bot.SendMessage(ctx, roomID, MCPNotificationEventType, &event.Content{
		Parsed: notification,
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to send MCP notification: %w", err)
	}

	c.log.Debug().
		Str("method", method).
		Stringer("room_id", roomID).
		Msg("Sent MCP notification")

	return nil
}

// HandleResponse processes an MCP response received from Matrix
func (c *MCPMatrixClient) HandleResponse(content *MCPResponseContent) {
	if content == nil || content.MCP.ID == nil {
		c.log.Warn().Msg("Received MCP response with nil ID")
		return
	}

	requestID, ok := content.MCP.ID.(string)
	if !ok {
		// Try to convert from float64 (JSON numbers)
		if num, ok := content.MCP.ID.(float64); ok {
			requestID = fmt.Sprintf("%v", num)
		} else {
			c.log.Warn().Interface("id", content.MCP.ID).Msg("Invalid MCP response ID type")
			return
		}
	}

	c.mu.Lock()
	respChan, exists := c.pendingRequests[requestID]
	c.mu.Unlock()

	if !exists {
		c.log.Debug().Str("request_id", requestID).Msg("Received response for unknown request")
		return
	}

	// Non-blocking send
	select {
	case respChan <- &content.MCP:
	default:
		c.log.Warn().Str("request_id", requestID).Msg("Response channel full, dropping response")
	}
}

// Initialize sends the MCP initialize request to a device
func (c *MCPMatrixClient) Initialize(ctx context.Context, roomID id.RoomID, deviceID string) (*MCPJSONRPCBase, error) {
	params := map[string]any{
		"protocolVersion": MCPProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"clientInfo": map[string]any{
			"name":    "beeper-ai-bridge",
			"version": "1.0.0",
		},
	}

	resp, err := c.Call(ctx, roomID, deviceID, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("initialize failed: %w", err)
	}

	// Send initialized notification
	if err := c.SendNotification(ctx, roomID, deviceID, "notifications/initialized", nil); err != nil {
		c.log.Warn().Err(err).Msg("Failed to send initialized notification")
	}

	return resp, nil
}

// ListTools requests the list of available tools from a device
func (c *MCPMatrixClient) ListTools(ctx context.Context, roomID id.RoomID, deviceID string) ([]MCPTool, error) {
	resp, err := c.Call(ctx, roomID, deviceID, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list failed: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}

	// Parse tools from result
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid tools/list result type")
	}

	toolsRaw, ok := result["tools"].([]any)
	if !ok {
		return nil, fmt.Errorf("invalid tools field in result")
	}

	var tools []MCPTool
	for _, t := range toolsRaw {
		toolMap, ok := t.(map[string]any)
		if !ok {
			continue
		}

		tool := MCPTool{
			Name: toolMap["name"].(string),
		}
		if desc, ok := toolMap["description"].(string); ok {
			tool.Description = desc
		}
		if schema, ok := toolMap["inputSchema"].(map[string]any); ok {
			tool.InputSchema = schema
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

// CallTool executes a tool on the device and returns the result
func (c *MCPMatrixClient) CallTool(ctx context.Context, roomID id.RoomID, deviceID string, toolName string, arguments map[string]any) (*MCPJSONRPCBase, error) {
	params := map[string]any{
		"name":      toolName,
		"arguments": arguments,
	}

	resp, err := c.Call(ctx, roomID, deviceID, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call failed: %w", err)
	}

	return resp, nil
}

// ParseToolResult extracts the content from a tools/call response
func ParseToolResult(resp *MCPJSONRPCBase) (string, error) {
	if resp.Error != nil {
		return "", fmt.Errorf("tool error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid result type")
	}

	contentRaw, ok := result["content"].([]any)
	if !ok || len(contentRaw) == 0 {
		return "", nil
	}

	// Concatenate all text content
	var text string
	for _, c := range contentRaw {
		contentMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if contentMap["type"] == "text" {
			if t, ok := contentMap["text"].(string); ok {
				text += t
			}
		}
	}

	return text, nil
}

// GetPreferredDevice returns the preferred desktop device for a user
// Falls back to the most recently seen device if no preference is set
func (c *MCPMatrixClient) GetPreferredDevice() *DesktopDeviceInfo {
	meta := loginMetadata(c.client.UserLogin)
	if len(meta.DesktopDevices) == 0 {
		return nil
	}

	// Check preferred device first
	if meta.PreferredDesktopDeviceID != "" {
		if device, ok := meta.DesktopDevices[meta.PreferredDesktopDeviceID]; ok {
			return device
		}
	}

	// Fall back to most recently seen device
	var latestDevice *DesktopDeviceInfo
	for _, device := range meta.DesktopDevices {
		if latestDevice == nil || device.LastSeen > latestDevice.LastSeen {
			latestDevice = device
		}
	}

	return latestDevice
}

// HasDesktopTools returns true if there are any MCP tools available from connected desktops
func (c *MCPMatrixClient) HasDesktopTools() bool {
	device := c.GetPreferredDevice()
	return device != nil && len(device.Tools) > 0
}

// GetDesktopTools returns the cached MCP tools from the preferred device
func (c *MCPMatrixClient) GetDesktopTools() []MCPTool {
	device := c.GetPreferredDevice()
	if device == nil {
		return nil
	}
	return device.Tools
}

// ParseMCPResponse parses a raw JSON event content into MCPResponseContent
func ParseMCPResponse(raw json.RawMessage) (*MCPResponseContent, error) {
	var content MCPResponseContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, fmt.Errorf("failed to parse MCP response: %w", err)
	}
	return &content, nil
}

// ParseMCPNotification parses a raw JSON event content into MCPNotificationContent
func ParseMCPNotification(raw json.RawMessage) (*MCPNotificationContent, error) {
	var content MCPNotificationContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, fmt.Errorf("failed to parse MCP notification: %w", err)
	}
	return &content, nil
}

// ParseDesktopHello parses a raw JSON event content into DesktopHelloContent
func ParseDesktopHello(raw json.RawMessage) (*DesktopHelloContent, error) {
	var content DesktopHelloContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, fmt.Errorf("failed to parse desktop hello: %w", err)
	}
	return &content, nil
}

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

	mcpPayload := MCPPayload{
		JSONRPC: "2.0",
		ID:      requestID,
		Method:  method,
		Params:  params,
	}

	payloadBytes, err := json.Marshal(mcpPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP payload: %w", err)
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

	// Send request via Matrix as m.room.message with custom msgtype
	if err := c.sendIPC(ctx, roomID, deviceID, IPCMCPRequest, payloadBytes); err != nil {
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
	mcpPayload := MCPPayload{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	payloadBytes, err := json.Marshal(mcpPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal MCP notification payload: %w", err)
	}

	if err := c.sendIPC(ctx, roomID, deviceID, IPCMCPNotification, payloadBytes); err != nil {
		return fmt.Errorf("failed to send MCP notification: %w", err)
	}

	c.log.Debug().
		Str("method", method).
		Stringer("room_id", roomID).
		Msg("Sent MCP notification")

	return nil
}

// sendIPC sends an IPC message using m.room.message with custom msgtype
func (c *MCPMatrixClient) sendIPC(ctx context.Context, roomID id.RoomID, deviceID string, messageType IPCMessageType, payload json.RawMessage) error {
	// Build the m.room.message content with IPC data in custom field
	content := &event.Content{
		Parsed: &event.MessageEventContent{
			MsgType: IPCMsgType,
			Body:    "[AI Bridge IPC]",
		},
		Raw: map[string]any{
			IPCMsgType: map[string]any{
				"device_id":    deviceID,
				"message_type": messageType,
				"payload":      payload,
			},
		},
	}

	bot := c.client.UserLogin.Bridge.Bot
	_, err := bot.SendMessage(ctx, roomID, event.EventMessage, content, nil)
	return err
}

// HandleResponse processes an MCP response received from Matrix
func (c *MCPMatrixClient) HandleResponse(ipc *IPCContent, payload *MCPPayload) {
	if ipc == nil || payload == nil || payload.ID == nil {
		c.log.Warn().Msg("Received MCP response with nil ID")
		return
	}

	requestID, ok := payload.ID.(string)
	if !ok {
		// Try to convert from float64 (JSON numbers)
		if num, ok := payload.ID.(float64); ok {
			requestID = fmt.Sprintf("%v", num)
		} else {
			c.log.Warn().Interface("id", payload.ID).Msg("Invalid MCP response ID type")
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

	// Convert MCPPayload to MCPJSONRPCBase for the response channel
	resp := &MCPJSONRPCBase{
		JSONRPC: payload.JSONRPC,
		ID:      payload.ID,
		Method:  payload.Method,
		Params:  payload.Params,
		Result:  payload.Result,
		Error:   payload.Error,
	}

	// Non-blocking send
	select {
	case respChan <- resp:
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

// ParseIPCContent parses a raw JSON event content into IPCContent
func ParseIPCContent(raw json.RawMessage) (*IPCContent, error) {
	var content IPCContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, fmt.Errorf("failed to parse IPC content: %w", err)
	}
	return &content, nil
}

// ParseDesktopHelloPayload parses the payload of a desktop_hello IPC message
func ParseDesktopHelloPayload(payload json.RawMessage) (*DesktopHelloPayload, error) {
	var content DesktopHelloPayload
	if err := json.Unmarshal(payload, &content); err != nil {
		return nil, fmt.Errorf("failed to parse desktop hello payload: %w", err)
	}
	return &content, nil
}

// ParseMCPPayload parses the payload of an MCP request/response/notification IPC message
func ParseMCPPayload(payload json.RawMessage) (*MCPPayload, error) {
	var content MCPPayload
	if err := json.Unmarshal(payload, &content); err != nil {
		return nil, fmt.Errorf("failed to parse MCP payload: %w", err)
	}
	return &content, nil
}

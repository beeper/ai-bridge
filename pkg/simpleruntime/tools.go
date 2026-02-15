//lint:file-ignore U1000 Hard-cut compatibility: pending full dead-code deletion.
package connector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/beeper/ai-bridge/pkg/core/aiprovider"
	"github.com/beeper/ai-bridge/pkg/core/aiutil"
	"github.com/beeper/ai-bridge/pkg/core/shared/toolspec"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"
)

// ToolDefinition defines a tool callable by the model.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
	Execute     func(ctx context.Context, args map[string]any) (string, error)
}

// toProviderToolDefs extracts the provider-facing fields from connector ToolDefinitions.
func toProviderToolDefs(tools []ToolDefinition) []aiprovider.ToolDefinition {
	out := make([]aiprovider.ToolDefinition, len(tools))
	for i, t := range tools {
		out[i] = aiprovider.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		}
	}
	return out
}

// BridgeToolContext carries runtime data for tool execution.
type BridgeToolContext struct {
	Client        *AIClient
	Portal        *bridgev2.Portal
	Meta          *PortalMetadata
	SourceEventID id.EventID
	SenderID      string
}

type bridgeToolContextKey struct{}

const (
	ToolNameMessage = toolspec.MessageName

	ImageResultPrefix  = "IMAGE:"
	ImagesResultPrefix = "IMAGES:"
	TTSResultPrefix    = "AUDIO:"
)

func WithBridgeToolContext(ctx context.Context, btc *BridgeToolContext) context.Context {
	return context.WithValue(ctx, bridgeToolContextKey{}, btc)
}

func GetBridgeToolContext(ctx context.Context) *BridgeToolContext {
	return aiutil.ContextValue[*BridgeToolContext](ctx, bridgeToolContextKey{})
}

// BuiltinTools returns builtin tools enabled by this bridge profile.
func BuiltinTools() []ToolDefinition {
	return buildBuiltinToolDefinitions()
}

func GetBuiltinTool(name string) *ToolDefinition {
	for _, tool := range BuiltinTools() {
		if tool.Name == name {
			copyTool := tool
			return &copyTool
		}
	}
	return nil
}

func GetEnabledBuiltinTools(isToolEnabled func(string) bool) []ToolDefinition {
	if isToolEnabled == nil {
		return nil
	}
	var enabled []ToolDefinition
	for _, tool := range BuiltinTools() {
		if isToolEnabled(tool.Name) {
			enabled = append(enabled, tool)
		}
	}
	return enabled
}

func isBuiltinToolEnabled(meta *PortalMetadata, name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	if meta == nil || len(meta.DisabledTools) == 0 {
		return true
	}
	for _, disabled := range meta.DisabledTools {
		if strings.EqualFold(strings.TrimSpace(disabled), trimmed) {
			return false
		}
	}
	return true
}

func executeWebSearch(_ context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", errors.New("web_search requires query")
	}
	return "", errors.New("web_search provider is not configured in simple bridge")
}

func normalizeMimeString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if semi := strings.IndexByte(value, ';'); semi >= 0 {
		value = value[:semi]
	}
	return strings.TrimSpace(value)
}

func readStringArg(args map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if raw, ok := args[key]; ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return s, true
			}
		}
	}
	return "", false
}

func jsonActionResult(action string, payload map[string]any) (string, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["action"] = action
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

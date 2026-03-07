package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func ValidateToolCall(tools []ai.Tool, toolCall ai.ContentBlock) (map[string]any, error) {
	for _, tool := range tools {
		if tool.Name == toolCall.Name {
			return ValidateToolArguments(tool, toolCall)
		}
	}
	return nil, fmt.Errorf(`tool "%s" not found`, toolCall.Name)
}

func ValidateToolArguments(tool ai.Tool, toolCall ai.ContentBlock) (map[string]any, error) {
	args := cloneMap(toolCall.Arguments)
	if args == nil {
		args = map[string]any{}
	}
	if tool.Parameters == nil {
		return args, nil
	}
	required, _ := tool.Parameters["required"].([]any)
	for _, raw := range required {
		key, ok := raw.(string)
		if !ok {
			continue
		}
		if _, exists := args[key]; !exists {
			return nil, fmt.Errorf("validation failed for tool %q: missing required field %q", toolCall.Name, key)
		}
	}

	props, _ := tool.Parameters["properties"].(map[string]any)
	for key, propSchemaRaw := range props {
		propSchema, ok := propSchemaRaw.(map[string]any)
		if !ok {
			continue
		}
		expectedType, _ := propSchema["type"].(string)
		if expectedType == "" {
			continue
		}
		value, exists := args[key]
		if !exists {
			continue
		}
		coerced, err := coerceJSONType(expectedType, value)
		if err != nil {
			return nil, fmt.Errorf("validation failed for tool %q: %s: %w", toolCall.Name, key, err)
		}
		args[key] = coerced
	}
	return args, nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func coerceJSONType(expected string, value any) (any, error) {
	switch expected {
	case "string":
		switch v := value.(type) {
		case string:
			return v, nil
		default:
			return fmt.Sprintf("%v", v), nil
		}
	case "number":
		switch v := value.(type) {
		case float64, float32, int, int64, uint64, uint32, int32:
			return toFloat64(v), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, fmt.Errorf("expected number, got %T", value)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("expected number, got %T", value)
		}
	case "integer":
		switch v := value.(type) {
		case int, int64, int32, uint64, uint32:
			return toInt64(v), nil
		case float64:
			return int64(v), nil
		case string:
			i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("expected integer, got %T", value)
			}
			return i, nil
		default:
			return nil, fmt.Errorf("expected integer, got %T", value)
		}
	case "boolean":
		switch v := value.(type) {
		case bool:
			return v, nil
		case string:
			b, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return nil, fmt.Errorf("expected boolean, got %T", value)
			}
			return b, nil
		default:
			return nil, fmt.Errorf("expected boolean, got %T", value)
		}
	case "object":
		if _, ok := value.(map[string]any); ok {
			return value, nil
		}
		return nil, fmt.Errorf("expected object, got %T", value)
	case "array":
		if _, ok := value.([]any); ok {
			return value, nil
		}
		return nil, fmt.Errorf("expected array, got %T", value)
	default:
		return value, nil
	}
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case uint64:
		return float64(n)
	case uint32:
		return float64(n)
	default:
		return 0
	}
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case int32:
		return int64(n)
	case uint64:
		return int64(n)
	case uint32:
		return int64(n)
	default:
		return 0
	}
}

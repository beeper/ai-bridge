package jsonutil

import (
	"encoding/json"
	"slices"
)

// ToMap converts a value into map[string]any. If the input is already a
// map[string]any, it is returned as-is.
func ToMap(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

// DeepCloneMap returns a deep copy of src.
func DeepCloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = DeepCloneAny(value)
	}
	return out
}

// DeepCloneAny returns a deep copy of dynamic JSON-ish values.
func DeepCloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return DeepCloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = DeepCloneAny(item)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for i, item := range typed {
			out[i] = DeepCloneMap(item)
		}
		return out
	case []string:
		return slices.Clone(typed)
	case []int:
		return slices.Clone(typed)
	case []int64:
		return slices.Clone(typed)
	case []float64:
		return slices.Clone(typed)
	case []bool:
		return slices.Clone(typed)
	default:
		return typed
	}
}

// MergeRecursive deep-merges two dynamic maps. Nested maps are merged
// recursively and all inserted values are deep-cloned.
func MergeRecursive(base, update map[string]any) map[string]any {
	if len(base) == 0 && len(update) == 0 {
		return nil
	}
	if len(base) == 0 {
		return DeepCloneMap(update)
	}
	if len(update) == 0 {
		return DeepCloneMap(base)
	}
	out := DeepCloneMap(base)
	for key, value := range update {
		if existing, ok := out[key].(map[string]any); ok {
			if next, ok := value.(map[string]any); ok {
				out[key] = MergeRecursive(existing, next)
				continue
			}
		}
		out[key] = DeepCloneAny(value)
	}
	return out
}

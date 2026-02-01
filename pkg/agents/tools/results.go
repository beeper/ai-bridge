package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// JSONResult creates a structured JSON result from any payload.
// Following clawdbot's jsonResult pattern.
func JSONResult(payload any) *Result {
	return &Result{
		Status:  ResultSuccess,
		Content: []ContentBlock{{Type: "text", Text: mustJSON(payload)}},
		Details: toMap(payload),
	}
}

// TextResult creates a simple text result.
func TextResult(text string) *Result {
	return &Result{
		Status:  ResultSuccess,
		Content: []ContentBlock{{Type: "text", Text: text}},
	}
}

// ErrorResult creates an error result.
// Follows clawdbot pattern: don't throw, return structured errors.
func ErrorResult(toolName, message string) *Result {
	return &Result{
		Status:  ResultError,
		Content: []ContentBlock{{Type: "text", Text: message}},
		Details: map[string]any{"tool": toolName, "error": message},
		Error:   message,
	}
}

// ErrorResultf creates an error result with formatted message.
func ErrorResultf(toolName, format string, args ...any) *Result {
	return ErrorResult(toolName, fmt.Sprintf(format, args...))
}

// ImageResult creates a result with image content.
func ImageResult(label, path string, data []byte, mimeType string) *Result {
	return &Result{
		Status: ResultSuccess,
		Content: []ContentBlock{
			{Type: "text", Text: fmt.Sprintf("MEDIA:%s", path)},
			{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType},
		},
		Details: map[string]any{"path": path, "label": label},
	}
}

// PartialResult creates a result indicating partial success.
func PartialResult(text string, errors []string) *Result {
	return &Result{
		Status:  ResultPartial,
		Content: []ContentBlock{{Type: "text", Text: text}},
		Details: map[string]any{"errors": errors},
	}
}

// mustJSON marshals payload to JSON, returning error message on failure.
func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":"failed to marshal: %s"}`, err.Error())
	}
	return string(data)
}

// toMap converts a struct to map[string]any for Details field.
func toMap(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// IsSuccess returns true if the result indicates success.
func (r *Result) IsSuccess() bool {
	return r.Status == ResultSuccess
}

// IsError returns true if the result indicates an error.
func (r *Result) IsError() bool {
	return r.Status == ResultError
}

// GetText returns the first text content block, if any.
func (r *Result) GetText() string {
	for _, block := range r.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}

// HasImages returns true if the result contains image blocks.
func (r *Result) HasImages() bool {
	for _, block := range r.Content {
		if block.Type == "image" {
			return true
		}
	}
	return false
}

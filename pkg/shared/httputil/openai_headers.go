package httputil

import (
	"strings"

	"github.com/openai/openai-go/v3/option"
)

// AppendHeaderOptions appends non-empty header values as OpenAI request options.
func AppendHeaderOptions(opts []option.RequestOption, headers map[string]string) []option.RequestOption {
	for key, value := range headers {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		opts = append(opts, option.WithHeader(key, trimmed))
	}
	return opts
}

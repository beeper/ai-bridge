package memory

import (
	"context"
)

type SearchOptions struct {
	SessionKey string
	MaxResults int
	MinScore float64
	Mode string
	PathPrefix string
	Sources []string
}

type SearchResult struct {
	Path string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Score float64 `json:"score,omitempty"`
	Line int `json:"line,omitempty"`
	Snippet string `json:"snippet,omitempty"`
	StartLine int `json:"start_line,omitempty"`
	EndLine int `json:"end_line,omitempty"`
}

type FallbackStatus struct {
	Active bool `json:"active,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type SearchStatus struct {
	Enabled bool `json:"enabled,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model string `json:"model,omitempty"`
	Fallback *FallbackStatus `json:"fallback,omitempty"`
}

type SearchManager struct{}

type ResolvedConfig struct {
	Enabled bool
}

func NewSearchManager(_ any) *SearchManager { return &SearchManager{} }
func (m *SearchManager) Close() error { return nil }
func (m *SearchManager) Reindex(context.Context) (int, error) { return 0, nil }
func (m *SearchManager) Search(context.Context, string, SearchOptions) ([]SearchResult, error) { return nil, nil }
func (m *SearchManager) Status() SearchStatus { return SearchStatus{Enabled: false} }
func (m *SearchManager) Get(context.Context, string) (string, error) { return "", nil }
func (m *SearchManager) Set(context.Context, string, string) error { return nil }
func (m *SearchManager) Append(context.Context, string, string) error { return nil }
func (m *SearchManager) ReadFile(context.Context, string, *int, *int) (map[string]any, error) {
	return map[string]any{"path": "", "text": ""}, nil
}

package websearch

import (
	"errors"
	"testing"
)

func TestConnectorError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "create request",
			err:     errors.New("failed to create request: boom"),
			wantMsg: "failed to create request: boom",
		},
		{
			name:    "parse results",
			err:     errors.New("failed to parse results: bad json"),
			wantMsg: "failed to parse search results: bad json",
		},
		{
			name:    "status",
			err:     errors.New("status 500: nope"),
			wantMsg: "web search failed: status 500: nope",
		},
		{
			name:    "request failed",
			err:     errors.New("request failed: dial tcp"),
			wantMsg: "web search failed: dial tcp",
		},
		{
			name:    "other",
			err:     errors.New("unknown"),
			wantMsg: "web search failed: unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ConnectorError(tc.err)
			if got == nil {
				t.Fatalf("expected error, got nil")
			}
			if got.Error() != tc.wantMsg {
				t.Fatalf("got %q, want %q", got.Error(), tc.wantMsg)
			}
		})
	}
}

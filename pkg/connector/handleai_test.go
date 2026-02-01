package connector

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeBase64Image(t *testing.T) {
	// Sample PNG header bytes (minimal valid PNG)
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	pngBase64 := base64.StdEncoding.EncodeToString(pngBytes)

	// Sample JPEG header bytes
	jpegBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	jpegBase64 := base64.StdEncoding.EncodeToString(jpegBytes)

	tests := []struct {
		name         string
		input        string
		wantErr      bool
		wantMimeType string
		errContains  string
	}{
		{
			name:         "raw base64 PNG",
			input:        pngBase64,
			wantErr:      false,
			wantMimeType: "image/png",
		},
		{
			name:         "data URL with PNG",
			input:        "data:image/png;base64," + pngBase64,
			wantErr:      false,
			wantMimeType: "image/png",
		},
		{
			name:         "data URL with JPEG",
			input:        "data:image/jpeg;base64," + jpegBase64,
			wantErr:      false,
			wantMimeType: "image/jpeg",
		},
		{
			name:         "data URL with webp",
			input:        "data:image/webp;base64," + pngBase64,
			wantErr:      false,
			wantMimeType: "image/webp",
		},
		{
			name:        "invalid data URL - no comma",
			input:       "data:image/png;base64" + pngBase64,
			wantErr:     true,
			errContains: "no comma found",
		},
		{
			name:    "invalid base64",
			input:   "not-valid-base64!!!",
			wantErr: true,
		},
		{
			name:         "URL-safe base64",
			input:        base64.URLEncoding.EncodeToString(pngBytes),
			wantErr:      false,
			wantMimeType: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, mimeType, err := decodeBase64Image(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(data) == 0 {
				t.Error("expected non-empty data")
			}

			if mimeType != tt.wantMimeType {
				t.Errorf("mimeType = %q, want %q", mimeType, tt.wantMimeType)
			}
		})
	}
}

package connector

import "testing"

func TestReadCronJobID_AcceptsJobIDAndLegacyID(t *testing.T) {
	if got := readCronJobID(map[string]any{"jobId": "job-123"}); got != "job-123" {
		t.Fatalf("expected jobId to be preferred, got %q", got)
	}
	if got := readCronJobID(map[string]any{"id": "legacy-456"}); got != "legacy-456" {
		t.Fatalf("expected legacy id alias to be accepted, got %q", got)
	}
	if got := readCronJobID(map[string]any{"jobId": "  ", "id": "fallback-789"}); got != "fallback-789" {
		t.Fatalf("expected fallback to id alias when jobId is empty, got %q", got)
	}
}

package connector

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSearchUsersRequiresLogin(t *testing.T) {
	oc := &AIClient{}
	_, err := oc.SearchUsers(context.Background(), "gpt")
	if err == nil {
		t.Fatalf("expected login error from SearchUsers")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "logged in") {
		t.Fatalf("expected logged-in message, got: %v", err)
	}
}

func TestGetContactListRequiresLogin(t *testing.T) {
	oc := &AIClient{}
	_, err := oc.GetContactList(context.Background())
	if err == nil {
		t.Fatalf("expected login error from GetContactList")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "logged in") {
		t.Fatalf("expected logged-in message, got: %v", err)
	}
}

func TestModelRedirectTarget(t *testing.T) {
	tests := []struct {
		name     string
		request  string
		resolved string
		wantSet  bool
	}{
		{name: "same", request: "openrouter/openai/gpt-4.1", resolved: "openrouter/openai/gpt-4.1", wantSet: false},
		{name: "different", request: "my-alias", resolved: "openrouter/openai/gpt-4.1", wantSet: true},
		{name: "empty request", request: "", resolved: "openrouter/openai/gpt-4.1", wantSet: false},
		{name: "empty resolved", request: "my-alias", resolved: "", wantSet: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := modelRedirectTarget(tc.request, tc.resolved)
			if tc.wantSet && got == "" {
				t.Fatalf("expected redirect target for request=%q resolved=%q", tc.request, tc.resolved)
			}
			if !tc.wantSet && got != "" {
				t.Fatalf("expected no redirect target, got %q", got)
			}
		})
	}
}

func TestDMModelSwitchBlockedError(t *testing.T) {
	err := dmModelSwitchBlockedError("anthropic/claude-3.7-sonnet")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrDMGhostImmutable) {
		t.Fatalf("expected ErrDMGhostImmutable, got %v", err)
	}
	if !strings.Contains(err.Error(), "requires creating a new chat") {
		t.Fatalf("expected guidance in error, got %v", err)
	}
}

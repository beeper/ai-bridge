package sdk

import (
	"context"
	"testing"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func TestSDKClientResolveIdentifierPreservesFullResponse(t *testing.T) {
	chat := &bridgev2.CreateChatResponse{
		PortalKey: networkid.PortalKey{ID: "portal-1", Receiver: "login-1"},
	}
	cfg := &Config{
		ResolveIdentifier: func(_ context.Context, _ any, id string, createChat bool) (*IdentifierResult, error) {
			if id != "agent:test" {
				t.Fatalf("unexpected identifier %q", id)
			}
			if !createChat {
				t.Fatalf("expected createChat to propagate")
			}
			return &bridgev2.ResolveIdentifierResponse{
				UserID: networkid.UserID("agent-user"),
				UserInfo: &bridgev2.UserInfo{
					Name:        ptr.Ptr("Agent"),
					Identifiers: []string{"agent:test"},
				},
				Chat: chat,
			}, nil
		},
	}
	client := newSDKClient(&bridgev2.UserLogin{UserLogin: &database.UserLogin{ID: "login-1"}}, cfg)
	resp, err := client.ResolveIdentifier(context.Background(), "agent:test", true)
	if err != nil {
		t.Fatalf("ResolveIdentifier returned error: %v", err)
	}
	if resp == nil || resp.UserID != "agent-user" {
		t.Fatalf("unexpected resolve response: %#v", resp)
	}
	if resp.Chat != chat {
		t.Fatalf("expected chat response to be preserved")
	}
	if resp.UserInfo == nil || len(resp.UserInfo.Identifiers) != 1 || resp.UserInfo.Identifiers[0] != "agent:test" {
		t.Fatalf("unexpected user info: %#v", resp.UserInfo)
	}
}

func TestSDKClientContactListingAndSearch(t *testing.T) {
	contact := &bridgev2.ResolveIdentifierResponse{UserID: "agent-user"}
	cfg := &Config{
		GetContactList: func(_ context.Context, _ any) ([]*IdentifierResult, error) {
			return []*IdentifierResult{contact}, nil
		},
		SearchUsers: func(_ context.Context, _ any, query string) ([]*IdentifierResult, error) {
			if query != "agent" {
				t.Fatalf("unexpected query %q", query)
			}
			return []*IdentifierResult{contact}, nil
		},
	}
	client := newSDKClient(&bridgev2.UserLogin{}, cfg)

	contacts, err := client.GetContactList(context.Background())
	if err != nil {
		t.Fatalf("GetContactList returned error: %v", err)
	}
	if len(contacts) != 1 || contacts[0] != contact {
		t.Fatalf("unexpected contacts: %#v", contacts)
	}

	results, err := client.SearchUsers(context.Background(), "agent")
	if err != nil {
		t.Fatalf("SearchUsers returned error: %v", err)
	}
	if len(results) != 1 || results[0] != contact {
		t.Fatalf("unexpected results: %#v", results)
	}
}

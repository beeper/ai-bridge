package connector

import (
	"testing"

	beeperdesktopapi "github.com/beeper/desktop-api-go"
)

func TestParseDesktopAPIAddArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantN   string
		wantT   string
		wantURL string
		wantErr bool
	}{
		{name: "token only", args: []string{"tok"}, wantN: desktopDefaultInstance, wantT: "tok"},
		{name: "token and base url", args: []string{"tok", "https://example.test"}, wantN: desktopDefaultInstance, wantT: "tok", wantURL: "https://example.test"},
		{name: "name and token", args: []string{"work", "tok"}, wantN: "work", wantT: "tok"},
		{name: "name token and base url", args: []string{"work", "tok", "https://example.test"}, wantN: "work", wantT: "tok", wantURL: "https://example.test"},
		{name: "empty", args: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotN, gotT, gotURL, err := parseDesktopAPIAddArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error mismatch: got=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if gotN != tt.wantN || gotT != tt.wantT || gotURL != tt.wantURL {
				t.Fatalf("unexpected parse: got (%q,%q,%q) want (%q,%q,%q)", gotN, gotT, gotURL, tt.wantN, tt.wantT, tt.wantURL)
			}
		})
	}
}

func TestMatchDesktopChatsByLabelAliases(t *testing.T) {
	chats := []beeperdesktopapi.Chat{
		{ID: "c1", Title: "Family", AccountID: "acc-wa"},
		{ID: "c2", Title: "Family", AccountID: "acc-ig"},
	}
	accounts := map[string]beeperdesktopapi.Account{
		"acc-wa": {AccountID: "acc-wa", Network: "whatsapp"},
		"acc-ig": {AccountID: "acc-ig", Network: "instagram"},
	}

	exact, _ := matchDesktopChatsByLabel(chats, "family", accounts)
	if len(exact) != 2 {
		t.Fatalf("expected 2 exact matches for plain title, got %d", len(exact))
	}

	exact, _ = matchDesktopChatsByLabel(chats, "whatsapp:family", accounts)
	if len(exact) != 1 || exact[0].ID != "c1" {
		t.Fatalf("expected whatsapp-qualified label to resolve c1, got %+v", exact)
	}

	exact, _ = matchDesktopChatsByLabel(chats, "acc-ig/family", accounts)
	if len(exact) != 1 || exact[0].ID != "c2" {
		t.Fatalf("expected account-qualified label to resolve c2, got %+v", exact)
	}
}

func TestFilterDesktopChatsByResolveOptions(t *testing.T) {
	chats := []beeperdesktopapi.Chat{
		{ID: "c1", Title: "Family", AccountID: "acc-wa"},
		{ID: "c2", Title: "Family", AccountID: "acc-ig"},
	}
	accounts := map[string]beeperdesktopapi.Account{
		"acc-wa": {AccountID: "acc-wa", Network: "whatsapp"},
		"acc-ig": {AccountID: "acc-ig", Network: "instagram"},
	}

	filtered := filterDesktopChatsByResolveOptions(chats, accounts, desktopLabelResolveOptions{AccountID: "acc-wa"})
	if len(filtered) != 1 || filtered[0].ID != "c1" {
		t.Fatalf("account filter failed: %+v", filtered)
	}

	filtered = filterDesktopChatsByResolveOptions(chats, accounts, desktopLabelResolveOptions{Network: "instagram"})
	if len(filtered) != 1 || filtered[0].ID != "c2" {
		t.Fatalf("network filter failed: %+v", filtered)
	}
}

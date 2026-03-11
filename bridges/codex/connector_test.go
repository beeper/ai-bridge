package codex

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote/pkg/bridgeadapter"
)

func TestFillPortalBridgeInfoSetsAIRoomType(t *testing.T) {
	conn := NewConnector()
	portal := &bridgev2.Portal{Portal: &database.Portal{RoomType: database.RoomTypeDM}}
	content := &event.BridgeEventContent{}

	conn.FillPortalBridgeInfo(portal, content)
	if content.BeeperRoomTypeV2 != "dm" {
		t.Fatalf("expected dm room type, got %q", content.BeeperRoomTypeV2)
	}
	if content.Protocol.ID != "ai-codex" {
		t.Fatalf("expected ai-codex protocol, got %q", content.Protocol.ID)
	}
}

func TestGetCapabilitiesEnablesContactListProvisioning(t *testing.T) {
	conn := NewConnector()
	caps := conn.GetCapabilities()
	if caps == nil {
		t.Fatal("expected capabilities")
	}
	if !caps.Provisioning.ResolveIdentifier.ContactList {
		t.Fatal("expected contact list provisioning to be enabled")
	}
}

func TestHostAuthLoginIDUsesDedicatedPrefix(t *testing.T) {
	conn := NewConnector()
	mxid := id.UserID("@alice:example.com")

	got := conn.hostAuthLoginID(mxid)
	manual := bridgeadapter.MakeUserLoginID("codex", mxid, 1)

	if got == manual {
		t.Fatalf("expected host-auth login id to differ from manual login id, got %q", got)
	}
	if !strings.HasPrefix(string(got), hostAuthLoginPrefix+":") {
		t.Fatalf("expected host-auth login id to use %q prefix, got %q", hostAuthLoginPrefix, got)
	}
}

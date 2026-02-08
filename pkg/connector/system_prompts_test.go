package connector

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func TestBuildSessionIdentityHint_IncludesRoomIDAndPortalID(t *testing.T) {
	portal := &bridgev2.Portal{Portal: &database.Portal{}}
	portal.MXID = id.RoomID("!room:example.org")
	portal.PortalKey = networkid.PortalKey{ID: networkid.PortalID("portal-123")}

	meta := &PortalMetadata{AgentID: "beeper"}
	got := buildSessionIdentityHint(portal, meta)
	if got == "" {
		t.Fatalf("expected non-empty hint")
	}
	if !strings.Contains(got, "sessionKey=!room:example.org") {
		t.Fatalf("expected hint to include sessionKey room id, got %q", got)
	}
	if !strings.Contains(got, "portalId=portal-123") {
		t.Fatalf("expected hint to include portalId, got %q", got)
	}
	if !strings.Contains(got, "agentId=beeper") {
		t.Fatalf("expected hint to include agentId, got %q", got)
	}
}

func TestBuildSessionIdentityHint_CronRoomIncludesJobID(t *testing.T) {
	portal := &bridgev2.Portal{Portal: &database.Portal{}}
	portal.MXID = id.RoomID("!cron:example.org")
	meta := &PortalMetadata{IsCronRoom: true, CronJobID: "job-1"}
	got := buildSessionIdentityHint(portal, meta)
	if !strings.Contains(got, "cronJobId=job-1") {
		t.Fatalf("expected cron job id in hint, got %q", got)
	}
}

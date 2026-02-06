package connector

import (
	"testing"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/cron"
)

func TestReadCronJobID_RequiresCanonicalJobID(t *testing.T) {
	if got := readCronJobID(map[string]any{"jobId": "job-123"}); got != "job-123" {
		t.Fatalf("expected jobId to be preferred, got %q", got)
	}
	if got := readCronJobID(map[string]any{"id": "legacy-456"}); got != "" {
		t.Fatalf("expected legacy id alias to be rejected, got %q", got)
	}
	if got := readCronJobID(map[string]any{"jobId": "  ", "id": "fallback-789"}); got != "" {
		t.Fatalf("expected empty job id when canonical jobId is blank, got %q", got)
	}
}

func TestInjectCronContext_SetsDeliveryTargetToCurrentRoom(t *testing.T) {
	job := cron.CronJobCreate{
		SessionTarget: cron.CronSessionIsolated,
		Payload:       cron.CronPayload{Kind: "agentTurn", Message: "Ping"},
		Delivery:      &cron.CronDelivery{Mode: cron.CronDeliveryAnnounce},
	}
	btc := &BridgeToolContext{
		Portal: &bridgev2.Portal{Portal: &database.Portal{MXID: id.RoomID("!room:example.org")}},
		Meta:   &PortalMetadata{AgentID: "beeper"},
	}

	injectCronContext(&job, btc)

	if job.AgentID == nil || *job.AgentID != "beeper" {
		t.Fatalf("expected agent id to be injected, got %#v", job.AgentID)
	}
	if job.Delivery == nil {
		t.Fatalf("expected delivery to stay defined")
	}
	if job.Delivery.Channel != "matrix" {
		t.Fatalf("expected delivery channel matrix, got %q", job.Delivery.Channel)
	}
	if job.Delivery.To != "!room:example.org" {
		t.Fatalf("expected delivery target room to be injected, got %q", job.Delivery.To)
	}
}

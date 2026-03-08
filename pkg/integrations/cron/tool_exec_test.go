package cron

import "testing"

func TestInjectToolContextSetsAgentID(t *testing.T) {
	job := JobCreate{
		Payload:  Payload{Kind: "agentTurn", Message: "Ping"},
		Delivery: &Delivery{Mode: DeliveryAnnounce},
	}
	injectToolContext(&job, func() ToolCreateContext {
		return ToolCreateContext{
			AgentID:        "main",
			SourceInternal: false,
			SourceRoomID:   "!room:example.org",
		}
	})
	if job.AgentID == nil || *job.AgentID != "main" {
		t.Fatalf("expected agent id to be set")
	}
	if job.Delivery == nil {
		t.Fatal("expected delivery to remain present")
	}
	if job.Delivery.To != "" {
		t.Fatalf("expected delivery target to remain unset, got %q", job.Delivery.To)
	}
}

func TestInjectToolContextSkipsDeliveryTargetForInternalSource(t *testing.T) {
	job := JobCreate{
		Payload:  Payload{Kind: "agentTurn", Message: "Ping"},
		Delivery: &Delivery{Mode: DeliveryAnnounce},
	}
	injectToolContext(&job, func() ToolCreateContext {
		return ToolCreateContext{
			AgentID:        "main",
			SourceInternal: true,
			SourceRoomID:   "!cronroom:example.org",
		}
	})
	if job.Delivery == nil {
		t.Fatal("expected delivery to remain present")
	}
	if job.Delivery.To != "" {
		t.Fatalf("expected delivery target to remain unset for internal source, got %q", job.Delivery.To)
	}
}

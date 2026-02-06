package cron

import (
	"context"
	"testing"
)

type testStoreBackend struct {
	files map[string][]byte
}

func (b *testStoreBackend) Read(_ context.Context, path string) ([]byte, bool, error) {
	if b.files == nil {
		return nil, false, nil
	}
	val, ok := b.files[path]
	if !ok {
		return nil, false, nil
	}
	return val, true, nil
}

func (b *testStoreBackend) Write(_ context.Context, path string, data []byte) error {
	if b.files == nil {
		b.files = map[string][]byte{}
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	b.files[path] = cp
	return nil
}

func TestLoadCronStoreMigratesLegacyJobFields(t *testing.T) {
	const storePath = "cron/jobs.json"
	backend := &testStoreBackend{
		files: map[string][]byte{
			storePath: []byte(`{
  "version": 1,
  "jobs": [
    {
      "id": "job-1",
      "name": "Legacy job",
      "enabled": true,
      "createdAtMs": 1700000000000,
      "updatedAtMs": 1700000000000,
      "schedule": { "kind": "at", "atMs": 1700000000000 },
      "sessionTarget": "isolated",
      "payload": {
        "kind": "agentTurn",
        "message": "hi",
        "deliver": true,
        "channel": "telegram",
        "to": "7200373102",
        "bestEffortDeliver": true
      },
      "isolation": { "postToMainPrefix": "Cron" },
      "state": {}
    }
  ]
}`),
		},
	}

	store, err := LoadCronStore(context.Background(), backend, storePath)
	if err != nil {
		t.Fatalf("LoadCronStore failed: %v", err)
	}
	if len(store.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(store.Jobs))
	}
	job := store.Jobs[0]
	if job.Schedule.Kind != "at" {
		t.Fatalf("expected schedule.kind=at, got %q", job.Schedule.Kind)
	}
	if job.Schedule.At != "2023-11-14T22:13:20.000Z" {
		t.Fatalf("expected migrated schedule.at, got %q", job.Schedule.At)
	}
	if job.WakeMode != CronWakeNextHeartbeat {
		t.Fatalf("expected default wakeMode=%q, got %q", CronWakeNextHeartbeat, job.WakeMode)
	}
	if job.Delivery == nil {
		t.Fatalf("expected default delivery for isolated agentTurn job")
	}
	if job.Delivery.Mode != CronDeliveryAnnounce {
		t.Fatalf("expected default delivery.mode=announce, got %q", job.Delivery.Mode)
	}
	if job.Delivery.Channel != "" {
		t.Fatalf("expected legacy payload channel to be ignored, got %q", job.Delivery.Channel)
	}
	if job.Delivery.To != "" {
		t.Fatalf("expected legacy payload recipient to be ignored, got %q", job.Delivery.To)
	}
	if job.Delivery.BestEffort != nil {
		t.Fatalf("expected legacy payload bestEffort to be ignored")
	}
}

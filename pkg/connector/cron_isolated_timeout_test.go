package connector

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/cron"
)

func TestResolveCronIsolatedTimeoutMs_Default(t *testing.T) {
	job := cron.CronJob{}
	got := resolveCronIsolatedTimeoutMs(job, &Config{})
	want := int64(defaultCronIsolatedTimeoutSeconds * 1000)
	if got != want {
		t.Fatalf("expected default timeout %dms, got %dms", want, got)
	}
}

func TestResolveCronIsolatedTimeoutMs_ConfigDefault(t *testing.T) {
	job := cron.CronJob{}
	cfg := &Config{
		Agents: &AgentsConfig{
			Defaults: &AgentDefaultsConfig{
				TimeoutSeconds: 90,
			},
		},
	}
	got := resolveCronIsolatedTimeoutMs(job, cfg)
	want := int64(90 * 1000)
	if got != want {
		t.Fatalf("expected config timeout %dms, got %dms", want, got)
	}
}

func TestResolveCronIsolatedTimeoutMs_OverrideSeconds(t *testing.T) {
	override := 42
	job := cron.CronJob{
		Payload: cron.CronPayload{
			TimeoutSeconds: &override,
		},
	}
	got := resolveCronIsolatedTimeoutMs(job, &Config{})
	want := int64(42 * 1000)
	if got != want {
		t.Fatalf("expected override timeout %dms, got %dms", want, got)
	}
}

func TestResolveCronIsolatedTimeoutMs_ZeroMeansNoTimeout(t *testing.T) {
	override := 0
	job := cron.CronJob{
		Payload: cron.CronPayload{
			TimeoutSeconds: &override,
		},
	}
	got := resolveCronIsolatedTimeoutMs(job, &Config{})
	if got != noTimeoutMs {
		t.Fatalf("expected no-timeout %dms, got %dms", noTimeoutMs, got)
	}
}

func TestResolveCronIsolatedTimeoutMs_NegativeFallsBackToDefault(t *testing.T) {
	override := -1
	job := cron.CronJob{
		Payload: cron.CronPayload{
			TimeoutSeconds: &override,
		},
	}
	got := resolveCronIsolatedTimeoutMs(job, &Config{})
	want := int64(defaultCronIsolatedTimeoutSeconds * 1000)
	if got != want {
		t.Fatalf("expected fallback timeout %dms, got %dms", want, got)
	}
}

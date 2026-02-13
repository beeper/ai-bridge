package agents

import (
	"testing"
	"time"
)

func TestDecideSoulEvil_PurgeOverridesChance(t *testing.T) {
	cfg := &SoulEvilConfig{
		Chance: 0.0,
		Purge:  &SoulEvilPurge{At: "10:00", Duration: "30m"},
	}
	now := time.Date(2026, 2, 3, 10, 5, 0, 0, time.UTC)
	decision := DecideSoulEvil(SoulEvilCheckParams{
		Config:       cfg,
		UserTimezone: "UTC",
		Now:          now,
		Random:       func() float64 { return 0.99 },
	})
	if !decision.UseEvil || decision.Reason != "purge" {
		t.Fatalf("expected purge decision, got %+v", decision)
	}
}

func TestDecideSoulEvil_Chance(t *testing.T) {
	cfg := &SoulEvilConfig{Chance: 0.5}
	decision := DecideSoulEvil(SoulEvilCheckParams{
		Config:       cfg,
		UserTimezone: "UTC",
		Now:          time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC),
		Random:       func() float64 { return 0.1 },
	})
	if !decision.UseEvil || decision.Reason != "chance" {
		t.Fatalf("expected chance decision, got %+v", decision)
	}

	decision = DecideSoulEvil(SoulEvilCheckParams{
		Config:       cfg,
		UserTimezone: "UTC",
		Now:          time.Date(2026, 2, 3, 12, 0, 0, 0, time.UTC),
		Random:       func() float64 { return 0.9 },
	})
	if decision.UseEvil {
		t.Fatalf("expected no decision, got %+v", decision)
	}
}

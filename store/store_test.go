package store

import (
	"context"
	"database/sql"
	"testing"

	"go.mau.fi/util/dbutil"

	_ "github.com/mattn/go-sqlite3"

	"github.com/beeper/agentremote/pkg/aidb"
)

func TestNewScopeTrimsIdentifiers(t *testing.T) {
	scope := NewScope(&dbutil.Database{}, " bridge ", " login ", " agent ")
	if scope == nil {
		t.Fatal("expected scope")
	}
	if scope.BridgeID != "bridge" || scope.LoginID != "login" || scope.AgentID != "agent" {
		t.Fatalf("expected trimmed identifiers, got %#v", scope)
	}
}

func TestNewScopeForLoginNilLogin(t *testing.T) {
	if scope := NewScopeForLogin(nil, "agent"); scope != nil {
		t.Fatalf("expected nil scope for nil login, got %#v", scope)
	}
}

func TestScopeAccessorsReturnStores(t *testing.T) {
	scope := NewScope(&dbutil.Database{}, "bridge", "login", "agent")
	if scope.Sessions() == nil || scope.SystemEvents() == nil || scope.Approvals() == nil {
		t.Fatal("expected all scoped stores")
	}
}

func TestStoresAreNilSafe(t *testing.T) {
	ctx := context.Background()

	if err := (&ApprovalStore{}).Upsert(ctx, ApprovalRecord{}); err != nil {
		t.Fatalf("expected nil-safe approval upsert, got %v", err)
	}
	if record, ok, err := (&ApprovalStore{}).Get(ctx, "approval"); err != nil || ok || record != (ApprovalRecord{}) {
		t.Fatalf("expected nil-safe approval get, got record=%#v ok=%v err=%v", record, ok, err)
	}

	if err := (&SessionStore{}).Upsert(ctx, SessionRecord{}); err != nil {
		t.Fatalf("expected nil-safe session upsert, got %v", err)
	}
	if record, ok, err := (&SessionStore{}).Get(ctx, "session"); err != nil || ok || record != (SessionRecord{}) {
		t.Fatalf("expected nil-safe session get, got record=%#v ok=%v err=%v", record, ok, err)
	}

	if err := (&SystemEventStore{}).Replace(ctx, nil); err != nil {
		t.Fatalf("expected nil-safe system event replace, got %v", err)
	}
	if queues, err := (&SystemEventStore{}).Load(ctx); err != nil || queues != nil {
		t.Fatalf("expected nil-safe system event load, got queues=%#v err=%v", queues, err)
	}
}

func TestSessionHelpers(t *testing.T) {
	if got := normalizeAgentID(""); got != "beep" {
		t.Fatalf("expected default normalized agent id, got %q", got)
	}
	if got := normalizeAgentID(" custom "); got != "custom" {
		t.Fatalf("expected trimmed agent id, got %q", got)
	}

	if got := nullableInt(sql.NullInt64{}); got != nil {
		t.Fatalf("expected nil nullable int for invalid raw value, got %#v", got)
	}
	value := nullableInt(sql.NullInt64{Int64: 42, Valid: true})
	if value == nil || *value != 42 {
		t.Fatalf("expected concrete int value, got %#v", value)
	}

	if got := nullableInt64Value(nil); got != nil {
		t.Fatalf("expected nil nullable int64 value, got %#v", got)
	}
	if got := nullableInt64Value(value); got != int64(42) {
		t.Fatalf("expected int64 conversion, got %#v", got)
	}
}

func TestSystemEventStoreIsAgentScoped(t *testing.T) {
	ctx := context.Background()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer raw.Close()
	db, err := dbutil.NewWithDB(raw, "sqlite3")
	if err != nil {
		t.Fatalf("wrap db: %v", err)
	}
	child := aidb.NewChild(db, dbutil.NoopLogger)
	if err := aidb.Upgrade(ctx, child, "ai_bridge", "database not initialized"); err != nil {
		t.Fatalf("upgrade child db: %v", err)
	}

	scopeA := NewScope(child, "bridge", "login", "agent-a")
	scopeB := NewScope(child, "bridge", "login", "agent-b")
	queueA := []SystemEventQueue{{SessionKey: "s", Events: []SystemEvent{{Text: "a", TS: 1}}, LastText: "last-a"}}
	queueB := []SystemEventQueue{{SessionKey: "s", Events: []SystemEvent{{Text: "b", TS: 2}}, LastText: "last-b"}}

	if err := scopeA.SystemEvents().Replace(ctx, queueA); err != nil {
		t.Fatalf("replace agent-a queues: %v", err)
	}
	if err := scopeB.SystemEvents().Replace(ctx, queueB); err != nil {
		t.Fatalf("replace agent-b queues: %v", err)
	}

	gotA, err := scopeA.SystemEvents().Load(ctx)
	if err != nil {
		t.Fatalf("load agent-a queues: %v", err)
	}
	gotB, err := scopeB.SystemEvents().Load(ctx)
	if err != nil {
		t.Fatalf("load agent-b queues: %v", err)
	}
	if len(gotA) != 1 || len(gotA[0].Events) != 1 || gotA[0].Events[0].Text != "a" {
		t.Fatalf("unexpected agent-a queues: %#v", gotA)
	}
	if len(gotB) != 1 || len(gotB[0].Events) != 1 || gotB[0].Events[0].Text != "b" {
		t.Fatalf("unexpected agent-b queues: %#v", gotB)
	}
}

package textfs

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/util/dbutil"
)

func setupStatDB(t *testing.T) *dbutil.Database {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db, err := dbutil.NewWithDB(raw, "sqlite3")
	if err != nil {
		t.Fatalf("wrap db: %v", err)
	}
	ctx := context.Background()
	_, err = db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS ai_memory_files (
			bridge_id TEXT NOT NULL,
			login_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			path TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'memory',
			content TEXT NOT NULL,
			hash TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (bridge_id, login_id, agent_id, path)
		);
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func TestStatFile(t *testing.T) {
	ctx := context.Background()
	db := setupStatDB(t)
	store := NewStore(db, "bridge", "login", "agent")

	if _, err := store.Write(ctx, "notes/plan.md", "hello"); err != nil {
		t.Fatalf("write: %v", err)
	}

	stat, err := store.Stat(ctx, "notes/plan.md")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if stat.Type != StatTypeFile {
		t.Fatalf("expected file type, got %q", stat.Type)
	}
	if stat.Size != 5 {
		t.Fatalf("unexpected size: %d", stat.Size)
	}
	if stat.Path != "notes/plan.md" {
		t.Fatalf("unexpected path: %s", stat.Path)
	}
	if stat.Hash == "" {
		t.Fatal("expected hash to be set")
	}
	if stat.UpdatedAt == 0 {
		t.Fatal("expected updated_at to be set")
	}
}

func TestStatDir(t *testing.T) {
	ctx := context.Background()
	db := setupStatDB(t)
	store := NewStore(db, "bridge", "login", "agent")

	if _, err := store.Write(ctx, "memory/2026-01-01.md", "one"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := store.Write(ctx, "memory/2026-01-02.md", "two"); err != nil {
		t.Fatalf("write: %v", err)
	}

	stat, err := store.Stat(ctx, "memory")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if stat.Type != StatTypeDir {
		t.Fatalf("expected dir type, got %q", stat.Type)
	}
	if stat.Entries != 2 {
		t.Fatalf("expected 2 entries, got %d", stat.Entries)
	}
}

func TestStatMissing(t *testing.T) {
	ctx := context.Background()
	db := setupStatDB(t)
	store := NewStore(db, "bridge", "login", "agent")

	if _, err := store.Stat(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing path")
	}
}

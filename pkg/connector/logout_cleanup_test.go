package connector

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/util/dbutil"
)

func TestPurgeAIMemoryTablesBestEffort(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	db, err := dbutil.NewWithDB(raw, "sqlite3")
	if err != nil {
		t.Fatalf("dbutil: %v", err)
	}

	ctx := context.Background()

	// Minimal schema needed for purge.
	stmts := []string{
		`CREATE TABLE ai_memory_files (bridge_id TEXT, login_id TEXT, agent_id TEXT, path TEXT, source TEXT, content TEXT, hash TEXT, updated_at INTEGER);`,
		`CREATE TABLE ai_memory_chunks (id TEXT, bridge_id TEXT, login_id TEXT, agent_id TEXT, path TEXT, source TEXT, start_line INTEGER, end_line INTEGER, hash TEXT, model TEXT, text TEXT, embedding TEXT, updated_at INTEGER);`,
		`CREATE TABLE ai_memory_meta (bridge_id TEXT, login_id TEXT, agent_id TEXT, provider TEXT, model TEXT, provider_key TEXT, chunk_tokens INTEGER, chunk_overlap INTEGER, vector_dims INTEGER, index_generation TEXT, updated_at INTEGER);`,
		`CREATE TABLE ai_memory_session_state (bridge_id TEXT, login_id TEXT, agent_id TEXT, session_key TEXT, last_rowid INTEGER, pending_bytes INTEGER, pending_messages INTEGER, updated_at INTEGER);`,
		`CREATE TABLE ai_memory_session_files (bridge_id TEXT, login_id TEXT, agent_id TEXT, session_key TEXT, path TEXT, content TEXT, hash TEXT, size INTEGER, updated_at INTEGER);`,
		`CREATE TABLE ai_memory_embedding_cache (bridge_id TEXT, login_id TEXT, agent_id TEXT, provider TEXT, model TEXT, provider_key TEXT, hash TEXT, embedding TEXT, dims INTEGER, updated_at INTEGER);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(ctx, stmt); err != nil {
			t.Fatalf("schema exec failed: %v\nstmt=%s", err, stmt)
		}
	}

	bridgeID := "b1"
	loginID := "l1"

	// Seed rows for target login.
	seed := []string{
		`INSERT INTO ai_memory_files VALUES ('b1','l1','default','MEMORY.md','memory','x','h',1);`,
		`INSERT INTO ai_memory_chunks VALUES ('c1','b1','l1','default','MEMORY.md','memory',1,1,'h','m','t','e',1);`,
		`INSERT INTO ai_memory_meta VALUES ('b1','l1','default','p','m','k',10,1,NULL,'g',1);`,
		`INSERT INTO ai_memory_session_state VALUES ('b1','l1','default','s',0,0,0,1);`,
		`INSERT INTO ai_memory_session_files VALUES ('b1','l1','default','s','sessions/s.jsonl','x','h',1,1);`,
		`INSERT INTO ai_memory_embedding_cache VALUES ('b1','l1','default','p','m','k','h','[]',1,1);`,
		// And some rows for a different login to ensure we don't over-delete.
		`INSERT INTO ai_memory_files VALUES ('b1','l2','default','MEMORY.md','memory','x','h',1);`,
		`INSERT INTO ai_memory_chunks VALUES ('c2','b1','l2','default','MEMORY.md','memory',1,1,'h','m','t','e',1);`,
	}
	for _, stmt := range seed {
		if _, err := db.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed exec failed: %v\nstmt=%s", err, stmt)
		}
	}

	purgeAIMemoryTablesBestEffort(ctx, db, bridgeID, loginID)

	// Verify target login rows are gone.
	checks := []struct {
		table string
		want  int
	}{
		{"ai_memory_files", 0},
		{"ai_memory_chunks", 0},
		{"ai_memory_meta", 0},
		{"ai_memory_session_state", 0},
		{"ai_memory_session_files", 0},
		{"ai_memory_embedding_cache", 0},
	}
	for _, c := range checks {
		var got int
		row := db.QueryRow(ctx, "SELECT COUNT(*) FROM "+c.table+" WHERE bridge_id=$1 AND login_id=$2", bridgeID, loginID)
		if err := row.Scan(&got); err != nil {
			t.Fatalf("count %s: %v", c.table, err)
		}
		if got != c.want {
			t.Fatalf("after purge %s: got %d want %d", c.table, got, c.want)
		}
	}

	// Verify other login rows remain.
	var keep int
	row := db.QueryRow(ctx, "SELECT COUNT(*) FROM ai_memory_chunks WHERE bridge_id=$1 AND login_id=$2", "b1", "l2")
	if err := row.Scan(&keep); err != nil {
		t.Fatalf("count chunks keep: %v", err)
	}
	if keep != 1 {
		t.Fatalf("chunks keep: got %d want 1", keep)
	}
}

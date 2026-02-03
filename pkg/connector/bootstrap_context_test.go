package connector

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func setupBootstrapDB(t *testing.T) *database.Database {
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
	return database.New(networkid.BridgeID("bridge"), database.MetaTypes{}, db)
}

func TestBuildBootstrapContextFiles(t *testing.T) {
	ctx := context.Background()
	db := setupBootstrapDB(t)
	bridge := &bridgev2.Bridge{DB: db}
	login := &database.UserLogin{ID: networkid.UserLoginID("login")}
	userLogin := &bridgev2.UserLogin{UserLogin: login, Bridge: bridge, Log: zerolog.Nop()}
	oc := &AIClient{
		UserLogin: userLogin,
		connector: &OpenAIConnector{Config: Config{}},
		log:       zerolog.Nop(),
	}

	files := oc.buildBootstrapContextFiles(ctx, "beeper")
	if len(files) == 0 {
		t.Fatal("expected bootstrap context files")
	}

	foundSoul := false
	for _, file := range files {
		if strings.EqualFold(file.Path, "SOUL.md") {
			foundSoul = true
			if strings.Contains(file.Content, "[MISSING]") {
				t.Fatalf("expected SOUL.md content, got missing placeholder")
			}
		}
	}
	if !foundSoul {
		t.Fatal("expected SOUL.md to be injected")
	}
}

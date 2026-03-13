package memory

import (
	"context"
	"strings"

	"go.mau.fi/util/dbutil"
)

func PurgeTablesBestEffort(ctx context.Context, db *dbutil.Database, bridgeID, loginID string) {
	if db == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_chunks_fts WHERE bridge_id=$1 AND login_id=$2`,
		bridgeID, loginID,
	)
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_session_files WHERE bridge_id=$1 AND login_id=$2`,
		bridgeID, loginID,
	)
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_session_state WHERE bridge_id=$1 AND login_id=$2`,
		bridgeID, loginID,
	)
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_chunks_vec WHERE id IN (
           SELECT id FROM ai_memory_chunks WHERE bridge_id=$1 AND login_id=$2
         )`,
		bridgeID, loginID,
	)
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_embedding_cache WHERE bridge_id=$1 AND login_id=$2`,
		bridgeID, loginID,
	)
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_chunks WHERE bridge_id=$1 AND login_id=$2`,
		bridgeID, loginID,
	)
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_files WHERE bridge_id=$1 AND login_id=$2`,
		bridgeID, loginID,
	)
	bestEffortExec(ctx, db,
		`DELETE FROM ai_memory_meta WHERE bridge_id=$1 AND login_id=$2`,
		bridgeID, loginID,
	)
}

func bestEffortExec(ctx context.Context, db *dbutil.Database, query string, args ...any) {
	if db == nil {
		return
	}
	_, err := db.Exec(ctx, query, args...)
	if err == nil {
		return
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "undefined table") ||
		strings.Contains(msg, "no such module") {
		return
	}
}

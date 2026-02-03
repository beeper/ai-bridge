package connector

import "context"

func (m *MemorySearchManager) purgeSessionPath(ctx context.Context, path string) {
	if path == "" {
		return
	}
	if m.vectorReady {
		ids := m.collectChunkIDs(ctx, path, "sessions", m.status.Model, "")
		m.deleteVectorIDs(ctx, ids)
	}
	_, _ = m.db.Exec(ctx,
		`DELETE FROM ai_memory_chunks
         WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND path=$4 AND source=$5`,
		m.bridgeID, m.loginID, m.agentID, path, "sessions",
	)
	if m.ftsAvailable {
		_, _ = m.db.Exec(ctx,
			`DELETE FROM ai_memory_chunks_fts
             WHERE bridge_id=$1 AND login_id=$2 AND agent_id=$3 AND path=$4 AND source=$5`,
			m.bridgeID, m.loginID, m.agentID, path, "sessions",
		)
	}
}

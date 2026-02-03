-- v22 -> v23: ai memory tables
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

CREATE TABLE IF NOT EXISTS ai_memory_chunks (
  id TEXT PRIMARY KEY,
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  path TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'memory',
  start_line INTEGER NOT NULL,
  end_line INTEGER NOT NULL,
  hash TEXT NOT NULL,
  model TEXT NOT NULL,
  text TEXT NOT NULL,
  embedding TEXT NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ai_memory_chunks_lookup ON ai_memory_chunks(bridge_id, login_id, agent_id, model, source);
CREATE INDEX IF NOT EXISTS idx_ai_memory_chunks_path ON ai_memory_chunks(path);

CREATE TABLE IF NOT EXISTS ai_memory_meta (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_key TEXT NOT NULL,
  chunk_tokens INTEGER NOT NULL,
  chunk_overlap INTEGER NOT NULL,
  vector_dims INTEGER,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (bridge_id, login_id, agent_id)
);

CREATE TABLE IF NOT EXISTS ai_memory_embedding_cache (
  bridge_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_key TEXT NOT NULL,
  hash TEXT NOT NULL,
  embedding TEXT NOT NULL,
  dims INTEGER,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (bridge_id, provider, model, provider_key, hash)
);

CREATE INDEX IF NOT EXISTS idx_ai_memory_embedding_cache_updated_at ON ai_memory_embedding_cache(updated_at);

CREATE TABLE IF NOT EXISTS ai_memory_session_state (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  session_key TEXT NOT NULL,
  last_rowid INTEGER NOT NULL DEFAULT 0,
  pending_bytes INTEGER NOT NULL DEFAULT 0,
  pending_messages INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (bridge_id, login_id, agent_id, session_key)
);

CREATE TABLE IF NOT EXISTS ai_memory_session_files (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  session_key TEXT NOT NULL,
  path TEXT NOT NULL,
  content TEXT NOT NULL,
  hash TEXT NOT NULL,
  size INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (bridge_id, login_id, agent_id, session_key)
);

CREATE INDEX IF NOT EXISTS idx_ai_memory_session_files_path ON ai_memory_session_files(path);

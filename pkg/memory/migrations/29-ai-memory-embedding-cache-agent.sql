-- v28 -> v29: scope embedding cache to login/agent
DROP TABLE IF EXISTS ai_memory_embedding_cache;

CREATE TABLE IF NOT EXISTS ai_memory_embedding_cache (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  provider_key TEXT NOT NULL,
  hash TEXT NOT NULL,
  embedding TEXT NOT NULL,
  dims INTEGER,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (bridge_id, login_id, agent_id, provider, model, provider_key, hash)
);

CREATE INDEX IF NOT EXISTS idx_ai_memory_embedding_cache_updated_at ON ai_memory_embedding_cache(updated_at);

-- v29 -> v30: dedicated cron state table (replaces __cron__ agent in ai_memory_files)
CREATE TABLE IF NOT EXISTS ai_cron_state (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  store_key TEXT NOT NULL,
  content TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (bridge_id, login_id, store_key)
);

-- v1 -> v2: create scheduler tables
CREATE TABLE IF NOT EXISTS ai_cron_jobs (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  job_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  delete_after_run INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  schedule_json TEXT NOT NULL DEFAULT '{}',
  payload_json TEXT NOT NULL DEFAULT '{}',
  delivery_json TEXT NOT NULL DEFAULT '',
  state_json TEXT NOT NULL DEFAULT '{}',
  room_id TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 1,
  pending_delay_id TEXT NOT NULL DEFAULT '',
  pending_delay_kind TEXT NOT NULL DEFAULT '',
  pending_run_key TEXT NOT NULL DEFAULT '',
  last_output_preview TEXT NOT NULL DEFAULT '',
  processed_run_keys_json TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY (bridge_id, login_id, job_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_cron_jobs_lookup ON ai_cron_jobs(bridge_id, login_id, agent_id);

CREATE TABLE IF NOT EXISTS ai_managed_heartbeats (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  interval_ms INTEGER NOT NULL DEFAULT 0,
  active_hours_json TEXT NOT NULL DEFAULT '',
  room_id TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 1,
  next_run_at_ms INTEGER,
  pending_delay_id TEXT NOT NULL DEFAULT '',
  pending_delay_kind TEXT NOT NULL DEFAULT '',
  pending_run_key TEXT NOT NULL DEFAULT '',
  last_run_at_ms INTEGER,
  last_result TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  processed_run_keys_json TEXT NOT NULL DEFAULT '[]',
  PRIMARY KEY (bridge_id, login_id, agent_id)
);

DELETE FROM ai_bridge_state
WHERE store_key IN ('cron/jobs.v2.json', 'heartbeat/managed.v1.json');

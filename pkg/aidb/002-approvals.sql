-- v1 -> v2: add centralized approval storage
CREATE TABLE IF NOT EXISTS ai_approvals (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  approval_id TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT '',
  room_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  tool_name TEXT NOT NULL DEFAULT '',
  request_json TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  expires_at_ms INTEGER NOT NULL DEFAULT 0,
  created_at_ms INTEGER NOT NULL DEFAULT 0,
  updated_at_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (bridge_id, login_id, agent_id, approval_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_approvals_lookup
  ON ai_approvals(bridge_id, login_id, agent_id, status, expires_at_ms);

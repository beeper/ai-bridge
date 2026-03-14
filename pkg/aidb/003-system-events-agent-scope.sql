-- v2 -> v3: scope system event storage by agent
CREATE TABLE IF NOT EXISTS ai_system_events_v3 (
  bridge_id TEXT NOT NULL,
  login_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT 'beep',
  session_key TEXT NOT NULL,
  event_index INTEGER NOT NULL,
  text TEXT NOT NULL DEFAULT '',
  ts INTEGER NOT NULL DEFAULT 0,
  last_text TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (bridge_id, login_id, agent_id, session_key, event_index)
);

INSERT INTO ai_system_events_v3 (
  bridge_id, login_id, agent_id, session_key, event_index, text, ts, last_text
)
SELECT bridge_id, login_id, 'beep', session_key, event_index, text, ts, last_text
FROM ai_system_events;

DROP TABLE ai_system_events;

ALTER TABLE ai_system_events_v3 RENAME TO ai_system_events;

package connector

import (
	"context"
	"database/sql"
	"encoding/json"

	"go.mau.fi/util/dbutil"

	integrationcron "github.com/beeper/ai-bridge/pkg/integrations/cron"
)

type schedulerDBScope struct {
	db       *dbutil.Database
	bridgeID string
	loginID  string
}

func (s *schedulerRuntime) schedulerDBScope() *schedulerDBScope {
	if s == nil || s.client == nil || s.client.UserLogin == nil || s.client.UserLogin.Bridge == nil || s.client.UserLogin.Bridge.DB == nil {
		return nil
	}
	db := s.client.bridgeDB()
	if db == nil {
		return nil
	}
	return &schedulerDBScope{
		db:       db,
		bridgeID: string(s.client.UserLogin.Bridge.DB.BridgeID),
		loginID:  string(s.client.UserLogin.ID),
	}
}

func (s *schedulerRuntime) loadCronStoreLocked(ctx context.Context) (scheduledCronStore, error) {
	scope := s.schedulerDBScope()
	if scope == nil {
		return scheduledCronStore{}, nil
	}
	rows, err := scope.db.Query(ctx, `
		SELECT
			job_id, agent_id, name, description, enabled, delete_after_run,
			created_at_ms, updated_at_ms, schedule_json, payload_json,
			delivery_json, state_json, room_id, revision, pending_delay_id,
			pending_delay_kind, pending_run_key, last_output_preview, processed_run_keys_json
		FROM ai_cron_jobs
		WHERE bridge_id=$1 AND login_id=$2
		ORDER BY job_id
	`, scope.bridgeID, scope.loginID)
	if err != nil {
		return scheduledCronStore{}, err
	}
	defer rows.Close()

	store := scheduledCronStore{}
	for rows.Next() {
		var (
			record            scheduledCronJob
			enabled           bool
			deleteAfterRun    bool
			scheduleJSON      string
			payloadJSON       string
			deliveryJSON      string
			stateJSON         string
			processedKeysJSON string
		)
		if err := rows.Scan(
			&record.Job.ID,
			&record.Job.AgentID,
			&record.Job.Name,
			&record.Job.Description,
			&enabled,
			&deleteAfterRun,
			&record.Job.CreatedAtMs,
			&record.Job.UpdatedAtMs,
			&scheduleJSON,
			&payloadJSON,
			&deliveryJSON,
			&stateJSON,
			&record.RoomID,
			&record.Revision,
			&record.PendingDelayID,
			&record.PendingDelayKind,
			&record.PendingRunKey,
			&record.LastOutputPreview,
			&processedKeysJSON,
		); err != nil {
			return scheduledCronStore{}, err
		}
		record.Job.Enabled = enabled
		record.Job.DeleteAfterRun = deleteAfterRun
		if err := schedulerUnmarshalJSON(scheduleJSON, &record.Job.Schedule); err != nil {
			return scheduledCronStore{}, err
		}
		if err := schedulerUnmarshalJSON(payloadJSON, &record.Job.Payload); err != nil {
			return scheduledCronStore{}, err
		}
		if deliveryJSON != "" {
			record.Job.Delivery = &integrationcron.Delivery{}
			if err := schedulerUnmarshalJSON(deliveryJSON, record.Job.Delivery); err != nil {
				return scheduledCronStore{}, err
			}
		}
		if err := schedulerUnmarshalJSON(stateJSON, &record.Job.State); err != nil {
			return scheduledCronStore{}, err
		}
		if err := schedulerUnmarshalJSON(processedKeysJSON, &record.ProcessedRunKeys); err != nil {
			return scheduledCronStore{}, err
		}
		store.Jobs = append(store.Jobs, record)
	}
	if err := rows.Err(); err != nil {
		return scheduledCronStore{}, err
	}
	return store, nil
}

func (s *schedulerRuntime) saveCronStoreLocked(ctx context.Context, store scheduledCronStore) error {
	scope := s.schedulerDBScope()
	if scope == nil {
		return nil
	}
	return scope.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		if _, err := scope.db.Exec(ctx, `DELETE FROM ai_cron_jobs WHERE bridge_id=$1 AND login_id=$2`, scope.bridgeID, scope.loginID); err != nil {
			return err
		}
		for _, record := range store.Jobs {
			scheduleJSON, err := schedulerMarshalJSON(record.Job.Schedule)
			if err != nil {
				return err
			}
			payloadJSON, err := schedulerMarshalJSON(record.Job.Payload)
			if err != nil {
				return err
			}
			deliveryJSON, err := schedulerMarshalNullableJSON(record.Job.Delivery)
			if err != nil {
				return err
			}
			stateJSON, err := schedulerMarshalJSON(record.Job.State)
			if err != nil {
				return err
			}
			processedJSON, err := schedulerMarshalJSON(record.ProcessedRunKeys)
			if err != nil {
				return err
			}
			if _, err := scope.db.Exec(ctx, `
				INSERT INTO ai_cron_jobs (
					bridge_id, login_id, job_id, agent_id, name, description,
					enabled, delete_after_run, created_at_ms, updated_at_ms,
					schedule_json, payload_json, delivery_json, state_json,
					room_id, revision, pending_delay_id, pending_delay_kind,
					pending_run_key, last_output_preview, processed_run_keys_json
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
			`,
				scope.bridgeID, scope.loginID, record.Job.ID, record.Job.AgentID, record.Job.Name, record.Job.Description,
				record.Job.Enabled, record.Job.DeleteAfterRun, record.Job.CreatedAtMs, record.Job.UpdatedAtMs,
				scheduleJSON, payloadJSON, deliveryJSON, stateJSON,
				record.RoomID, record.Revision, record.PendingDelayID, record.PendingDelayKind,
				record.PendingRunKey, record.LastOutputPreview, processedJSON,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *schedulerRuntime) loadHeartbeatStoreLocked(ctx context.Context) (managedHeartbeatStore, error) {
	scope := s.schedulerDBScope()
	if scope == nil {
		return managedHeartbeatStore{}, nil
	}
	rows, err := scope.db.Query(ctx, `
		SELECT
			agent_id, enabled, interval_ms, active_hours_json, room_id, revision,
			next_run_at_ms, pending_delay_id, pending_delay_kind, pending_run_key,
			last_run_at_ms, last_result, last_error, processed_run_keys_json
		FROM ai_managed_heartbeats
		WHERE bridge_id=$1 AND login_id=$2
		ORDER BY agent_id
	`, scope.bridgeID, scope.loginID)
	if err != nil {
		return managedHeartbeatStore{}, err
	}
	defer rows.Close()

	store := managedHeartbeatStore{}
	for rows.Next() {
		var (
			state             managedHeartbeatState
			enabled           bool
			activeHoursJSON   string
			nextRunAtMs       sql.NullInt64
			lastRunAtMs       sql.NullInt64
			processedKeysJSON string
		)
		if err := rows.Scan(
			&state.AgentID,
			&enabled,
			&state.IntervalMs,
			&activeHoursJSON,
			&state.RoomID,
			&state.Revision,
			&nextRunAtMs,
			&state.PendingDelayID,
			&state.PendingDelayKind,
			&state.PendingRunKey,
			&lastRunAtMs,
			&state.LastResult,
			&state.LastError,
			&processedKeysJSON,
		); err != nil {
			return managedHeartbeatStore{}, err
		}
		state.Enabled = enabled
		state.NextRunAtMs = nextRunAtMs.Int64
		state.LastRunAtMs = lastRunAtMs.Int64
		if activeHoursJSON != "" {
			state.ActiveHours = &HeartbeatActiveHoursConfig{}
			if err := schedulerUnmarshalJSON(activeHoursJSON, state.ActiveHours); err != nil {
				return managedHeartbeatStore{}, err
			}
		}
		if err := schedulerUnmarshalJSON(processedKeysJSON, &state.ProcessedRunKeys); err != nil {
			return managedHeartbeatStore{}, err
		}
		store.Agents = append(store.Agents, state)
	}
	if err := rows.Err(); err != nil {
		return managedHeartbeatStore{}, err
	}
	return store, nil
}

func (s *schedulerRuntime) saveHeartbeatStoreLocked(ctx context.Context, store managedHeartbeatStore) error {
	scope := s.schedulerDBScope()
	if scope == nil {
		return nil
	}
	return scope.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		if _, err := scope.db.Exec(ctx, `DELETE FROM ai_managed_heartbeats WHERE bridge_id=$1 AND login_id=$2`, scope.bridgeID, scope.loginID); err != nil {
			return err
		}
		for _, state := range store.Agents {
			activeHoursJSON, err := schedulerMarshalNullableJSON(state.ActiveHours)
			if err != nil {
				return err
			}
			processedJSON, err := schedulerMarshalJSON(state.ProcessedRunKeys)
			if err != nil {
				return err
			}
			if _, err := scope.db.Exec(ctx, `
				INSERT INTO ai_managed_heartbeats (
					bridge_id, login_id, agent_id, enabled, interval_ms, active_hours_json,
					room_id, revision, next_run_at_ms, pending_delay_id, pending_delay_kind,
					pending_run_key, last_run_at_ms, last_result, last_error, processed_run_keys_json
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, 0), $10, $11, $12, NULLIF($13, 0), $14, $15, $16)
			`,
				scope.bridgeID, scope.loginID, state.AgentID, state.Enabled, state.IntervalMs, activeHoursJSON,
				state.RoomID, state.Revision, state.NextRunAtMs, state.PendingDelayID, state.PendingDelayKind,
				state.PendingRunKey, state.LastRunAtMs, state.LastResult, state.LastError, processedJSON,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func schedulerMarshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func schedulerMarshalNullableJSON(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	return schedulerMarshalJSON(value)
}

func schedulerUnmarshalJSON(raw string, target any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}

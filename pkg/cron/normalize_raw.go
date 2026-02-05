package cron

import (
	"encoding/json"
	"fmt"
	"strings"
)

type rawRecord map[string]any

func normalizeCronJobInputRaw(raw any, applyDefaults bool) rawRecord {
	base, ok := unwrapCronJob(raw)
	if !ok {
		return nil
	}
	next := rawRecord{}
	for k, v := range base {
		next[k] = v
	}

	// agentId handling (trim, allow null to clear)
	if val, ok := base["agentId"]; ok {
		switch v := val.(type) {
		case nil:
			next["agentId"] = nil
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				delete(next, "agentId")
			} else {
				next["agentId"] = sanitizeAgentID(trimmed)
			}
		}
	}

	// enabled coercion
	if val, ok := base["enabled"]; ok {
		switch v := val.(type) {
		case bool:
			next["enabled"] = v
		case string:
			trimmed := strings.ToLower(strings.TrimSpace(v))
			if trimmed == "true" {
				next["enabled"] = true
			} else if trimmed == "false" {
				next["enabled"] = false
			}
		}
	}

	// schedule coercion
	if schedRaw, ok := base["schedule"]; ok {
		if schedMap, ok := schedRaw.(map[string]any); ok {
			next["schedule"] = coerceScheduleMap(schedMap)
		}
	}

	if applyDefaults {
		if _, ok := next["wakeMode"]; !ok {
			next["wakeMode"] = string(CronWakeNextHeartbeat)
		}
		if _, ok := next["sessionTarget"]; !ok {
			if payloadMap, ok := next["payload"].(map[string]any); ok {
				if kind, ok := payloadMap["kind"].(string); ok {
					switch strings.ToLower(strings.TrimSpace(kind)) {
					case "systemevent":
						next["sessionTarget"] = string(CronSessionMain)
					case "agentturn":
						next["sessionTarget"] = string(CronSessionIsolated)
					}
				}
			}
		}
	}

	return next
}

func unwrapCronJob(raw any) (rawRecord, bool) {
	base, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	if job, ok := base["job"].(map[string]any); ok {
		return job, true
	}
	return base, true
}

func coerceScheduleMap(schedule map[string]any) map[string]any {
	next := map[string]any{}
	for k, v := range schedule {
		next[k] = v
	}
	kind, _ := schedule["kind"].(string)
	if strings.TrimSpace(kind) == "" {
		if schedule["at"] != nil {
			next["kind"] = "at"
		} else if schedule["everyMs"] != nil {
			next["kind"] = "every"
		} else if schedule["expr"] != nil {
			next["kind"] = "cron"
		}
	}
	return next
}

// NormalizeCronJobCreateRaw normalizes raw input into a CronJobCreate.
func NormalizeCronJobCreateRaw(raw any) (CronJobCreate, error) {
	normalized := normalizeCronJobInputRaw(raw, true)
	if normalized == nil {
		return CronJobCreate{}, fmt.Errorf("invalid cron job")
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return CronJobCreate{}, err
	}
	var out CronJobCreate
	if err := json.Unmarshal(data, &out); err != nil {
		return CronJobCreate{}, err
	}
	return NormalizeCronJobCreate(out), nil
}

// NormalizeCronJobPatchRaw normalizes raw input into a CronJobPatch.
func NormalizeCronJobPatchRaw(raw any) (CronJobPatch, error) {
	normalized := normalizeCronJobInputRaw(raw, false)
	if normalized == nil {
		return CronJobPatch{}, fmt.Errorf("invalid cron patch")
	}
	agentIDPresent := false
	agentIDNil := false
	if val, ok := normalized["agentId"]; ok {
		agentIDPresent = true
		agentIDNil = val == nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return CronJobPatch{}, err
	}
	var out CronJobPatch
	if err := json.Unmarshal(data, &out); err != nil {
		return CronJobPatch{}, err
	}
	if agentIDPresent && agentIDNil && out.AgentID == nil {
		empty := ""
		out.AgentID = &empty
	}
	return NormalizeCronJobPatch(out), nil
}

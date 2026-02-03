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
			next["agentId"] = ""
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

	// payload coercion (legacy provider -> channel)
	if payloadRaw, ok := base["payload"]; ok {
		if payloadMap, ok := payloadRaw.(map[string]any); ok {
			next["payload"] = coercePayloadMap(payloadMap)
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
	if data, ok := base["data"].(map[string]any); ok {
		return data, true
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
	if val, ok := schedule["every"]; ok {
		if ms, ok := coerceEveryMs(val); ok {
			next["everyMs"] = ms
		}
		if _, exists := next["everyMs"]; exists {
			delete(next, "every")
		}
	}
	if val, ok := schedule["everyMs"]; ok {
		if ms, ok := coerceEveryMsExact(val); ok {
			next["everyMs"] = ms
		}
	}
	kind, _ := schedule["kind"].(string)
	if strings.TrimSpace(kind) == "" {
		if schedule["atMs"] != nil || schedule["at"] != nil {
			next["kind"] = "at"
		} else if schedule["everyMs"] != nil {
			next["kind"] = "every"
		} else if schedule["expr"] != nil {
			next["kind"] = "cron"
		}
	}

	var atRaw string
	if val, ok := schedule["atMs"].(string); ok {
		atRaw = val
	} else if val, ok := schedule["at"].(string); ok {
		atRaw = val
	}
	if atRaw != "" {
		if parsed, ok := parseAbsoluteTimeMs(atRaw); ok {
			next["atMs"] = parsed
		}
	}
	if _, ok := next["at"]; ok {
		delete(next, "at")
	}
	if val, ok := schedule["anchorMs"]; ok {
		if parsed, ok := coerceAbsoluteMs(val); ok {
			next["anchorMs"] = parsed
		}
	}
	if val, ok := schedule["anchor"]; ok {
		if parsed, ok := coerceAbsoluteMs(val); ok {
			next["anchorMs"] = parsed
		}
		delete(next, "anchor")
	}
	return next
}

func coerceEveryMs(val any) (int64, bool) {
	switch v := val.(type) {
	case float64:
		if v <= 0 {
			return 0, false
		}
		return int64(v * 60_000), true
	case float32:
		if v <= 0 {
			return 0, false
		}
		return int64(float64(v) * 60_000), true
	case int:
		if v <= 0 {
			return 0, false
		}
		return int64(v) * 60_000, true
	case int64:
		if v <= 0 {
			return 0, false
		}
		return v * 60_000, true
	case int32:
		if v <= 0 {
			return 0, false
		}
		return int64(v) * 60_000, true
	case string:
		ms, err := parseDurationMs(v, "m")
		if err != nil || ms <= 0 {
			return 0, false
		}
		return ms, true
	default:
		return 0, false
	}
}

func coerceEveryMsExact(val any) (int64, bool) {
	switch v := val.(type) {
	case float64:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case float32:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case int:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case int64:
		if v <= 0 {
			return 0, false
		}
		return v, true
	case int32:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case string:
		ms, err := parseDurationMs(v, "ms")
		if err != nil || ms <= 0 {
			return 0, false
		}
		return ms, true
	default:
		return 0, false
	}
}

func coerceAbsoluteMs(val any) (int64, bool) {
	switch v := val.(type) {
	case float64:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case float32:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case int:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case int64:
		if v <= 0 {
			return 0, false
		}
		return v, true
	case int32:
		if v <= 0 {
			return 0, false
		}
		return int64(v), true
	case string:
		ms, ok := parseAbsoluteTimeMs(v)
		if !ok || ms <= 0 {
			return 0, false
		}
		return ms, true
	default:
		return 0, false
	}
}

func coercePayloadMap(payload map[string]any) map[string]any {
	next := map[string]any{}
	for k, v := range payload {
		next[k] = v
	}
	channel := ""
	if raw, ok := payload["channel"].(string); ok && strings.TrimSpace(raw) != "" {
		channel = strings.ToLower(strings.TrimSpace(raw))
	} else if raw, ok := payload["provider"].(string); ok && strings.TrimSpace(raw) != "" {
		channel = strings.ToLower(strings.TrimSpace(raw))
	}
	if channel != "" {
		next["channel"] = channel
	}
	if _, ok := next["provider"]; ok {
		delete(next, "provider")
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
	data, err := json.Marshal(normalized)
	if err != nil {
		return CronJobPatch{}, err
	}
	var out CronJobPatch
	if err := json.Unmarshal(data, &out); err != nil {
		return CronJobPatch{}, err
	}
	return NormalizeCronJobPatch(out), nil
}

package connector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/agents"
	agenttools "github.com/beeper/ai-bridge/pkg/agents/tools"
	"github.com/beeper/ai-bridge/pkg/cron"
)

func executeCron(ctx context.Context, args map[string]any) (string, error) {
	btc := GetBridgeToolContext(ctx)
	if btc == nil || btc.Client == nil {
		return "", fmt.Errorf("cron tool requires bridge context")
	}
	client := btc.Client
	if client.cronService == nil {
		return "", fmt.Errorf("cron service not available")
	}

	action := strings.ToLower(strings.TrimSpace(agenttools.ReadStringDefault(args, "action", "")))
	if action == "" {
		return agenttools.JSONResult(map[string]any{
			"status": "error",
			"error":  "action is required",
		}).Text(), nil
	}

	switch action {
	case "status":
		enabled, storePath, jobCount, nextWake, err := client.cronService.Status()
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		out := map[string]any{
			"enabled":   enabled,
			"storePath": storePath,
			"jobCount":  jobCount,
		}
		if nextWake != nil {
			out["nextWakeAtMs"] = *nextWake
		}
		return agenttools.JSONResult(out).Text(), nil
	case "list":
		includeDisabled := agenttools.ReadBool(args, "includeDisabled", false)
		jobs, err := client.cronService.List(includeDisabled)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		return agenttools.JSONResult(map[string]any{
			"jobs":  jobs,
			"count": len(jobs),
		}).Text(), nil
	case "add":
		normalizedArgs := coerceCronArgs(args)
		jobInput, err := cron.NormalizeCronJobCreateRaw(normalizedArgs)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		injectCronContext(&jobInput, btc)
		if strings.TrimSpace(jobInput.Payload.Kind) == "" {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  "payload.kind is required",
			}).Text(), nil
		}
		job, err := client.cronService.Add(jobInput)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		return agenttools.JSONResult(map[string]any{
			"job": job,
		}).Text(), nil
	case "update":
		jobID := readCronJobID(args)
		if jobID == "" {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  "id is required",
			}).Text(), nil
		}
		rawPatch := selectCronPatch(args)
		patch, err := cron.NormalizeCronJobPatchRaw(rawPatch)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		job, err := client.cronService.Update(jobID, patch)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		return agenttools.JSONResult(map[string]any{
			"job": job,
		}).Text(), nil
	case "remove":
		jobID := readCronJobID(args)
		if jobID == "" {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  "id is required",
			}).Text(), nil
		}
		removed, err := client.cronService.Remove(jobID)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		return agenttools.JSONResult(map[string]any{
			"removed": removed,
		}).Text(), nil
	case "run":
		jobID := readCronJobID(args)
		if jobID == "" {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  "id is required",
			}).Text(), nil
		}
		mode := strings.ToLower(strings.TrimSpace(agenttools.ReadStringDefault(args, "mode", "")))
		if mode == "" && agenttools.ReadBool(args, "force", false) {
			mode = "force"
		}
		ran, reason, err := client.cronService.Run(jobID, mode)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		out := map[string]any{
			"ran": ran,
		}
		if reason != "" {
			out["reason"] = reason
		}
		return agenttools.JSONResult(out).Text(), nil
	case "runs":
		jobID := readCronJobID(args)
		limit := agenttools.ReadIntDefault(args, "limit", 200)
		runs, err := client.readCronRuns(jobID, limit)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		out := map[string]any{
			"runs":  runs,
			"count": len(runs),
		}
		if jobID != "" {
			out["jobId"] = jobID
		}
		return agenttools.JSONResult(out).Text(), nil
	case "wake":
		text := strings.TrimSpace(firstNonEmptyString(args["text"], args["message"]))
		if text == "" {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  "text is required",
			}).Text(), nil
		}
		mode := strings.ToLower(strings.TrimSpace(agenttools.ReadStringDefault(args, "mode", "")))
		if mode == "" {
			mode = "next-heartbeat"
		}
		enqueued, err := client.cronService.Wake(mode, text)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		return agenttools.JSONResult(map[string]any{
			"enqueued": enqueued,
		}).Text(), nil
	default:
		return agenttools.JSONResult(map[string]any{
			"status": "error",
			"error":  fmt.Sprintf("unknown action: %s", action),
		}).Text(), nil
	}
}

func readCronJobID(args map[string]any) string {
	if args == nil {
		return ""
	}
	if val := strings.TrimSpace(agenttools.ReadStringDefault(args, "id", "")); val != "" {
		return val
	}
	if val := strings.TrimSpace(agenttools.ReadStringDefault(args, "jobId", "")); val != "" {
		return val
	}
	if val := strings.TrimSpace(agenttools.ReadStringDefault(args, "job_id", "")); val != "" {
		return val
	}
	return ""
}

func selectCronPatch(args map[string]any) any {
	if args == nil {
		return args
	}
	if raw, ok := args["patch"]; ok {
		if _, ok := raw.(map[string]any); ok {
			return raw
		}
	}
	return coerceCronArgs(args)
}

func coerceCronArgs(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	clone := map[string]any{}
	for k, v := range args {
		clone[k] = v
	}
	schedule := extractScheduleFields(clone)
	if len(schedule) > 0 {
		if raw, ok := clone["job"].(map[string]any); ok {
			jobCopy := map[string]any{}
			for k, v := range raw {
				jobCopy[k] = v
			}
			if _, ok := jobCopy["schedule"]; !ok {
				jobCopy["schedule"] = schedule
			}
			clone["job"] = jobCopy
		} else if raw, ok := clone["data"].(map[string]any); ok {
			dataCopy := map[string]any{}
			for k, v := range raw {
				dataCopy[k] = v
			}
			if _, ok := dataCopy["schedule"]; !ok {
				dataCopy["schedule"] = schedule
			}
			clone["data"] = dataCopy
		} else if _, ok := clone["schedule"]; !ok {
			clone["schedule"] = schedule
		}
	}
	return clone
}

func extractScheduleFields(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	schedule := map[string]any{}
	for _, key := range []string{"kind", "at", "atMs", "every", "everyMs", "anchor", "anchorMs", "expr", "tz"} {
		if val, ok := args[key]; ok {
			schedule[key] = val
		}
	}
	if len(schedule) == 0 {
		return nil
	}
	return schedule
}

func injectCronContext(job *cron.CronJobCreate, btc *BridgeToolContext) {
	if job == nil || btc == nil {
		return
	}
	meta := btc.Meta
	if meta == nil && btc.Portal != nil {
		meta = portalMeta(btc.Portal)
	}
	if job.AgentID == nil || strings.TrimSpace(*job.AgentID) == "" {
		agentID := resolveAgentID(meta)
		if strings.TrimSpace(agentID) == "" {
			agentID = agents.DefaultAgentID
		}
		job.AgentID = &agentID
	}
	if strings.TrimSpace(job.Payload.Channel) == "" {
		job.Payload.Channel = "matrix"
	}
	if strings.TrimSpace(job.Payload.To) == "" && btc.Portal != nil && btc.Portal.MXID != "" {
		job.Payload.To = btc.Portal.MXID.String()
	}
}

func (oc *AIClient) readCronRuns(jobID string, limit int) ([]cron.CronRunLogEntry, error) {
	if oc == nil || oc.cronService == nil {
		return nil, fmt.Errorf("cron service not available")
	}
	if limit <= 0 {
		limit = 200
	}
	_, storePath, _, _, err := oc.cronService.Status()
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(jobID)
	if trimmed != "" {
		path := cron.ResolveCronRunLogPath(storePath, trimmed)
		return cron.ReadCronRunLogEntries(path, limit, trimmed)
	}
	runDir := filepath.Join(filepath.Dir(storePath), "runs")
	entries := make([]cron.CronRunLogEntry, 0)
	files, err := os.ReadDir(runDir)
	if err != nil {
		return entries, nil
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(file.Name()), ".jsonl") {
			continue
		}
		path := filepath.Join(runDir, file.Name())
		list, _ := cron.ReadCronRunLogEntries(path, limit, "")
		if len(list) > 0 {
			entries = append(entries, list...)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TS < entries[j].TS
	})
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

func cronRunLogEntryFromEvent(evt cron.CronEvent) cron.CronRunLogEntry {
	return cron.CronRunLogEntry{
		TS:          time.Now().UnixMilli(),
		JobID:       evt.JobID,
		Action:      evt.Action,
		Status:      evt.Status,
		Error:       evt.Error,
		Summary:     evt.Summary,
		RunAtMs:     evt.RunAtMs,
		DurationMs:  evt.DurationMs,
		NextRunAtMs: evt.NextRunAtMs,
	}
}

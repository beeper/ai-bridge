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
			"jobs":      jobCount,
		}
		if nextWake != nil {
			out["nextWakeAtMs"] = *nextWake
		} else {
			out["nextWakeAtMs"] = nil
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
			"jobs": jobs,
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
		contextMessages := agenttools.ReadIntDefault(args, "contextMessages", 0)
		if contextMessages > 0 {
			injectCronReminderContext(&jobInput, btc, contextMessages)
		}
		job, err := client.cronService.Add(jobInput)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		return agenttools.JSONResult(job).Text(), nil
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
		return agenttools.JSONResult(job).Text(), nil
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
			"ok":      true,
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
			"ok":  true,
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
			"entries": runs,
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
		_, err := client.cronService.Wake(mode, text)
		if err != nil {
			return agenttools.JSONResult(map[string]any{
				"status": "error",
				"error":  err.Error(),
			}).Text(), nil
		}
		return agenttools.JSONResult(map[string]any{
			"ok": true,
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
}

const (
	reminderContextMessagesMax   = 10
	reminderContextPerMessageMax = 220
	reminderContextTotalMax      = 700
	reminderContextMarker        = "\n\nRecent context:\n"
)

func injectCronReminderContext(job *cron.CronJobCreate, btc *BridgeToolContext, count int) {
	if job == nil || btc == nil || btc.Client == nil {
		return
	}
	if strings.ToLower(strings.TrimSpace(job.Payload.Kind)) != "systemevent" {
		return
	}
	text := strings.TrimSpace(job.Payload.Text)
	if text == "" {
		return
	}
	contextLines := buildReminderContextLines(btc, count)
	if len(contextLines) == 0 {
		return
	}
	baseText := stripExistingReminderContext(text)
	job.Payload.Text = baseText + reminderContextMarker + strings.Join(contextLines, "\n")
}

func stripExistingReminderContext(text string) string {
	idx := strings.Index(text, reminderContextMarker)
	if idx == -1 {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(text[:idx])
}

func buildReminderContextLines(btc *BridgeToolContext, count int) []string {
	if btc == nil || btc.Client == nil || btc.Portal == nil {
		return nil
	}
	maxMessages := count
	if maxMessages <= 0 {
		return nil
	}
	if maxMessages > reminderContextMessagesMax {
		maxMessages = reminderContextMessagesMax
	}
	ctx := btc.Client.backgroundContext(context.Background())
	history, err := btc.Client.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, btc.Portal.PortalKey, maxMessages)
	if err != nil || len(history) == 0 {
		return nil
	}
	entries := make([]contextEntry, 0, len(history))
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		meta := messageMeta(msg)
		if !shouldIncludeInHistory(meta) {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(meta.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		text := normalizeContextText(meta.Body)
		if text == "" {
			continue
		}
		entries = append(entries, contextEntry{role: role, text: text})
	}
	if len(entries) == 0 {
		return nil
	}
	if len(entries) > maxMessages {
		entries = entries[len(entries)-maxMessages:]
	}
	lines := make([]string, 0, len(entries))
	total := 0
	for _, entry := range entries {
		label := "User"
		if entry.role == "assistant" {
			label = "Assistant"
		}
		text := truncateContextText(entry.text, reminderContextPerMessageMax)
		line := fmt.Sprintf("- %s: %s", label, text)
		total += len(line)
		if total > reminderContextTotalMax {
			break
		}
		lines = append(lines, line)
	}
	return lines
}

type contextEntry struct {
	role string
	text string
}

func normalizeContextText(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

func truncateContextText(input string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= maxLen {
		return input
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	truncated := strings.TrimRight(string(runes[:maxLen-3]), " ")
	return truncated + "..."
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

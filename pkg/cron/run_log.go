package cron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// CronRunLogEntry mirrors OpenClaw's log format.
type CronRunLogEntry struct {
	TS        int64  `json:"ts"`
	JobID     string `json:"jobId"`
	Action    string `json:"action"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
	Summary   string `json:"summary,omitempty"`
	RunAtMs   int64  `json:"runAtMs,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`
	NextRunAtMs int64 `json:"nextRunAtMs,omitempty"`
}

// ResolveCronRunLogPath returns runs/<jobId>.jsonl next to store.
func ResolveCronRunLogPath(storePath, jobID string) string {
	storeDir := filepath.Dir(filepath.Clean(storePath))
	return filepath.Join(storeDir, "runs", jobID+".jsonl")
}

// AppendCronRunLog appends a log entry and prunes if too large.
func AppendCronRunLog(path string, entry CronRunLogEntry, maxBytes int64, keepLines int) error {
	if maxBytes <= 0 {
		maxBytes = 2_000_000
	}
	if keepLines <= 0 {
		keepLines = 2000
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(payload, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	_ = f.Close()
	return pruneCronLog(path, maxBytes, keepLines)
}

func pruneCronLog(path string, maxBytes int64, keepLines int) error {
	stat, err := os.Stat(path)
	if err != nil || stat.Size() <= maxBytes {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := make([]string, 0)
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := string(data[start:i])
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(data) {
		line := string(data[start:])
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) <= keepLines {
		return nil
	}
	lines = lines[len(lines)-keepLines:]
	tmp := path + ".tmp"
	payload := []byte{}
	for _, line := range lines {
		payload = append(payload, []byte(line)...)
		payload = append(payload, '\n')
	}
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return nil
	}
	_ = os.Rename(tmp, path)
	return nil
}

// ReadCronRunLogEntries reads recent entries from a jsonl log.
func ReadCronRunLogEntries(path string, limit int, jobID string) ([]CronRunLogEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 5000 {
		limit = 5000
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return []CronRunLogEntry{}, nil
	}
	lines := make([]string, 0)
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := string(data[start:i])
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(data) {
		line := string(data[start:])
		if line != "" {
			lines = append(lines, line)
		}
	}
	entries := make([]CronRunLogEntry, 0)
	for i := len(lines) - 1; i >= 0 && len(entries) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry CronRunLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Action != "finished" || entry.JobID == "" || entry.TS == 0 {
			continue
		}
		if jobID != "" && entry.JobID != jobID {
			continue
		}
		entries = append(entries, entry)
	}
	// reverse
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

package cron

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/beeper/ai-bridge/pkg/textfs"
	json5 "github.com/yosuke-furukawa/json5/encoding/json5"
)

const (
	defaultCronDir       = "cron"
	defaultCronFileName  = "jobs.json"
	defaultCronStorePath = defaultCronDir + "/" + defaultCronFileName
)

// ResolveCronStorePath resolves the virtual JSON store path.
func ResolveCronStorePath(storePath string) string {
	trimmed := strings.TrimSpace(storePath)
	if trimmed != "" {
		if normalized, err := textfs.NormalizePath(trimmed); err == nil {
			return normalized
		}
	}
	override := strings.TrimSpace(os.Getenv("OPENCLAW_STATE_DIR"))
	if override == "" {
		override = strings.TrimSpace(os.Getenv("CLAWDBOT_STATE_DIR"))
	}
	if override != "" {
		if dir, err := textfs.NormalizeDir(override); err == nil {
			if dir == "" {
				return defaultCronStorePath
			}
			return path.Join(dir, defaultCronDir, defaultCronFileName)
		}
	}
	return defaultCronStorePath
}

// LoadCronStore reads the JSON store, tolerating missing or invalid files.
func LoadCronStore(ctx context.Context, backend StoreBackend, storePath string) (CronStoreFile, error) {
	if backend == nil {
		return CronStoreFile{Version: 1, Jobs: []CronJob{}}, fmt.Errorf("cron store backend not configured")
	}
	data, found, err := backend.Read(ctx, storePath)
	if err != nil || !found {
		return CronStoreFile{Version: 1, Jobs: []CronJob{}}, nil
	}
	var parsed CronStoreFile
	if err := json5.Unmarshal(data, &parsed); err != nil {
		return CronStoreFile{Version: 1, Jobs: []CronJob{}}, nil
	}
	if parsed.Version == 0 {
		parsed.Version = 1
	}
	if parsed.Jobs == nil {
		parsed.Jobs = []CronJob{}
	}
	return parsed, nil
}

// SaveCronStore writes the JSON store and keeps a .bak copy in virtual storage.
func SaveCronStore(ctx context.Context, backend StoreBackend, storePath string, store CronStoreFile) error {
	if backend == nil {
		return fmt.Errorf("cron store backend not configured")
	}
	if store.Version == 0 {
		store.Version = 1
	}
	payload, err := json5.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := backend.Write(ctx, storePath, payload); err != nil {
		return err
	}
	_ = backend.Write(ctx, storePath+".bak", payload)
	return nil
}

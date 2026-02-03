package cron

import (
	"os"
	"path/filepath"
	"strings"

	json5 "github.com/yosuke-furukawa/json5/encoding/json5"
)

const (
	defaultCronDirName  = ".openclaw/cron"
	defaultCronFileName = "jobs.json"
)

// ResolveCronStorePath resolves the JSON store path.
func ResolveCronStorePath(storePath string) string {
	trimmed := strings.TrimSpace(storePath)
	if trimmed != "" {
		if strings.HasPrefix(trimmed, "~") {
			if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
				return filepath.Join(home, strings.TrimPrefix(trimmed, "~"))
			}
		}
		return filepath.Clean(trimmed)
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(os.TempDir(), "openclaw", "cron", defaultCronFileName)
	}
	return filepath.Join(home, defaultCronDirName, defaultCronFileName)
}

// LoadCronStore reads the JSON store, tolerating missing files.
func LoadCronStore(storePath string) (CronStoreFile, error) {
	data, err := os.ReadFile(storePath)
	if err != nil {
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

// SaveCronStore writes the JSON store atomically and keeps a .bak copy.
func SaveCronStore(storePath string, store CronStoreFile) error {
	if store.Version == 0 {
		store.Version = 1
	}
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return err
	}
	payload, err := json5.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := storePath + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, storePath); err != nil {
		return err
	}
	_ = os.WriteFile(storePath+".bak", payload, 0o644)
	return nil
}

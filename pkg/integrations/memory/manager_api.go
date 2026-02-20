package memory

import (
	"context"
	"errors"
)

// SyncWithProgress forces a reindex and reports indexing progress.
func (m *MemorySearchManager) SyncWithProgress(ctx context.Context, onProgress func(completed, total int, label string)) error {
	if m == nil {
		return errors.New("memory search unavailable")
	}
	return m.syncWithProgress(ctx, "", true, onProgress)
}

package cron

import (
	"strings"
	"sync"
)

var cronStoreLocks sync.Map

func storeLockKey(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "__cron_store__"
	}
	return trimmed
}

func storeLockForPath(path string) *sync.Mutex {
	key := storeLockKey(path)
	if val, ok := cronStoreLocks.Load(key); ok {
		return val.(*sync.Mutex)
	}
	mu := &sync.Mutex{}
	actual, _ := cronStoreLocks.LoadOrStore(key, mu)
	return actual.(*sync.Mutex)
}

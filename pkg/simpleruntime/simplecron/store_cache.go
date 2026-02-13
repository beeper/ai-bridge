package cron

import "sync"

type cronStoreCache struct {
	mu   sync.Mutex
	data map[string]*CronStoreFile
}

var sharedStoreCache = &cronStoreCache{data: make(map[string]*CronStoreFile)}

func getCachedStore(path string) *CronStoreFile {
	if path == "" {
		return nil
	}
	sharedStoreCache.mu.Lock()
	defer sharedStoreCache.mu.Unlock()
	return sharedStoreCache.data[path]
}

func setCachedStore(path string, store *CronStoreFile) {
	if path == "" || store == nil {
		return
	}
	sharedStoreCache.mu.Lock()
	sharedStoreCache.data[path] = store
	sharedStoreCache.mu.Unlock()
}

func clearCachedStore(path string) {
	if path == "" {
		return
	}
	sharedStoreCache.mu.Lock()
	delete(sharedStoreCache.data, path)
	sharedStoreCache.mu.Unlock()
}

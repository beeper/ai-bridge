package cron

import "context"

// StoreEntry represents a key-value entry returned by List.
type StoreEntry struct {
	Key  string
	Data []byte
}

// StoreBackend provides key-value storage access for cron state.
type StoreBackend interface {
	Read(ctx context.Context, path string) ([]byte, bool, error)
	Write(ctx context.Context, path string, data []byte) error
	List(ctx context.Context, prefix string) ([]StoreEntry, error)
}

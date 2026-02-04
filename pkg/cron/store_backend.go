package cron

import "context"

// StoreBackend provides virtual filesystem access for cron state.
type StoreBackend interface {
	Read(ctx context.Context, path string) ([]byte, bool, error)
	Write(ctx context.Context, path string, data []byte) error
}

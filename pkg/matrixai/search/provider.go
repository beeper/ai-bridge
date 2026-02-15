package search

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/core/shared/registry"
)

// Provider performs web searches for a given backend.
type Provider interface {
	Name() string
	Search(ctx context.Context, req Request) (*Response, error)
}

// Registry is a thin wrapper around the generic provider registry.
type Registry struct {
	inner *registry.Registry[Provider]
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{inner: registry.New[Provider]()}
}

// Register adds or replaces a provider by name.
func (r *Registry) Register(provider Provider) {
	if r == nil || r.inner == nil {
		return
	}
	r.inner.Register(provider)
}

// Get returns a registered provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	if r == nil || r.inner == nil {
		var zero Provider
		return zero, false
	}
	return r.inner.Get(name)
}

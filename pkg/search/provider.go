package search

import "context"

// Provider performs web searches for a given backend.
type Provider interface {
	Name() string
	Search(ctx context.Context, req Request) (*Response, error)
}

// Registry stores named providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds or replaces a provider by name.
func (r *Registry) Register(provider Provider) {
	if r == nil || provider == nil {
		return
	}
	if r.providers == nil {
		r.providers = make(map[string]Provider)
	}
	r.providers[provider.Name()] = provider
}

// Get returns a provider by name.
func (r *Registry) Get(name string) Provider {
	if r == nil {
		return nil
	}
	return r.providers[name]
}

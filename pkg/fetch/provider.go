package fetch

import "context"

// Provider fetches readable content for a given backend.
type Provider interface {
	Name() string
	Fetch(ctx context.Context, req Request) (*Response, error)
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

// Names returns registered provider names.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.providers))
	for name := range r.providers {
		out = append(out, name)
	}
	return out
}

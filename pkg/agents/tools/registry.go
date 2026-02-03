package tools

import (
	"maps"
	"sort"
	"sync"
)

// Registry manages available tools with grouping and aliasing support.
type Registry struct {
	mu      sync.RWMutex
	tools   map[string]*Tool    // name -> tool
	groups  map[string][]string // group name -> tool names
	aliases map[string]string   // alias -> canonical name
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:   make(map[string]*Tool),
		groups:  make(map[string][]string),
		aliases: make(map[string]string),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name
	r.tools[name] = tool

	// Add to group if specified
	if tool.Group != "" {
		r.groups[tool.Group] = append(r.groups[tool.Group], name)
	}
}

// RegisterAlias creates an alias for a tool (e.g., "search" -> "web_search").
func (r *Registry) RegisterAlias(alias, canonical string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases[alias] = canonical
}

// Get retrieves a tool by name, resolving aliases.
func (r *Registry) Get(name string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Resolve alias first
	if canonical, ok := r.aliases[name]; ok {
		name = canonical
	}

	return r.tools[name]
}

// Has checks if a tool exists by name.
func (r *Registry) Has(name string) bool {
	return r.Get(name) != nil
}

// GetByGroup returns all tools in a group.
func (r *Registry) GetByGroup(group string) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names, ok := r.groups[group]
	if !ok {
		return nil
	}

	tools := make([]*Tool, 0, len(names))
	for _, name := range names {
		if tool, ok := r.tools[name]; ok {
			tools = append(tools, tool)
		}
	}
	return tools
}

// All returns all registered tools.
func (r *Registry) All() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]*Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	// Sort by name for consistent ordering
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	return tools
}

// Groups returns all group names.
func (r *Registry) Groups() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	groups := make([]string, 0, len(r.groups))
	for group := range r.groups {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups
}

// ToolsInGroup returns tool names in a group.
func (r *Registry) ToolsInGroup(group string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.groups[group]
	if names == nil {
		return nil
	}

	// Return copy to prevent mutation
	result := make([]string, len(names))
	copy(result, names)
	return result
}

// Filter returns tools matching a predicate.
func (r *Registry) Filter(pred func(*Tool) bool) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Tool
	for _, tool := range r.tools {
		if pred(tool) {
			result = append(result, tool)
		}
	}
	return result
}

// ByType returns tools of a specific type.
func (r *Registry) ByType(toolType ToolType) []*Tool {
	return r.Filter(func(t *Tool) bool {
		return t.Type == toolType
	})
}

// Clone creates a copy of the registry.
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	newReg := NewRegistry()
	for name, tool := range r.tools {
		newReg.tools[name] = tool.Clone()
	}
	for group, names := range r.groups {
		newReg.groups[group] = append([]string{}, names...)
	}
	maps.Copy(newReg.aliases, r.aliases)
	return newReg
}

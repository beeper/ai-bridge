package tools

import (
	"context"
	"fmt"
)

// Policy defines which tools are allowed or denied.
type Policy struct {
	Allowed   map[string]bool // Explicitly allowed tools
	Denied    map[string]bool // Explicitly denied tools
	AllowAll  bool            // If true, allow all tools except denied
	DenyAll   bool            // If true, deny all tools except allowed
}

// NewPolicy creates a new empty policy.
func NewPolicy() *Policy {
	return &Policy{
		Allowed: make(map[string]bool),
		Denied:  make(map[string]bool),
	}
}

// AllowAllPolicy creates a policy that allows all tools.
func AllowAllPolicy() *Policy {
	return &Policy{
		Allowed:  make(map[string]bool),
		Denied:   make(map[string]bool),
		AllowAll: true,
	}
}

// DenyAllPolicy creates a policy that denies all tools.
func DenyAllPolicy() *Policy {
	return &Policy{
		Allowed: make(map[string]bool),
		Denied:  make(map[string]bool),
		DenyAll: true,
	}
}

// Allow explicitly allows a tool.
func (p *Policy) Allow(name string) *Policy {
	p.Allowed[name] = true
	delete(p.Denied, name)
	return p
}

// Deny explicitly denies a tool.
func (p *Policy) Deny(name string) *Policy {
	p.Denied[name] = true
	delete(p.Allowed, name)
	return p
}

// AllowGroup allows all tools in a group.
func (p *Policy) AllowGroup(registry *Registry, group string) *Policy {
	for _, name := range registry.ToolsInGroup(group) {
		p.Allow(name)
	}
	return p
}

// DenyGroup denies all tools in a group.
func (p *Policy) DenyGroup(registry *Registry, group string) *Policy {
	for _, name := range registry.ToolsInGroup(group) {
		p.Deny(name)
	}
	return p
}

// IsAllowed checks if a tool is allowed by this policy.
func (p *Policy) IsAllowed(name string) bool {
	// Explicit deny takes precedence
	if p.Denied[name] {
		return false
	}

	// Explicit allow
	if p.Allowed[name] {
		return true
	}

	// Default behavior
	if p.DenyAll {
		return false
	}
	if p.AllowAll {
		return true
	}

	// Default: deny if not explicitly allowed
	return false
}

// Executor handles tool execution with policy enforcement.
type Executor struct {
	registry *Registry
	policy   *Policy
	guard    *Guard
}

// NewExecutor creates a new executor with the given registry and policy.
func NewExecutor(registry *Registry, policy *Policy) *Executor {
	if policy == nil {
		policy = AllowAllPolicy()
	}
	return &Executor{
		registry: registry,
		policy:   policy,
		guard:    DefaultGuard(),
	}
}

// NewExecutorWithGuard creates an executor with a custom guard.
func NewExecutorWithGuard(registry *Registry, policy *Policy, guard *Guard) *Executor {
	if policy == nil {
		policy = AllowAllPolicy()
	}
	if guard == nil {
		guard = DefaultGuard()
	}
	return &Executor{
		registry: registry,
		policy:   policy,
		guard:    guard,
	}
}

// Execute runs a tool if allowed by policy.
func (e *Executor) Execute(ctx context.Context, name string, input map[string]any) (*Result, error) {
	tool := e.registry.Get(name)
	if tool == nil {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}

	// Policy check
	if !e.policy.IsAllowed(name) {
		return nil, fmt.Errorf("tool %s is not allowed by policy", name)
	}

	// Provider tools have no local executor
	if tool.Execute == nil {
		return nil, fmt.Errorf("tool %s has no local executor (provider tool)", name)
	}

	// Execute the tool
	return tool.Execute(ctx, input)
}

// ExecuteWithID runs a tool with call ID tracking via guard.
func (e *Executor) ExecuteWithID(ctx context.Context, callID, name string, input map[string]any) (*Result, error) {
	// Register with guard
	if !e.guard.Register(callID, name, input) {
		return nil, fmt.Errorf("duplicate tool call: %s", callID)
	}

	result, err := e.Execute(ctx, name, input)

	// Complete the guard entry
	e.guard.Complete(callID)

	return result, err
}

// CanExecute checks if a tool can be executed (exists and is allowed).
func (e *Executor) CanExecute(name string) bool {
	tool := e.registry.Get(name)
	if tool == nil {
		return false
	}
	return e.policy.IsAllowed(name)
}

// AllowedTools returns all tools that are allowed by the policy.
func (e *Executor) AllowedTools() []*Tool {
	var allowed []*Tool
	for _, tool := range e.registry.All() {
		if e.policy.IsAllowed(tool.Name) {
			allowed = append(allowed, tool)
		}
	}
	return allowed
}

// AllowedToolInfos returns info about allowed tools.
func (e *Executor) AllowedToolInfos() []ToolInfo {
	var infos []ToolInfo
	for _, tool := range e.AllowedTools() {
		infos = append(infos, ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Type:        tool.Type,
			Group:       tool.Group,
			Enabled:     true,
		})
	}
	return infos
}

// Registry returns the underlying registry.
func (e *Executor) Registry() *Registry {
	return e.registry
}

// Policy returns the current policy.
func (e *Executor) Policy() *Policy {
	return e.policy
}

// Guard returns the underlying guard.
func (e *Executor) Guard() *Guard {
	return e.guard
}

// SetPolicy updates the executor's policy.
func (e *Executor) SetPolicy(policy *Policy) {
	e.policy = policy
}

// WithPolicy returns a new executor with a different policy.
func (e *Executor) WithPolicy(policy *Policy) *Executor {
	return &Executor{
		registry: e.registry,
		policy:   policy,
		guard:    e.guard,
	}
}

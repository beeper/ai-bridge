package tools

import (
	"context"
	"errors"
	"fmt"
)

type Result struct {
	Status  string
	Content string
}

type Tool struct {
	Execute func(context.Context, map[string]any) (*Result, error)
}

type ToolRegistry interface {
	Get(string) *Tool
}

// Executor provides a transport-agnostic way to execute tools from the shared registry.
// Higher layers (bridge/bot) provide policy + per-room enablement checks.
type Executor struct {
	Registry ToolRegistry
}

func (e *Executor) Execute(ctx context.Context, toolName string, input map[string]any) (*Result, error) {
	if e == nil || e.Registry == nil {
		return nil, errors.New("missing tool registry")
	}
	t := e.Registry.Get(toolName)
	if t == nil {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	if t.Execute == nil {
		return nil, fmt.Errorf("tool %s has no local executor", toolName)
	}
	return t.Execute(ctx, input)
}

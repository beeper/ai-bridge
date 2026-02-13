package connector

import "strings"

var simpleCommandSet = map[string]struct{}{
	"activation":    {},
	"approve":       {},
	"commands":      {},
	"config":        {},
	"context":       {},
	"debounce":      {},
	"elevated":      {},
	"fork":          {},
	"gravatar":      {},
	"mode":          {},
	"model":         {},
	"models":        {},
	"new":           {},
	"playground":    {},
	"queue":         {},
	"reasoning":     {},
	"regenerate":    {},
	"reset":         {},
	"send":          {},
	"status":        {},
	"stop":          {},
	"system-prompt": {},
	"temp":          {},
	"think":         {},
	"timezone":      {},
	"title":         {},
	"tokens":        {},
	"typing":        {},
	"verbose":       {},
	"whoami":        {},
}

func (oc *OpenAIConnector) shouldRegisterCommand(name string) bool {
	if oc == nil {
		return false
	}
	policy := oc.bridgePolicy()
	if strings.TrimSpace(policy.NetworkID) != "ai-simple" {
		return true
	}
	_, ok := simpleCommandSet[name]
	return ok
}

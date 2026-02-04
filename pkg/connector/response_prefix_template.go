package connector

import (
	"regexp"
	"strings"
)

// ResponsePrefixContext mirrors OpenClaw's template context.
type ResponsePrefixContext struct {
	Model         string
	ModelFull     string
	Provider      string
	ThinkingLevel string
	IdentityName  string
}

var responsePrefixTemplatePattern = regexp.MustCompile(`\{([a-zA-Z][a-zA-Z0-9.]*)\}`)
var responsePrefixDateSuffix = regexp.MustCompile(`-\d{8}$`)

func resolveResponsePrefixTemplate(template string, ctx ResponsePrefixContext) string {
	if template == "" {
		return ""
	}
	return responsePrefixTemplatePattern.ReplaceAllStringFunc(template, func(match string) string {
		groups := responsePrefixTemplatePattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		varName := strings.ToLower(groups[1])
		switch varName {
		case "model":
			if ctx.Model != "" {
				return ctx.Model
			}
		case "modelfull":
			if ctx.ModelFull != "" {
				return ctx.ModelFull
			}
		case "provider":
			if ctx.Provider != "" {
				return ctx.Provider
			}
		case "thinkinglevel", "think":
			if ctx.ThinkingLevel != "" {
				return ctx.ThinkingLevel
			}
		case "identity.name", "identityname":
			if ctx.IdentityName != "" {
				return ctx.IdentityName
			}
		}
		return match
	})
}

func extractShortModelName(fullModel string) string {
	modelPart := strings.TrimSpace(fullModel)
	if modelPart == "" {
		return ""
	}
	if idx := strings.LastIndex(modelPart, "/"); idx >= 0 && idx+1 < len(modelPart) {
		modelPart = modelPart[idx+1:]
	}
	modelPart = responsePrefixDateSuffix.ReplaceAllString(modelPart, "")
	modelPart = strings.TrimSuffix(modelPart, "-latest")
	return modelPart
}

package connector

import (
	"context"
	"strings"
)

func resolveChannelResponsePrefix(cfg *Config) string {
	if cfg == nil || cfg.Channels == nil {
		return ""
	}
	if cfg.Channels.Matrix != nil {
		if trimmed := strings.TrimSpace(cfg.Channels.Matrix.ResponsePrefix); trimmed != "" {
			return trimmed
		}
	}
	if cfg.Channels.Defaults != nil {
		if trimmed := strings.TrimSpace(cfg.Channels.Defaults.ResponsePrefix); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resolveResponsePrefixRaw(oc *AIClient, cfg *Config, meta *PortalMetadata) string {
	if meta != nil {
		if trimmed := strings.TrimSpace(meta.ResponsePrefix); trimmed != "" {
			return trimmed
		}
	}
	if oc != nil && oc.UserLogin != nil {
		if login := loginMetadata(oc.UserLogin); login != nil {
			if trimmed := strings.TrimSpace(login.ResponsePrefix); trimmed != "" {
				return trimmed
			}
		}
	}
	if channelPrefix := resolveChannelResponsePrefix(cfg); channelPrefix != "" {
		return channelPrefix
	}
	if cfg == nil || cfg.Messages == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Messages.ResponsePrefix)
}

func resolveIdentityNameForPrefix(oc *AIClient, agentID string) string {
	if oc == nil {
		return ""
	}
	resolved := strings.TrimSpace(agentID)
	if resolved == "" {
		resolved = defaultAgentID
	}
	store := NewAgentStoreAdapter(oc)
	if agent, err := store.GetAgentByID(context.Background(), resolved); err == nil && agent != nil {
		if agent.Identity != nil && strings.TrimSpace(agent.Identity.Name) != "" {
			return strings.TrimSpace(agent.Identity.Name)
		}
	}
	return oc.resolveAgentIdentityName(context.Background(), resolved)
}

func buildResponsePrefixContext(oc *AIClient, agentID string, meta *PortalMetadata) ResponsePrefixContext {
	ctx := ResponsePrefixContext{
		IdentityName: resolveIdentityNameForPrefix(oc, agentID),
	}
	if oc == nil {
		return ctx
	}
	modelFull := oc.effectiveModel(meta)
	if modelFull != "" {
		ctx.ModelFull = modelFull
		ctx.Model = extractShortModelName(modelFull)
		if idx := strings.Index(modelFull, "/"); idx > 0 {
			ctx.Provider = modelFull[:idx]
		}
	}
	if ctx.Provider == "" {
		if login := loginMetadata(oc.UserLogin); login != nil {
			ctx.Provider = strings.TrimSpace(login.Provider)
		}
	}
	think := strings.TrimSpace(oc.effectiveReasoningEffort(meta))
	if think == "" {
		think = "off"
	}
	ctx.ThinkingLevel = think
	return ctx
}

func resolveResponsePrefixForHeartbeat(oc *AIClient, cfg *Config, agentID string, meta *PortalMetadata) string {
	raw := resolveResponsePrefixRaw(oc, cfg, meta)
	if raw == "" {
		return ""
	}
	if strings.EqualFold(raw, "auto") {
		name := resolveIdentityNameForPrefix(oc, agentID)
		if name == "" {
			return ""
		}
		return "[" + name + "]"
	}
	ctx := buildResponsePrefixContext(oc, agentID, meta)
	return resolveResponsePrefixTemplate(raw, ctx)
}

func resolveResponsePrefixForReply(oc *AIClient, cfg *Config, meta *PortalMetadata) string {
	raw := resolveResponsePrefixRaw(oc, cfg, meta)
	if raw == "" {
		return ""
	}
	agentID := resolveAgentID(meta)
	if strings.EqualFold(raw, "auto") {
		name := resolveIdentityNameForPrefix(oc, agentID)
		if name == "" {
			return ""
		}
		return "[" + name + "]"
	}
	ctx := buildResponsePrefixContext(oc, agentID, meta)
	return resolveResponsePrefixTemplate(raw, ctx)
}

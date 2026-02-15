package connector

import (
	"strings"

	"github.com/beeper/ai-bridge/pkg/core/aiqueue"
)

type queueResolveParams struct {
	cfg        *Config
	channel    string
	inlineMode aiqueue.QueueMode
	inlineOpts aiqueue.QueueInlineOptions
}

func resolveQueueSettings(params queueResolveParams) aiqueue.QueueSettings {
	channel := strings.TrimSpace(strings.ToLower(params.channel))
	cfg := params.cfg
	queueCfg := (*QueueConfig)(nil)
	if cfg != nil && cfg.Messages != nil {
		queueCfg = cfg.Messages.Queue
	}

	resolvedMode := params.inlineMode
	if resolvedMode == "" && queueCfg != nil {
		if channel != "" && queueCfg.ByChannel != nil {
			if raw, ok := queueCfg.ByChannel[channel]; ok {
				if mode, ok := aiqueue.NormalizeQueueMode(raw); ok {
					resolvedMode = mode
				}
			}
		}
		if resolvedMode == "" {
			if mode, ok := aiqueue.NormalizeQueueMode(queueCfg.Mode); ok {
				resolvedMode = mode
			}
		}
	}
	if resolvedMode == "" {
		resolvedMode = aiqueue.DefaultQueueMode
	}

	debounce := (*int)(nil)
	if params.inlineOpts.DebounceMs != nil {
		debounce = params.inlineOpts.DebounceMs
	} else if queueCfg != nil {
		if channel != "" && queueCfg.DebounceMsByChannel != nil {
			if v, ok := queueCfg.DebounceMsByChannel[channel]; ok {
				debounce = &v
			}
		}
		if debounce == nil && queueCfg.DebounceMs != nil {
			debounce = queueCfg.DebounceMs
		}
	}

	debounceMs := aiqueue.DefaultQueueDebounceMs
	if debounce != nil {
		debounceMs = *debounce
		if debounceMs < 0 {
			debounceMs = 0
		}
	}

	capValue := (*int)(nil)
	if params.inlineOpts.Cap != nil {
		capValue = params.inlineOpts.Cap
	} else if queueCfg != nil && queueCfg.Cap != nil {
		capValue = queueCfg.Cap
	}
	cap := aiqueue.DefaultQueueCap
	if capValue != nil {
		if *capValue > 0 {
			cap = *capValue
		}
	}

	dropPolicy := aiqueue.QueueDropPolicy("")
	if params.inlineOpts.DropPolicy != nil {
		dropPolicy = *params.inlineOpts.DropPolicy
	} else if queueCfg != nil {
		if policy, ok := aiqueue.NormalizeQueueDropPolicy(queueCfg.Drop); ok {
			dropPolicy = policy
		}
	}
	if dropPolicy == "" {
		dropPolicy = aiqueue.DefaultQueueDrop
	}

	return aiqueue.QueueSettings{
		Mode:       resolvedMode,
		DebounceMs: debounceMs,
		Cap:        cap,
		DropPolicy: dropPolicy,
	}
}

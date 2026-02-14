package websearch

import base "github.com/beeper/ai-bridge/pkg/core/shared/websearch"

func ParseCountAndIgnoredOptions(args map[string]any) (int, []string) {
	return base.ParseCountAndIgnoredOptions(args)
}

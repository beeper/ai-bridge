package websearch

import "github.com/beeper/ai-bridge/pkg/shared/maputil"

// ParseCountAndIgnoredOptions extracts count and unsupported option warnings from args.
func ParseCountAndIgnoredOptions(args map[string]any) (int, []string) {
	count := 5
	if v, ok := maputil.IntArg(args, "count"); ok {
		count = v
	}
	if count < 1 {
		count = 1
	} else if count > 10 {
		count = 10
	}

	var ignoredOptions []string
	for _, key := range []string{"country", "search_lang", "ui_lang", "freshness"} {
		if maputil.StringArg(args, key) != "" {
			ignoredOptions = append(ignoredOptions, key)
		}
	}

	return count, ignoredOptions
}

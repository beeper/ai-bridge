package websearch

import "strings"

// ParseCountAndIgnoredOptions extracts count and unsupported option warnings from args.
func ParseCountAndIgnoredOptions(args map[string]any) (int, []string) {
	count := 5
	if args != nil {
		if rawCount, ok := args["count"]; ok {
			switch v := rawCount.(type) {
			case float64:
				count = int(v)
			case int:
				count = v
			case int64:
				count = int(v)
			}
		}
	}
	if count < 1 {
		count = 1
	} else if count > 10 {
		count = 10
	}

	var ignoredOptions []string
	if v, _ := args["country"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "country")
	}
	if v, _ := args["search_lang"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "search_lang")
	}
	if v, _ := args["ui_lang"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "ui_lang")
	}
	if v, _ := args["freshness"].(string); strings.TrimSpace(v) != "" {
		ignoredOptions = append(ignoredOptions, "freshness")
	}

	return count, ignoredOptions
}

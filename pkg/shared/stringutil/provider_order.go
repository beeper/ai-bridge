package stringutil

import (
	"slices"
	"strings"
)

// BuildProviderOrder constructs a deduplicated provider order list.
// primary is placed first (unless empty or "auto"), followed by fallbacks.
// If the resulting list is empty, a clone of defaultOrder is returned.
func BuildProviderOrder(primary string, fallbacks []string, defaultOrder []string) []string {
	order := make([]string, 0, len(fallbacks)+1)
	p := strings.TrimSpace(primary)
	if p != "" && p != "auto" {
		order = append(order, p)
	}
	order = append(order, fallbacks...)
	result := DedupeStrings(order)
	if len(result) == 0 {
		return slices.Clone(defaultOrder)
	}
	return result
}

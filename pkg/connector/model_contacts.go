package connector

import "strings"

func modelContactName(modelID string, info *ModelInfo) string {
	if info != nil && info.Name != "" {
		return info.Name
	}
	return FormatModelDisplay(modelID)
}

func modelContactProvider(modelID string, info *ModelInfo) string {
	if info != nil && info.Provider != "" {
		return info.Provider
	}
	if backend, _ := ParseModelPrefix(modelID); backend != "" {
		return string(backend)
	}
	return ""
}

func modelContactIdentifiers(modelID string, info *ModelInfo) []string {
	identifiers := []string{modelID}
	name := modelContactName(modelID, info)
	if name != "" && name != modelID {
		identifiers = append(identifiers, name)
	}
	if provider := modelContactProvider(modelID, info); provider != "" {
		if name != "" {
			identifiers = append(identifiers, provider+"/"+name)
		}
		lowerProvider := strings.ToLower(provider) + "/"
		if !strings.HasPrefix(strings.ToLower(modelID), lowerProvider) {
			identifiers = append(identifiers, provider+"/"+modelID)
		}
	}
	return uniqueStrings(identifiers)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

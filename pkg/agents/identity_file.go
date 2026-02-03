package agents

import "strings"

// IdentityFile represents values parsed from IDENTITY.md.
type IdentityFile struct {
	Name     string
	Emoji    string
	Theme    string
	Creature string
	Vibe     string
	Avatar   string
}

var identityPlaceholderValues = map[string]struct{}{
	"pick something you like":                                  {},
	"ai? robot? familiar? ghost in the machine? something weirder?": {},
	"how do you come across? sharp? warm? chaotic? calm?":      {},
	"your signature - pick one that feels right":              {},
	"workspace-relative path, http(s) url, or data uri":       {},
}

func normalizeIdentityValue(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.Trim(normalized, "*_")
	if strings.HasPrefix(normalized, "(") && strings.HasSuffix(normalized, ")") {
		normalized = strings.TrimSpace(normalized[1 : len(normalized)-1])
	}
	replacer := strings.NewReplacer("\u2013", "-", "\u2014", "-")
	normalized = replacer.Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	normalized = strings.ToLower(normalized)
	return normalized
}

func isIdentityPlaceholder(value string) bool {
	normalized := normalizeIdentityValue(value)
	_, ok := identityPlaceholderValues[normalized]
	return ok
}

// ParseIdentityMarkdown extracts identity fields from IDENTITY.md content.
func ParseIdentityMarkdown(content string) IdentityFile {
	identity := IdentityFile{}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		cleaned := strings.TrimSpace(line)
		cleaned = strings.TrimPrefix(cleaned, "-")
		cleaned = strings.TrimSpace(cleaned)
		colonIndex := strings.Index(cleaned, ":")
		if colonIndex == -1 {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(strings.Trim(cleaned[:colonIndex], "*_")))
		value := strings.TrimSpace(strings.Trim(cleaned[colonIndex+1:], "*_"))
		if value == "" {
			continue
		}
		if isIdentityPlaceholder(value) {
			continue
		}
		switch label {
		case "name":
			identity.Name = value
		case "emoji":
			identity.Emoji = value
		case "theme":
			identity.Theme = value
		case "creature":
			identity.Creature = value
		case "vibe":
			identity.Vibe = value
		case "avatar":
			identity.Avatar = value
		}
	}
	return identity
}

// IdentityHasValues returns true if any identity fields are set.
func IdentityHasValues(identity IdentityFile) bool {
	return identity.Name != "" ||
		identity.Emoji != "" ||
		identity.Theme != "" ||
		identity.Creature != "" ||
		identity.Vibe != "" ||
		identity.Avatar != ""
}

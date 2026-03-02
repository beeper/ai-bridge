package connector

var thinkLevelAliases = map[string]string{
	"off":     "off",
	"on":      "low",
	"minimal": "minimal",
	"low":     "low",
	"medium":  "medium",
	"high":    "high",
	"xhigh":   "xhigh",
}

var verboseLevelAliases = map[string]string{
	"off":  "off",
	"on":   "on",
	"full": "full",
}

var reasoningLevelAliases = map[string]string{
	"off":    "off",
	"on":     "on",
	"low":    "low",
	"medium": "medium",
	"high":   "high",
	"xhigh":  "xhigh",
}

var sendPolicyAliases = map[string]string{
	"on":      "allow",
	"off":     "deny",
	"inherit": "inherit",
}

var groupActivationAliases = map[string]string{
	"mention": "mention",
	"always":  "always",
}

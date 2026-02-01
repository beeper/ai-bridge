package connector

import (
	"regexp"
	"strings"

	"maunium.net/go/mautrix/id"
)

// SilentReplyToken is the token the agent uses to indicate no response is needed.
// Matches clawdbot/OpenClaw's SILENT_REPLY_TOKEN.
const SilentReplyToken = "NO_REPLY"

// ResponseDirectives contains parsed directives from an LLM response.
type ResponseDirectives struct {
	// Text is the cleaned response text with directives stripped.
	Text string

	// IsSilent indicates the response should not be sent (NO_REPLY token present).
	IsSilent bool

	// ReplyToEventID is the Matrix event ID to reply to (from [[reply_to:<id>]] or [[reply_to_current]]).
	ReplyToEventID id.EventID

	// ReplyToCurrent indicates [[reply_to_current]] was used (reply to triggering message).
	ReplyToCurrent bool

	// Reactions contains emoji reactions to send (from [[react:<emoji>]] or [[react:<emoji>:<event_id>]]).
	Reactions []ReactionDirective

	// HasReplyTag indicates a reply tag was present in the original text.
	HasReplyTag bool
}

// ReactionDirective represents a single reaction to send.
type ReactionDirective struct {
	Emoji   string     // The emoji to react with
	EventID id.EventID // Target event (empty = react to current message)
}

var (
	// replyTagRE matches [[reply_to_current]] or [[reply_to:<id>]]
	// Allows whitespace inside brackets: [[ reply_to_current ]] or [[ reply_to: abc123 ]]
	replyTagRE = regexp.MustCompile(`\[\[\s*(?:reply_to_current|reply_to\s*:\s*([^\]\n]+))\s*\]\]`)

	// reactTagRE matches [[react:<emoji>]] or [[react:<emoji>:<event_id>]]
	// Examples: [[react:👍]], [[react:🎉:$abc123]]
	reactTagRE = regexp.MustCompile(`\[\[\s*react\s*:\s*([^\]:\s]+)(?:\s*:\s*([^\]\s]+))?\s*\]\]`)

	// silentPrefixRE matches NO_REPLY at the start (with optional whitespace)
	silentPrefixRE = regexp.MustCompile(`^\s*` + regexp.QuoteMeta(SilentReplyToken) + `(?:$|\W)`)

	// silentSuffixRE matches NO_REPLY at the end (word boundary)
	silentSuffixRE = regexp.MustCompile(`\b` + regexp.QuoteMeta(SilentReplyToken) + `\b\W*$`)
)

// ParseResponseDirectives extracts directives from LLM response text.
// currentEventID is the triggering message's event ID (used for [[reply_to_current]]).
func ParseResponseDirectives(text string, currentEventID id.EventID) *ResponseDirectives {
	if text == "" {
		return &ResponseDirectives{IsSilent: true}
	}

	result := &ResponseDirectives{}
	cleaned := text

	// Check for silent reply token (at start or end)
	if isSilentReplyText(text) {
		result.IsSilent = true
		// Still parse other directives but mark as silent
	}

	// Parse reply tags
	var sawCurrent bool
	var lastExplicitID string

	cleaned = replyTagRE.ReplaceAllStringFunc(cleaned, func(match string) string {
		result.HasReplyTag = true
		submatches := replyTagRE.FindStringSubmatch(match)
		if len(submatches) > 1 && submatches[1] != "" {
			// Explicit ID: [[reply_to:<id>]]
			lastExplicitID = strings.TrimSpace(submatches[1])
		} else {
			// [[reply_to_current]]
			sawCurrent = true
		}
		return " " // Replace with space to maintain word boundaries
	})

	// Resolve reply target (explicit ID takes precedence)
	if lastExplicitID != "" {
		result.ReplyToEventID = id.EventID(lastExplicitID)
	} else if sawCurrent && currentEventID != "" {
		result.ReplyToEventID = currentEventID
		result.ReplyToCurrent = true
	}

	// Parse reaction tags
	cleaned = reactTagRE.ReplaceAllStringFunc(cleaned, func(match string) string {
		submatches := reactTagRE.FindStringSubmatch(match)
		if len(submatches) >= 2 {
			emoji := strings.TrimSpace(submatches[1])
			var targetEvent id.EventID
			if len(submatches) >= 3 && submatches[2] != "" {
				targetEvent = id.EventID(strings.TrimSpace(submatches[2]))
			}
			result.Reactions = append(result.Reactions, ReactionDirective{
				Emoji:   emoji,
				EventID: targetEvent,
			})
		}
		return " " // Replace with space
	})

	// Normalize whitespace
	cleaned = normalizeDirectiveWhitespace(cleaned)
	result.Text = cleaned

	// If only whitespace remains after stripping, treat as silent
	if strings.TrimSpace(cleaned) == "" {
		result.IsSilent = true
	}

	return result
}

// isSilentReplyText checks if text starts or ends with the silent token.
// Matches clawdbot's isSilentReplyText behavior.
func isSilentReplyText(text string) bool {
	if text == "" {
		return false
	}
	// Check prefix
	if silentPrefixRE.MatchString(text) {
		return true
	}
	// Check suffix
	return silentSuffixRE.MatchString(text)
}

// normalizeDirectiveWhitespace cleans up whitespace after directive removal.
func normalizeDirectiveWhitespace(text string) string {
	// Collapse multiple spaces into one
	spaceRE := regexp.MustCompile(`[ \t]+`)
	text = spaceRE.ReplaceAllString(text, " ")

	// Normalize newlines with surrounding whitespace
	nlRE := regexp.MustCompile(`[ \t]*\n[ \t]*`)
	text = nlRE.ReplaceAllString(text, "\n")

	// Collapse multiple newlines into two (preserve paragraph breaks)
	multiNL := regexp.MustCompile(`\n{3,}`)
	text = multiNL.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

// StripSilentToken removes the silent token from text if present.
// Returns the cleaned text.
func StripSilentToken(text string) string {
	// Remove from start
	text = silentPrefixRE.ReplaceAllString(text, "")
	// Remove from end
	text = silentSuffixRE.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

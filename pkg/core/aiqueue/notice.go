package aiqueue

import (
	"fmt"
)

const QueueDirectiveOptionsHint = "modes steer, followup, collect, steer+backlog, interrupt; debounce:<ms|s|m>, cap:<n>, drop:old|new|summarize"

func BuildQueueStatusLine(settings QueueSettings) string {
	debounceLabel := fmt.Sprintf("%dms", settings.DebounceMs)
	capLabel := fmt.Sprintf("%d", settings.Cap)
	dropLabel := string(settings.DropPolicy)
	return fmt.Sprintf(
		"Current queue settings: mode=%s, debounce=%s, cap=%s, drop=%s.\nOptions: %s.",
		settings.Mode,
		debounceLabel,
		capLabel,
		dropLabel,
		QueueDirectiveOptionsHint,
	)
}

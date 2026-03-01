package connector

import (
	"strings"

	airuntime "github.com/beeper/ai-bridge/pkg/runtime"
)

type QueueMode = airuntime.QueueMode

const (
	QueueModeSteer        QueueMode = airuntime.QueueModeSteer
	QueueModeFollowup     QueueMode = airuntime.QueueModeFollowup
	QueueModeCollect      QueueMode = airuntime.QueueModeCollect
	QueueModeSteerBacklog QueueMode = airuntime.QueueModeSteerBacklog
	QueueModeInterrupt    QueueMode = airuntime.QueueModeInterrupt
)

type QueueDropPolicy string

const (
	QueueDropOld       QueueDropPolicy = "old"
	QueueDropNew       QueueDropPolicy = "new"
	QueueDropSummarize QueueDropPolicy = "summarize"
)

const (
	DefaultQueueDebounceMs = 1000
	DefaultQueueCap        = 20
)

const DefaultQueueDrop = QueueDropSummarize
const DefaultQueueMode = QueueModeCollect

type QueueSettings struct {
	Mode       QueueMode
	DebounceMs int
	Cap        int
	DropPolicy QueueDropPolicy
}

type QueueInlineOptions struct {
	DebounceMs *int
	Cap        *int
	DropPolicy *QueueDropPolicy
}

func normalizeQueueMode(raw string) (QueueMode, bool) {
	cleaned := strings.TrimSpace(strings.ToLower(raw))
	switch cleaned {
	case "queue", "queued":
		return QueueModeSteer, true
	case "interrupt", "interrupts", "abort":
		return QueueModeInterrupt, true
	case "steer", "steering":
		return QueueModeSteer, true
	case "followup", "follow-ups", "followups":
		return QueueModeFollowup, true
	case "collect", "coalesce":
		return QueueModeCollect, true
	case "steer+backlog", "steer-backlog", "steer_backlog":
		return QueueModeSteerBacklog, true
	default:
		return "", false
	}
}

func normalizeQueueDropPolicy(raw string) (QueueDropPolicy, bool) {
	cleaned := strings.TrimSpace(strings.ToLower(raw))
	switch cleaned {
	case "old", "oldest":
		return QueueDropOld, true
	case "new", "newest":
		return QueueDropNew, true
	case "summarize", "summary":
		return QueueDropSummarize, true
	default:
		return "", false
	}
}

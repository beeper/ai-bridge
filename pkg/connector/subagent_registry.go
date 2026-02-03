package connector

import (
	"time"

	"maunium.net/go/mautrix/id"
)

type subagentRun struct {
	RunID        string
	ChildRoomID  id.RoomID
	ParentRoomID id.RoomID
	Label        string
	Task         string
	Cleanup      string
	StartedAt    time.Time
	Timeout      time.Duration
}

func (oc *AIClient) registerSubagentRun(run *subagentRun) {
	if oc == nil || run == nil || run.RunID == "" {
		return
	}
	oc.subagentRunsMu.Lock()
	defer oc.subagentRunsMu.Unlock()
	if oc.subagentRuns == nil {
		oc.subagentRuns = make(map[string]*subagentRun)
	}
	oc.subagentRuns[run.RunID] = run
}

func (oc *AIClient) unregisterSubagentRun(runID string) {
	if oc == nil || runID == "" {
		return
	}
	oc.subagentRunsMu.Lock()
	defer oc.subagentRunsMu.Unlock()
	delete(oc.subagentRuns, runID)
}

func (oc *AIClient) subagentRun(runID string) *subagentRun {
	if oc == nil || runID == "" {
		return nil
	}
	oc.subagentRunsMu.Lock()
	defer oc.subagentRunsMu.Unlock()
	return oc.subagentRuns[runID]
}

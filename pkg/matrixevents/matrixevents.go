package matrixevents

import contractevents "github.com/beeper/ai-bridge/modules/contracts/matrixevents"

var (
	AssistantTurnEventType = contractevents.AssistantTurnEventType
	ToolCallEventType      = contractevents.ToolCallEventType
	ToolResultEventType    = contractevents.ToolResultEventType
	AIErrorEventType       = contractevents.AIErrorEventType
	TurnCancelledEventType = contractevents.TurnCancelledEventType
	AgentHandoffEventType  = contractevents.AgentHandoffEventType
	StepBoundaryEventType  = contractevents.StepBoundaryEventType

	StreamDeltaEventType   = contractevents.StreamDeltaEventType
	StreamEventMessageType = contractevents.StreamEventMessageType

	GenerationStatusEventType = contractevents.GenerationStatusEventType
	ToolProgressEventType     = contractevents.ToolProgressEventType
	CompactionStatusEventType = contractevents.CompactionStatusEventType

	RoomCapabilitiesEventType  = contractevents.RoomCapabilitiesEventType
	RoomSettingsEventType      = contractevents.RoomSettingsEventType
	ModelCapabilitiesEventType = contractevents.ModelCapabilitiesEventType
	AgentsEventType            = contractevents.AgentsEventType
)

const (
	RelReplace   = contractevents.RelReplace
	RelReference = contractevents.RelReference
	RelThread    = contractevents.RelThread
	RelInReplyTo = contractevents.RelInReplyTo

	BeeperAIKey           = contractevents.BeeperAIKey
	BeeperAIToolCallKey   = contractevents.BeeperAIToolCallKey
	BeeperAIToolResultKey = contractevents.BeeperAIToolResultKey
)

type StreamEventOpts = contractevents.StreamEventOpts

func BuildStreamEventEnvelope(turnID string, seq int, part map[string]any, opts StreamEventOpts) (map[string]any, error) {
	return contractevents.BuildStreamEventEnvelope(turnID, seq, part, opts)
}

func BuildStreamEventTxnID(turnID string, seq int) string {
	return contractevents.BuildStreamEventTxnID(turnID, seq)
}

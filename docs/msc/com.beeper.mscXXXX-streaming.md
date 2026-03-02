# com.beeper.ai.stream_event — AI Streaming Profile

**Prior art:** [MSC2477](https://github.com/matrix-org/matrix-spec-proposals/pull/2477) (user-defined ephemeral events)

**Transport:** [com.beeper.ephemeral](com.beeper.mscXXXX-ephemeral.md) (our MSC2477 implementation)

**Status:** Implemented and running in ai-bridge. No code changes needed — documentation only.

## Summary

This document defines a profile on top of `com.beeper.ephemeral` for real-time AI streaming in Matrix rooms. The profile specifies an application-level envelope convention for ordered, resumable streaming of AI assistant output.

## Event Type

```
com.beeper.ai.stream_event
```

Registered as `EphemeralEventType` in mautrix-go.

## Envelope Schema

```json
{
  "turn_id": "uuid-v4",
  "seq": 1,
  "part": {
    "type": "text-delta",
    "text": "Hello"
  },
  "target_event": "$event_id",
  "agent_id": "researcher",
  "m.relates_to": {
    "rel_type": "m.reference",
    "event_id": "$event_id"
  }
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `turn_id` | string | yes | UUID identifying the conversation turn. All stream events for one assistant response share the same `turn_id`. |
| `seq` | integer | yes | Monotonically increasing sequence number (starts at 1). Used for ordering and gap detection. |
| `part` | object | yes | The payload. Structure depends on `part.type`. |
| `target_event` | string | no | Event ID of the timeline message being streamed into. Set once the first timeline event is sent. |
| `agent_id` | string | no | Identifier of the agent producing this stream (for multi-agent rooms). |
| `m.relates_to` | object | no | Standard Matrix relation. When `target_event` is set, includes `rel_type: "m.reference"`. |

### Part Types

| `part.type` | Description |
|-------------|-------------|
| `text-delta` | Incremental text content: `{ "type": "text-delta", "text": "..." }` |
| `thinking-delta` | Reasoning/thinking content: `{ "type": "thinking-delta", "text": "..." }` |
| `tool-call-begin` | Tool invocation start: `{ "type": "tool-call-begin", "toolCallId": "...", "toolName": "..." }` |
| `tool-call-delta` | Tool call argument streaming: `{ "type": "tool-call-delta", "toolCallId": "...", "argsTextDelta": "..." }` |
| `tool-output-available` | Tool result ready: `{ "type": "tool-output-available", "toolCallId": "..." }` |
| `tool-output-denied` | Tool denied: `{ "type": "tool-output-denied", "toolCallId": "..." }` |
| `status` | Stream status: `{ "type": "status", "status": "streaming|done|error" }` |

## Transaction ID Convention

```
ai_stream_{turn_id}_{seq}
```

Built by `BuildStreamEventTxnID()` in `pkg/matrixevents/matrixevents.go`. Ensures idempotent delivery via the `com.beeper.ephemeral` deduplication mechanism.

## E2EE

Inherited from the transport layer. When the room is encrypted, mautrix-go's `SendEphemeralEvent()` wraps the content with Megolm before sending. Clients decrypt using shared room keys.

## Client Behavior

1. Subscribe to `com.beeper.ai.stream_event` in `/sync` ephemeral events
2. Group events by `turn_id`
3. Order by `seq` within each turn
4. Apply `part` content incrementally (text deltas append, tool events update tool state)
5. When `target_event` appears, associate the stream with the timeline message
6. On `status: "done"`, finalize the stream and display the completed timeline message

## Resilience

- Gaps in `seq` indicate missed events (ephemeral events have no delivery guarantee)
- Clients should gracefully degrade: if stream events are missed, the finalized timeline message contains the complete content
- `target_event` allows late-joining clients to skip the stream and read the persisted message

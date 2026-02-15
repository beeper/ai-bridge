# Matrix AI Event Contract (v1)

This document defines the Matrix event contract emitted by the ai-bridge simple runtime.

## Message Events

- `com.beeper.ai.assistant_turn`
- `com.beeper.ai.tool_call`
- `com.beeper.ai.tool_result`
- `com.beeper.ai.error`
- `com.beeper.ai.turn_cancelled`
- `com.beeper.ai.step_boundary`
- `com.beeper.ai.generation_status`
- `com.beeper.ai.tool_progress`
- `com.beeper.ai.compaction_status`

## Ephemeral Events

- `com.beeper.ai.stream_delta`
- `com.beeper.ai.stream_event`

`com.beeper.ai.stream_event` payload envelope:

- `turn_id: string` (required)
- `seq: number` (required, > 0)
- `part: object` (required)
- `target_event?: string`
- `m.relates_to?: { rel_type: "m.reference", event_id: string }`

## State Events

- `com.beeper.ai.room_capabilities`
- `com.beeper.ai.room_settings`
- `com.beeper.ai.model_capabilities`

## Shared Keys

- `com.beeper.ai`
- `com.beeper.ai.tool_call`
- `com.beeper.ai.tool_result`

## Relation Types

- `m.replace`
- `m.reference`
- `m.thread`
- `m.in_reply_to`

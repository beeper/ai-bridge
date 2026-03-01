# MSCXXXX: AI Extensions for Matrix Bridge Transport (Beeper profile)

This MSC defines a set of Matrix extensions for AI assistant conversations in bridged rooms.
It standardizes how assistant turns, live streaming deltas, tool execution, approvals, and room
AI settings are represented.

The proposal is based on production behavior in `ai-bridge` and intentionally prefers
interoperability with existing clients over idealized redesign.

## Background

Matrix currently has no single common shape for:

- Streaming assistant tokens and reasoning updates.
- Timeline-visible tool call/result projections.
- Human approval gates for privileged tool execution.
- Room-level AI settings and capability state.

In practice, bridge and client implementations have already converged on a de-facto profile in the
`com.beeper.ai.*` namespace. This MSC formalizes that profile so independent implementations can
interoperate.

## Goals

- Define a canonical assistant message carrier in `m.room.message`.
- Define an ephemeral stream event envelope with deterministic sequencing.
- Define timeline projection events for tools and compaction status.
- Define room state events for AI capabilities and settings.
- Define a portable approval flow for tools requiring human confirmation.
- Preserve fallback readability in non-supporting Matrix clients.

## Non-goals

- Defining provider-side LLM APIs.
- Defining room version changes.
- Defining a stable Matrix `m.*` namespace for AI transport (this remains unstable/vendor-prefixed).

## Proposal

### Event and key registry

The following event types are defined:

- `com.beeper.ai.stream_event` (ephemeral): UI chunk streaming envelope.
- `com.beeper.ai.tool_call` (message): timeline tool invocation projection.
- `com.beeper.ai.tool_result` (message): timeline tool output projection.
- `com.beeper.ai.compaction_status` (message): context compaction lifecycle events.
- `com.beeper.ai.room_capabilities` (state): bridge-controlled capabilities and effective settings.
- `com.beeper.ai.room_settings` (state): user-editable room AI settings.

The following event types are part of the declared surface area but MAY be unused by a given
implementation version:

- `com.beeper.ai.assistant_turn`
- `com.beeper.ai.error`
- `com.beeper.ai.turn_cancelled`
- `com.beeper.ai.agent_handoff`
- `com.beeper.ai.step_boundary`
- `com.beeper.ai.generation_status`
- `com.beeper.ai.tool_progress`
- `com.beeper.ai.stream_delta` (legacy)
- `com.beeper.ai.model_capabilities`
- `com.beeper.ai.agents`

The following content keys are defined:

- `com.beeper.ai` inside `m.room.message` (canonical assistant `UIMessage` payload).
- `com.beeper.ai.tool_call` inside `com.beeper.ai.tool_call` events.
- `com.beeper.ai.tool_result` inside `com.beeper.ai.tool_result` events.
- `com.beeper.ai.approval_decision` in inbound `m.room.message` payloads.
- `com.beeper.ai.model_id` and `com.beeper.ai.agent` as routing/display hints.
- `com.beeper.ai.image_generation` and `com.beeper.ai.tts` for generated media tags.

### Canonical assistant message

Assistant turns MUST be represented in a standard `m.room.message` event carrying:

- Matrix fallback fields (`msgtype`, `body`).
- `com.beeper.ai` containing an AI SDK-compatible `UIMessage` object.

#### `com.beeper.ai` shape

`com.beeper.ai` MUST contain:

- `id: string`
- `role: "assistant"`
- `parts: UIMessagePart[]`

`metadata` SHOULD include:

- `turn_id`
- `agent_id`
- `model`
- `finish_reason`
- `usage` with token counters
- `timing` with unix-ms timestamps

Example:

```json
{
  "type": "m.room.message",
  "content": {
    "msgtype": "m.text",
    "body": "Thinking...",
    "com.beeper.ai": {
      "id": "turn_abc",
      "role": "assistant",
      "metadata": {"turn_id": "turn_abc"},
      "parts": []
    }
  }
}
```

#### Streaming placeholder and final replacement

For streaming turns, senders SHOULD:

1. Send an initial placeholder `m.room.message` with `com.beeper.ai` seed metadata.
2. Emit ephemeral stream chunks (defined below).
3. Finalize with an `m.replace` edit including `m.new_content` and full final `com.beeper.ai` payload.

When using `m.replace`, senders SHOULD include an outer short fallback body and put full textual
assistant output in `m.new_content`.

### Ephemeral streaming event

Streaming uses `com.beeper.ai.stream_event` (ephemeral).

Each event MUST contain:

- `turn_id: string`
- `seq: integer` (strictly increasing per turn, first value 1)
- `part: object` (AI SDK-compatible `UIMessageChunk`)

Each event SHOULD contain:

- `target_event: string` (event ID of placeholder message)
- `m.relates_to: {"rel_type":"m.reference","event_id":"..."}` when `target_event` is present

Each event MAY contain:

- `agent_id: string`

Envelope example:

```json
{
  "turn_id": "turn_abc",
  "seq": 7,
  "target_event": "$placeholder",
  "m.relates_to": {"rel_type": "m.reference", "event_id": "$placeholder"},
  "part": {"type": "text-delta", "id": "text-turn_abc", "delta": "hello"}
}
```

#### Sequencing and ordering

Producers MUST emit monotonically increasing `seq` values per `turn_id`.

Consumers:

- MUST ignore stale/duplicate chunks where `seq <= last_applied_seq`.
- SHOULD reorder brief out-of-order arrivals using `seq`.
- MUST ignore unknown future chunk types.

#### Chunk compatibility

`part` MUST be valid AI SDK `UIMessageChunk` JSON.

A producer MAY emit any chunk type defined by the AI SDK stream protocol, including:
`start`, `start-step`, `finish-step`, `message-metadata`, `text-*`, `reasoning-*`,
`tool-input-*`, `tool-output-*`, `tool-approval-request`, `source-url`, `source-document`,
`file`, `finish`, `abort`, `error`, and `data-*` chunks.

#### Bridge-specific `data-*` chunks

The profile additionally defines these `data-*` chunks used for UI coordination:

- `data-tool-call-event` (non-transient)
  - `id = "tool-call-event:<toolCallId>"`
  - `data.toolCallId`, `data.callEventId`
- `data-image_generation_partial` (transient)
  - `data.item_id`, `data.index`, `data.image_b64`
- `data-annotation` (transient)
  - `data.annotation`, `data.index`
- `data-tool-progress` (transient, optional)
  - `data.call_id`, `data.tool_name`, `data.status`, `data.progress`

Clients that do not understand these MUST ignore them.

### Tool call timeline projection

`com.beeper.ai.tool_call` is a timeline event for tool invocation visibility.

Event content MUST include Matrix fallback:

- `msgtype: "m.notice"`
- `body: string`

And MUST include `com.beeper.ai.tool_call`:

- `call_id: string`
- `turn_id: string`
- `tool_name: string`
- `tool_type: "builtin" | "provider" | "function" | "mcp"`
- `status: "pending" | "running" | "completed" | "failed" | "timeout" | "cancelled" | "approval_required"`

Optional fields:

- `agent_id`
- `input`
- `display` (`title`, `icon`, `collapsed`)
- `timing` (`started_at`, `first_token_at`, `completed_at`)
- `result_event`
- `mcp_server`
- `requires_approval`
- `approval` (`reason`, `actions`)
- `approval_id`
- `approval_expires_at_ms`

When a target assistant placeholder exists, senders SHOULD set
`m.relates_to = {"rel_type":"m.reference","event_id":<placeholder>}`.

Example:

```json
{
  "type": "com.beeper.ai.tool_call",
  "content": {
    "msgtype": "m.notice",
    "body": "Calling Web Search...",
    "com.beeper.ai.tool_call": {
      "call_id": "call_1",
      "turn_id": "turn_abc",
      "tool_name": "web_search",
      "tool_type": "provider",
      "status": "running",
      "timing": {"started_at": 1738970000000}
    }
  }
}
```

### Tool result timeline projection

`com.beeper.ai.tool_result` is a timeline event for tool outputs.

Event content MUST include Matrix fallback:

- `msgtype: "m.notice"`
- `body: string`

And MUST include `com.beeper.ai.tool_result`:

- `call_id: string`
- `turn_id: string`
- `tool_name: string`
- `status: "success" | "error" | "partial" | "denied"`

Optional fields:

- `agent_id`
- `output`
- `artifacts[]` (`type`, `mxc_uri`, `filename`, `mimetype`, `size`)
- `display` (`format`, `expandable`, `default_expanded`, `show_stdout`, `show_artifacts`)

Tool result events SHOULD reference their corresponding tool call event via
`m.relates_to` with `m.reference`.

### Compaction status timeline event

`com.beeper.ai.compaction_status` tracks context compaction/retry lifecycle.

Fields:

- `type: "compaction_start" | "compaction_end"` (required)
- `session_id` (optional)
- `messages_before`, `messages_after` (optional)
- `tokens_before`, `tokens_after` (optional)
- `summary` (optional)
- `will_retry` (optional)
- `error` (optional)
- `duration_ms` (optional)

Example:

```json
{
  "type": "compaction_end",
  "session_id": "!room:example.org",
  "messages_before": 50,
  "messages_after": 20,
  "tokens_before": 80000,
  "tokens_after": 30000,
  "will_retry": true
}
```

### Room state events

#### `com.beeper.ai.room_capabilities`

Bridge-controlled state. Typical power level requirement is bridge/bot only.

Fields:

- `capabilities`
- `available_tools`
- `reasoning_effort_options`
- `provider`
- `effective_settings`

`effective_settings` SHOULD include values and source attribution (`room_override`, `user_default`,
`provider_config`, etc.) to explain resolved behavior.

#### `com.beeper.ai.room_settings`

User-editable state. Typical power level requirement allows normal room users.

Fields:

- `model`
- `system_prompt`
- `temperature`
- `max_context_messages`
- `max_completion_tokens`
- `reasoning_effort`
- `conversation_mode` (`messages` or `responses`)
- `agent_id`
- `emit_thinking`
- `emit_tool_args`

Profile behavior for partial updates:

- Producers MAY treat absent/empty fields as "no change".
- This MSC does not define explicit clearing/unset semantics.

In encrypted-room deployments using wrapper events, implementations MAY transport this payload via
`com.beeper.send_state` wrappers and apply equivalent validation.

#### `com.beeper.ai.model_capabilities` and `com.beeper.ai.agents`

These state event schemas are defined for interoperability and MAY be emitted:

- `com.beeper.ai.model_capabilities`: `available_models[]`
- `com.beeper.ai.agents`: `agents[]`, optional orchestration config

### Tool approvals

Tool approvals gate selected privileged operations (notably MCP tools and configurable builtin tools).

#### Approval request emission

When approval is required, producers SHOULD emit all of:

1. Ephemeral stream chunk:
   - `part.type = "tool-approval-request"`
   - `approvalId`, `toolCallId`
2. Timeline `com.beeper.ai.tool_call` event with `status = "approval_required"` and approval metadata.
3. Timeline `m.room.message` fallback notice with a `com.beeper.ai` part indicating
   `dynamic-tool` state `approval-requested`.

#### Approval decision payload

Users (or clients on their behalf) can resolve approvals by sending an `m.room.message` containing:

```json
{
  "com.beeper.ai.approval_decision": {
    "approvalId": "abc123",
    "decision": "allow|always|deny",
    "reason": "optional"
  }
}
```

Command-equivalent flows (for clients without custom UI) MAY also be supported, for example:

- `!ai approve <approvalId> <allow|always|deny> [reason]`

Semantics:

- `allow`: approve once.
- `always`: approve and persist allow-rule for matching tool identity.
- `deny`: reject.

#### Authorization and timeout

- Only the room owner/login owner MUST be permitted to resolve approvals for that login.
- Approvals MUST be room-scoped (approval IDs cannot be resolved across rooms).
- Pending approvals MUST expire after configured TTL.
- On timeout/expiry/cancel, producers SHOULD emit terminal tool output state (`tool-output-denied`
  or equivalent timeline snapshot update) so UI does not remain actionable.

### Auxiliary keys and metadata

#### Routing and display hints on messages/member events

The following keys MAY appear where relevant:

- `com.beeper.ai.model_id`
- `com.beeper.ai.agent`

These are hints and MUST NOT be treated as sole authorization or identity source of truth.

#### Generated media metadata

AI-generated media messages MAY include:

- `com.beeper.ai.image_generation`: at least `turn_id`
- `com.beeper.ai.tts`: at least `turn_id`

Implementations MAY extend these objects with additional fields (e.g. model, revised prompt, style,
quality) while preserving compatibility.

#### Unstable HTTP namespace

Implementations MAY expose AI-related helper APIs under:

- `/_matrix/client/unstable/com.beeper.ai/...`

This MSC does not define endpoint semantics beyond namespace reservation.

## Backwards compatibility

- Clients that do not understand `com.beeper.ai.*` events should still render fallback `body` values
  from `m.room.message`/`m.notice` where present.
- Because `com.beeper.ai.stream_event` is ephemeral, non-supporting homeservers/clients may drop it;
  timeline fallback messages are therefore required for critical actions like approvals.
- Unknown fields and unknown `data-*` chunk types MUST be ignored.

## Potential issues

- Ephemeral drops can cause incomplete live rendering until final `m.replace` arrives.
- Out-of-order stream chunks require buffering/reordering logic in clients.
- Partial-update room settings without explicit clear semantics can be ambiguous for UX.
- Approval UX can diverge if a client renders only timeline or only ephemeral updates.

## Security considerations

- Approval resolution must enforce owner-only authorization and room scoping.
- Persisted "always allow" rules should be stored per login and be auditable.
- Clients should treat all tool outputs as untrusted data and render safely.
- If `formatted_body` or rich text is used, standard Matrix sanitization rules apply.
- Routing hint keys (`com.beeper.ai.model_id`, `com.beeper.ai.agent`) are metadata only and not
  proof of privilege.

## Alternatives considered

### Reusing only `m.room.message` without custom stream events

Rejected: this loses low-latency streaming fidelity and tool lifecycle granularity.

### Stable `m.ai.*` namespace immediately

Deferred: current ecosystem usage is implementation-specific; vendor prefix allows iteration before
cross-vendor convergence.

### Server-calculated tool aggregation only

Rejected for this profile: sender-projected timeline events are simpler to deploy and mirror current
implementation behavior.

## Unstable prefixes

This proposal uses unstable/vendor-prefixed identifiers:

- Event types: `com.beeper.ai.*`
- Content keys: `com.beeper.ai*`
- Unstable API namespace: `/_matrix/client/unstable/com.beeper.ai`

A future MSC may define stable `m.ai.*` equivalents and migration rules.

## Dependencies

This proposal depends conceptually on:

- Matrix event relations and `m.replace` edit semantics.
- Matrix custom event type extensibility.
- AI SDK UI message/chunk compatibility for payload semantics.

## Implementation notes (reference profile)

Reference implementation:

- Event identifiers: `pkg/matrixevents/matrixevents.go`
- Event/state schemas: `pkg/connector/events.go`
- Stream envelope + sequencing: `pkg/connector/stream_events.go`
- Streaming UI chunk emission: `pkg/connector/streaming_ui_*.go`
- Final `m.replace` assistant turn: `pkg/connector/response_finalization.go`
- Tool projections: `pkg/connector/tool_execution.go`
- Compaction status events: `pkg/connector/response_retry.go`
- Room state broadcast/apply: `pkg/connector/chat.go`, `pkg/connector/connector.go`
- Approval lifecycle: `pkg/connector/tool_approvals*.go`, `pkg/connector/handlematrix.go`,
  `pkg/connector/commands_parity.go`

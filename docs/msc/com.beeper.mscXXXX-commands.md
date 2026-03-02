# Bot Command Descriptions for ai-bridge

**Prior art:** [MSC4391](https://github.com/matrix-org/matrix-spec-proposals/pull/4391) (bot command descriptions)

**Status:** MSC4391 already implemented in mautrix-go and gomuks. ai-bridge migration in progress.

## Summary

ai-bridge adopts MSC4391 for advertising available bot commands to clients. Instead of users memorizing `!ai status`, `!ai model`, etc., clients discover commands from state events and render them as slash commands with autocomplete and typed parameters.

## State Event

Type: `org.matrix.msc4391.command_description`

ai-bridge broadcasts one state event per command on room join:

```json
{
  "type": "org.matrix.msc4391.command_description",
  "state_key": "status",
  "content": {
    "description": "Show current session status",
    "arguments": {}
  }
}
```

```json
{
  "type": "org.matrix.msc4391.command_description",
  "state_key": "model",
  "content": {
    "description": "Get or set the AI model",
    "arguments": {
      "model_id": {
        "description": "Model identifier (e.g. gpt-4o, claude-sonnet)",
        "required": false,
        "type": "string"
      }
    }
  }
}
```

## Structured Invocation

When a client sends a command, it includes the structured field:

```json
{
  "type": "m.room.message",
  "content": {
    "msgtype": "m.text",
    "body": "!ai model gpt-4o",
    "org.matrix.msc4391.command": {
      "command": "model",
      "arguments": {
        "model_id": "gpt-4o"
      }
    }
  }
}
```

The `body` field contains the text fallback for clients without MSC4391 support.

## Command List

Commands broadcast by ai-bridge:

| Command | Description | Arguments |
|---------|-------------|-----------|
| `status` | Show current session status | — |
| `model` | Get or set the AI model | `model_id?: string` |
| `reset` | Start a new session/thread | — |
| `stop` | Abort current run and clear queue | — |
| `think` | Get or set thinking level | `level?: off\|minimal\|low\|medium\|high\|xhigh` |
| `verbose` | Get or set verbosity | `level?: off\|on\|full` |
| `reasoning` | Get or set reasoning visibility | `level?: off\|on\|low\|medium\|high\|xhigh` |
| `elevated` | Get or set elevated access | `level?: off\|on\|ask\|full` |
| `activation` | Set group activation policy | `policy: mention\|always` |
| `send` | Allow/deny sending messages | `mode: on\|off\|inherit` |
| `queue` | Inspect or configure message queue | `action?: status\|reset\|<mode>` |
| `whoami` | Show your Matrix user ID | — |
| `last-heartbeat` | Show last heartbeat event | — |

Dynamic commands from integrations/modules are also broadcast.

## Implementation

### ai-bridge (`command_registry.go`)

`BroadcastCommandDescriptions()`:
1. Iterates `aiCommandRegistry.All()`
2. Maps each `commandregistry.Definition` to an MSC4391 command description
3. Sends `org.matrix.msc4391.command_description` state events via the bot intent
4. Called on room join and when commands change dynamically

### Text Fallback

The `!ai` text prefix parsing is kept as a fallback for clients without MSC4391 support. When `org.matrix.msc4391.command` is present in the message, it takes precedence over text parsing.

### mautrix-go

Already has:
- `StateMSC4391BotCommand` event type (`event/type.go:208`)
- `MSC4391BotCommandInput` struct in `MessageEventContent` (`event/message.go:147`)
- gomuks renders slash commands from these state events

# PLAN

This plan reconstructs the requested work and tracks status.

## Part 1: OpenClaw Reference + Workspace Bootstrap
- [x] Clone OpenClaw for reference (`/Users/batuhan/Projects/texts/openclaw`).
- [x] Embed workspace templates (AGENTS.md, SOUL.md, TOOLS.md, IDENTITY.md, USER.md, HEARTBEAT.md, BOOTSTRAP.md).
- [x] Add template loader with frontmatter stripping.
- [x] Ensure bootstrap files in textfs and load + trim for prompt injection.
- [x] Add SOUL_Evil swap logic and config hooks.
- [x] Document new agent defaults in example config (bootstrap max chars, soul evil).

## Part 2: Prompt Builder Alignment
- [x] Extend `SystemPromptParams` with workspace/time/tool/runtime fields.
- [x] Implement heartbeat/silent reply token parsing.
- [x] Inject bootstrap context into prompts with max-char trimming and SoulEvil override.
- [x] Add session bootstrap greeting for /new or /reset flows.

## Part 3: Tooling Compatibility
- [x] Add OpenRouter tool alias (`better_web_search`) and normalization.
- [x] Normalize tool names in streaming + continuation handling.
- [x] Improve duplicate-tool fallback/logging in Responses API flow.

## Part 4: Model Contact Metadata
- [x] Add `com.beeper.ai.model_id` and identifiers in member events.
- [x] Use model names in model-switch notices.

## Part 5: Verification
- [ ] Re-run `go test ./pkg/textfs ./pkg/agents` (previous run hung).
- [ ] Run a quick `go test ./pkg/connector` smoke test if needed.

# ai-bridge

Shared AI bridge foundation plus the simple Matrix bridge binary.

## Repository scope

This repo contains:
- shared modules (`modules/contracts`, `modules/runtime`, `modules/core`, `modules/aiproxy`, `modules/simple`)
- shared runtime/tool/helper packages (`pkg/*`)
- simple bridge binary (`bridges/simple`)

Dedicated products live in sibling repos:
- `../beep` (agentic)
- `../beeper-codex` (codex-only)
- `../beeper-opencode` (opencode-only)

## Product matrix
- `ai-bridge` (this repo): simple chat bridge + shared libraries
- `beep`: agentic bridge (memory/cron/subagents/tool orchestration)
- `beeper-codex`: codex-only bridge
- `beeper-opencode`: opencode-only bridge

## Canonical specs
- Architecture/modules spec: `docs/architecture-modules.md`
- Matrix AI event contract spec: `docs/matrix-ai-matrix-spec-v1.md`

## Dependency policy

Downstream repos pin `github.com/beeper/ai-bridge` by SHA-based pseudo versions.
Local `replace` directives are only for development workspaces.

## Build

```bash
./build.sh
./build-bridges.sh
```

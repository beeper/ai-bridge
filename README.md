# ai-bridge

Shared AI bridge foundation plus the simple Matrix bridge binary.

## Repository scope

This repo contains:
- shared modules (`modules/contracts`, `modules/core`)
- shared runtime/tool/helper packages (`pkg/*`)
- simple bridge binary (`bridges/simple`)

Dedicated products live in sibling repos:
- `../beep` (agentic)
- `../beeper-codex` (codex-only)
- `../beeper-opencode` (opencode-only)

## Dependency policy

Downstream repos pin `github.com/beeper/ai-bridge` by SHA-based pseudo versions.
Local `replace` directives are only for development workspaces.

## Build

```bash
./build.sh
./build-bridges.sh
```

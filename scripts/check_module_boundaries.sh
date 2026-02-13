#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail() {
  echo "boundary check failed: $1" >&2
  exit 1
}

imports_for_module() {
  local mod_dir="$1"
  (cd "$mod_dir" && go list -f '{{join .Imports "\n"}}' ./...)
}

core_imports="$(imports_for_module modules/core)"
contracts_imports="$(imports_for_module modules/contracts)"

if echo "$contracts_imports" | rg -q '^github.com/beeper/ai-bridge/modules/core'; then
  fail "modules/contracts must not import modules/core"
fi

if echo "$core_imports" | rg -q 'github.com/(batuhan/beeper-codex|batuhan/beeper-opencode|beeper/beep)'; then
  fail "modules/core must not import dedicated runtime repos"
fi

echo "module boundary checks passed"

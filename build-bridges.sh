#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

echo "Building ai"
(cd bridges/ai && go build -o "$ROOT/ai-bridge" ./cmd/bridge)

echo "Built: ai-bridge"

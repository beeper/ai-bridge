#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

echo "Building ai-simple"
(cd bridges/simple && go build -o "$ROOT/ai-simple" ./cmd/bridge)

echo "Built: ai-simple"

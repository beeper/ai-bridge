#!/bin/bash
# Generate AI models Go file from OpenRouter API
# Usage: ./generate-models.sh --openrouter-token="YOUR_TOKEN"
#
# This script fetches model capabilities from OpenRouter and generates
# a Go file with model definitions. The generated file is checked into
# version control to provide a stable, known model list.

set -e

# Parse arguments
OPENROUTER_TOKEN=""
OUTPUT_FILE="pkg/connector/beeper_models_generated.go"

while [[ $# -gt 0 ]]; do
  case $1 in
    --openrouter-token=*)
      OPENROUTER_TOKEN="${1#*=}"
      shift
      ;;
    --output=*)
      OUTPUT_FILE="${1#*=}"
      shift
      ;;
    -h|--help)
      echo "Usage: $0 --openrouter-token=TOKEN [--output=FILE]"
      echo ""
      echo "Options:"
      echo "  --openrouter-token=TOKEN  OpenRouter API token (required)"
      echo "  --output=FILE             Output file path (default: pkg/connector/beeper_models_generated.go)"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

if [ -z "$OPENROUTER_TOKEN" ]; then
  echo "Error: --openrouter-token is required"
  echo "Usage: $0 --openrouter-token=TOKEN"
  exit 1
fi

# Change to script directory
cd "$(dirname "$0")"

# Run the generator
echo "Generating models from OpenRouter API..."
go run ./cmd/generate-models/main.go --openrouter-token="$OPENROUTER_TOKEN" --output="$OUTPUT_FILE"

echo "Generated: $OUTPUT_FILE"
echo "Don't forget to check in the generated file!"

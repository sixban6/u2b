#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

OUTPUT_PATH="$ROOT_DIR/u2b_download"
RUN_TESTS=1

usage() {
  cat <<'EOF'
Usage: ./build.sh [options]

Options:
  -o, --output <path>   Output binary path (default: ./u2b_download)
      --no-test         Skip running go test
  -h, --help            Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output)
      if [[ $# -lt 2 ]]; then
        echo "Error: missing value for $1" >&2
        exit 1
      fi
      OUTPUT_PATH="$2"
      shift 2
      ;;
    --no-test)
      RUN_TESTS=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown option $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "Error: go is not installed or not in PATH" >&2
  exit 1
fi

echo "==> Downloading dependencies"
go mod download

if [[ "$RUN_TESTS" -eq 1 ]]; then
  echo "==> Running tests"
  go test ./...
fi

echo "==> Building binary"
go build -o "$OUTPUT_PATH" .

echo "==> Build success: $OUTPUT_PATH"

#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

OUTPUT_PATH="$ROOT_DIR/u2b_download"
RUN_TESTS=1
BUILD_ALL=0

usage() {
  cat <<'EOF'
Usage: ./build.sh [options]

Options:
  -o, --output <path>   Output binary path (default: ./u2b_download)
      --all             Build and package for all platforms into dist/
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
    --all)
      BUILD_ALL=1
      shift
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

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo "v1.0.0")}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "none")}"
DATE="${DATE:-$(date -u +'%Y-%m-%dT%H:%M:%SZ')}"

LDFLAGS="-s -w -X git.sr.ht/~sixban/u2b_download/cmd.Version=${VERSION} -X git.sr.ht/~sixban/u2b_download/cmd.GitCommit=${COMMIT} -X git.sr.ht/~sixban/u2b_download/cmd.BuildDate=${DATE}"

if [[ "$BUILD_ALL" -eq 1 ]]; then
  echo "==> Building all release packages into ./dist"
  mkdir -p dist
  rm -rf dist/*

  TARGETS=(
    "darwin/amd64/u2b/tar.gz"
    "darwin/arm64/u2b/tar.gz"
    "linux/amd64/u2b/tar.gz"
    "linux/arm64/u2b/tar.gz"
    "windows/amd64/u2b.exe/zip"
    "windows/arm64/u2b.exe/zip"
  )

  for TARGET in "${TARGETS[@]}"; do
    IFS="/" read -r OS ARCH BIN_NAME PKG_TYPE <<< "$TARGET"
    echo "==> Building for ${OS}/${ARCH}..."
    BUILD_DIR="build_${OS}_${ARCH}"
    mkdir -p "$BUILD_DIR"
    
    CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -ldflags "$LDFLAGS" -trimpath -o "${BUILD_DIR}/${BIN_NAME}" .
    cp README.md LICENSE "${BUILD_DIR}/"

    ARCHIVE_NAME="u2b-${OS}-${ARCH}"
    if [ "$PKG_TYPE" = "zip" ]; then
      (cd "$BUILD_DIR" && zip -9 -q -r "../dist/${ARCHIVE_NAME}.zip" .)
    else
      tar -czf "dist/${ARCHIVE_NAME}.tar.gz" -C "$BUILD_DIR" .
    fi
    rm -rf "$BUILD_DIR"
  done

  (
    cd dist
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum * > checksums.txt
    elif command -v shasum >/dev/null 2>&1; then
      shasum -a 256 * > checksums.txt
    fi
    echo "==> Created packages in dist/:"
    ls -lh
  )
  exit 0
fi

echo "==> Building binary ($OUTPUT_PATH)"
go build -ldflags "$LDFLAGS" -trimpath -o "$OUTPUT_PATH" .
echo "==> Build success: $OUTPUT_PATH"

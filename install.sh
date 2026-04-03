#!/usr/bin/env bash

set -euo pipefail

REPO_MODULE="git.sr.ht/~sixban/u2b_download"

log() {
  printf "==> %s\n" "$*"
}

warn() {
  printf "Warning: %s\n" "$*" >&2
}

die() {
  printf "Error: %s\n" "$*" >&2
  exit 1
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

detect_os() {
  local kernel
  kernel="$(uname -s 2>/dev/null || echo unknown)"
  case "$kernel" in
    Darwin*)
      echo "darwin"
      ;;
    Linux*)
      echo "linux"
      ;;
    CYGWIN*|MINGW*|MSYS*)
      echo "windows"
      ;;
    *)
      echo "unknown"
      ;;
  esac
}

install_with_brew() {
  local pkg="$1"
  brew install "$pkg" || brew upgrade "$pkg"
}

install_ytdlp_windows() {
  if has_cmd winget; then
    winget install --id yt-dlp.yt-dlp --exact --source winget --silent --accept-package-agreements --accept-source-agreements \
      || winget upgrade --id yt-dlp.yt-dlp --exact --source winget --silent --accept-package-agreements --accept-source-agreements
    return 0
  fi

  if has_cmd choco; then
    choco install yt-dlp -y || choco upgrade yt-dlp -y
    return 0
  fi

  if has_cmd scoop; then
    scoop install yt-dlp || scoop update yt-dlp
    return 0
  fi

  return 1
}

install_ytdlp_linux() {
  if has_cmd apt-get; then
    sudo apt-get update
    sudo apt-get install -y yt-dlp
    return 0
  fi

  if has_cmd dnf; then
    sudo dnf install -y yt-dlp
    return 0
  fi

  if has_cmd yum; then
    sudo yum install -y yt-dlp
    return 0
  fi

  if has_cmd pacman; then
    sudo pacman -Sy --noconfirm yt-dlp
    return 0
  fi

  return 1
}

main() {
  local os_name
  os_name="$(detect_os)"
  log "Detected OS: ${os_name}"

  if ! has_cmd go; then
    die "Go not found in PATH. Please install Go first: https://go.dev/dl/"
  fi

  case "$os_name" in
    darwin)
      if ! has_cmd brew; then
        die "Homebrew not found. Please install Homebrew first: https://brew.sh/"
      fi
      log "Installing/upgrading yt-dlp via Homebrew"
      install_with_brew yt-dlp
      ;;
    windows)
      log "Installing/upgrading yt-dlp on Windows (winget/choco/scoop)"
      if ! install_ytdlp_windows; then
        die "No supported package manager found. Please install yt-dlp manually: https://github.com/yt-dlp/yt-dlp/wiki/Installation"
      fi
      ;;
    linux)
      log "Installing/upgrading yt-dlp on Linux"
      if ! install_ytdlp_linux; then
        warn "Could not auto-install yt-dlp. Please install it manually and rerun."
      fi
      ;;
    *)
      warn "Unknown OS; skipping yt-dlp auto-install."
      ;;
  esac

  log "Installing u2b_download via go install"
  GOPROXY="${GOPROXY:-direct}" go install "${REPO_MODULE}@main"

  log "Install complete."
  if has_cmd u2b_download; then
    log "u2b_download: $(command -v u2b_download)"
  fi
}

main "$@"

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
      if has_cmd yt-dlp; then
        log "yt-dlp already installed: $(command -v yt-dlp)"
      else
        if ! has_cmd brew; then
          die "Homebrew not found. Please install Homebrew first: https://brew.sh/"
        fi
        log "Installing yt-dlp via Homebrew"
        install_with_brew yt-dlp
      fi
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

  local gobin
  gobin="$(go env GOBIN)"
  if [[ -z "$gobin" ]]; then
    gobin="$(go env GOPATH)/bin"
  fi
  mkdir -p "$gobin"

  if [[ -f "go.mod" ]]; then
    log "Building and installing from local repository"
    go build -o "$gobin/u2b_download" .
  else
    log "Installing u2b_download via go install"
    GOPROXY="${GOPROXY:-direct}" go install "${REPO_MODULE}@main"
  fi
  ln -sf "$gobin/u2b_download" "$gobin/u2b"

  local local_bin="$HOME/.local/bin"
  if [[ -d "$local_bin" ]] || [[ ":$PATH:" == *":$local_bin:"* ]]; then
    mkdir -p "$local_bin"
    rm -f "$local_bin/u2b_download" "$local_bin/u2b"
    cp "$gobin/u2b_download" "$local_bin/u2b_download"
    chmod +x "$local_bin/u2b_download"
    ln -sf "$local_bin/u2b_download" "$local_bin/u2b"
  fi

  log "Install complete."
  if has_cmd u2b; then
    log "u2b:          $(command -v u2b)"
  fi
  if has_cmd u2b_download; then
    log "u2b_download: $(command -v u2b_download)"
  fi

  log "Deploying u2b Brave/Chrome/Edge browser extension"
  "$gobin/u2b" ext install || warn "Browser extension setup skipped"
}

main "$@"

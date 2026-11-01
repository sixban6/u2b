#!/usr/bin/env bash

set -euo pipefail

REPO_OWNER="sixban6"
REPO_NAME="u2b_download"
GITHUB_REPO="${REPO_OWNER}/${REPO_NAME}"

log() {
  printf "==> %s\n" "$*"
}

warn() {
  printf "\033[33mWarning:\033[0m %s\n" "$*" >&2
}

die() {
  printf "\033[31mError:\033[0m %s\n" "$*" >&2
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

detect_arch() {
  local m
  m="$(uname -m 2>/dev/null || echo unknown)"
  case "$m" in
    x86_64|amd64)
      echo "amd64"
      ;;
    arm64|aarch64)
      echo "arm64"
      ;;
    *)
      echo "$m"
      ;;
  esac
}

install_dependencies_mac() {
  local to_install=()
  if ! has_cmd yt-dlp; then
    to_install+=("yt-dlp")
  fi
  if ! has_cmd ffmpeg; then
    to_install+=("ffmpeg")
  fi

  if [[ ${#to_install[@]} -eq 0 ]]; then
    log "Dependencies (yt-dlp, ffmpeg) are already installed."
    return 0
  fi

  if ! has_cmd brew; then
    warn "Homebrew not found. Please install Homebrew (https://brew.sh/) or install yt-dlp and ffmpeg manually."
    return 0
  fi

  log "Installing missing dependencies via Homebrew: ${to_install[*]}"
  brew install "${to_install[@]}" || brew upgrade "${to_install[@]}"
}

install_dependencies_linux() {
  local to_install=()
  if ! has_cmd yt-dlp; then
    to_install+=("yt-dlp")
  fi
  if ! has_cmd ffmpeg; then
    to_install+=("ffmpeg")
  fi

  if [[ ${#to_install[@]} -eq 0 ]]; then
    log "Dependencies (yt-dlp, ffmpeg) are already installed."
    return 0
  fi

  log "Attempting to install missing dependencies: ${to_install[*]}"
  if has_cmd apt-get; then
    sudo apt-get update && sudo apt-get install -y "${to_install[@]}" || warn "apt-get install failed, please install manually"
  elif has_cmd dnf; then
    sudo dnf install -y "${to_install[@]}" || warn "dnf install failed, please install manually"
  elif has_cmd pacman; then
    sudo pacman -Sy --noconfirm "${to_install[@]}" || warn "pacman install failed, please install manually"
  else
    warn "No supported package manager found; please install ${to_install[*]} manually."
  fi
}

choose_install_dir() {
  if [[ -w "/usr/local/bin" ]]; then
    echo "/usr/local/bin"
    return
  fi

  local user_bin="$HOME/.local/bin"
  mkdir -p "$user_bin"
  echo "$user_bin"
}

ensure_path() {
  local dir="$1"
  if [[ ":$PATH:" == *":$dir:"* ]]; then
    return 0
  fi

  warn "Directory '$dir' is not in your PATH."
  local shell_rc=""
  if [[ -n "${ZSH_VERSION:-}" ]] || [[ "$SHELL" == *"zsh" ]]; then
    shell_rc="$HOME/.zshrc"
  elif [[ -n "${BASH_VERSION:-}" ]] || [[ "$SHELL" == *"bash" ]]; then
    shell_rc="$HOME/.bashrc"
  fi

  if [[ -n "$shell_rc" ]]; then
    log "Adding '$dir' to $shell_rc"
    printf '\n# u2b path\nexport PATH="%s:$PATH"\n' "$dir" >> "$shell_rc"
    log "Run 'source %s' or open a new terminal session to use 'u2b'." "$shell_rc"
  fi
}

download_prebuilt_binary() {
  local os_name="$1"
  local arch="$2"
  local target_dest="$3"
  local version="${4:-latest}"

  local tar_name="u2b-${os_name}-${arch}.tar.gz"
  local download_url=""

  if [[ "$version" == "latest" ]]; then
    download_url="https://github.com/${GITHUB_REPO}/releases/latest/download/${tar_name}"
  else
    download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${tar_name}"
  fi

  log "Downloading prebuilt release from: ${download_url}"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  local tmp_archive="${tmp_dir}/${tar_name}"

  local curl_opts=(-fSL --retry 2)
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    curl_opts+=(-H "Authorization: token ${GITHUB_TOKEN}")
  fi

  if ! curl "${curl_opts[@]}" "$download_url" -o "$tmp_archive" 2>/dev/null; then
    rm -rf "$tmp_dir"
    return 1
  fi

  tar -xzf "$tmp_archive" -C "$tmp_dir"
  if [[ ! -f "$tmp_dir/u2b" ]]; then
    rm -rf "$tmp_dir"
    return 1
  fi

  cp "$tmp_dir/u2b" "$target_dest"
  chmod +x "$target_dest"
  rm -rf "$tmp_dir"
  return 0
}

build_from_source() {
  local target_dest="$1"

  if ! has_cmd go; then
    die "Go is not installed and prebuilt binary was not found. Please install Go (https://go.dev/dl/) or download a release."
  fi

  log "Building from source via Go..."
  local build_dir=""
  local should_cleanup=0

  if [[ -f "go.mod" ]] && grep -q "u2b_download" "go.mod"; then
    log "Using current repository source"
    go build -trimpath -ldflags "-s -w" -o "$target_dest" .
  else
    build_dir="$(mktemp -d)"
    should_cleanup=1
    log "Cloning repository for compilation..."
    git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "$build_dir"
    (cd "$build_dir" && go build -trimpath -ldflags "-s -w" -o "$target_dest" .)
    rm -rf "$build_dir"
  fi
  chmod +x "$target_dest"
}

main() {
  local force_build=0
  local version="latest"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --build)
        force_build=1
        shift
        ;;
      --version)
        version="$2"
        shift 2
        ;;
      -h|--help)
        cat <<EOF
u2b installer
Usage: ./install.sh [options]

Options:
  --version <tag>   Install specific version (default: latest)
  --build           Force compile from Go source instead of downloading release
  -h, --help        Show this help
EOF
        exit 0
        ;;
      *)
        warn "Unknown option: $1"
        shift
        ;;
    esac
  done

  local os_name arch
  os_name="$(detect_os)"
  arch="$(detect_arch)"
  log "System: ${os_name} (${arch})"

  # 1. Install dependencies
  case "$os_name" in
    darwin)
      install_dependencies_mac
      ;;
    linux)
      install_dependencies_linux
      ;;
    windows)
      die "On Windows, please run install.ps1 in PowerShell instead of install.sh"
      ;;
    *)
      warn "Unrecognized OS: $os_name. Skipping dependency auto-install."
      ;;
  esac

  # 2. Determine target binary path
  local install_dir
  install_dir="$(choose_install_dir)"
  local target_bin="${install_dir}/u2b"
  local symlink_bin="${install_dir}/u2b_download"

  # 3. Install binary (Prebuilt or Source)
  local installed=0
  if [[ "$force_build" -eq 0 ]]; then
    log "Fetching precompiled binary (${os_name}/${arch})..."
    if download_prebuilt_binary "$os_name" "$arch" "$target_bin" "$version"; then
      installed=1
      log "Precompiled binary installed successfully!"
    else
      warn "Could not fetch precompiled binary (release may not exist yet or network restricted). Falling back to source build..."
    fi
  fi

  if [[ "$installed" -eq 0 ]]; then
    build_from_source "$target_bin"
  fi

  # Symlink u2b_download -> u2b
  ln -sf "$target_bin" "$symlink_bin"
  ensure_path "$install_dir"

  log "Installation successful!"
  log "u2b path: $target_bin"
  "$target_bin" version || true

  # 4. Extension setup
  log "Deploying u2b Brave/Chrome/Edge browser extension"
  "$target_bin" ext install || warn "Browser extension setup skipped"
}

main "$@"

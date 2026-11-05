#!/usr/bin/env bash
# u2b Rapid One-Click Installer (macOS & Linux)
# Downloads the latest precompiled GitHub Release binary with mandatory SHA-256 & provenance security checks.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/sixban6/u2b/main/install.sh | bash
#   ./install.sh [--version vX.Y.Z] [--build] [--require-attestation]

set -euo pipefail

REPO_OWNER="sixban6"
REPO_NAME="u2b"
GITHUB_REPO="${REPO_OWNER}/${REPO_NAME}"

log() {
  printf "\033[36m==>\033[0m %s\n" "$*"
}

ok() {
  printf "\033[32m✓\033[0m %s\n" "$*"
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
  local kernel m
  kernel="$(uname -s 2>/dev/null || echo unknown)"
  m="$(uname -m 2>/dev/null || echo unknown)"

  # Detect Apple Silicon even when running inside a Rosetta 2 x86_64 terminal
  if [[ "$kernel" == "Darwin"* ]] && [[ "$m" == "x86_64" ]]; then
    if [[ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" == "1" ]]; then
      echo "arm64"
      return
    fi
  fi

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

validate_tag_format() {
  local tag="$1"
  [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]
}

compute_sha256() {
  local file="$1"
  if has_cmd sha256sum; then
    sha256sum "$file" | awk '{print tolower($1)}'
  elif has_cmd shasum; then
    shasum -a 256 "$file" | awk '{print tolower($1)}'
  elif has_cmd openssl; then
    openssl dgst -sha256 "$file" | awk '{print tolower($NF)}'
  else
    die "Cannot verify SHA-256 checksum: neither 'sha256sum', 'shasum', nor 'openssl' is installed."
  fi
}

secure_curl() {
  local opts=(-fsSL --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15)
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    opts+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
  fi
  curl "${opts[@]}" "$@"
}

extract_api_asset_sha256() {
  local json_file="$1"
  local asset_name="$2"

  if has_cmd jq; then
    jq -r --arg name "$asset_name" '.assets[]? | select(.name == $name) | .digest // empty' "$json_file" 2>/dev/null | sed -n 's/^sha256:\([0-9a-fA-F]\{64\}\)$/\1/p' | tr '[:upper:]' '[:lower:]' | head -n 1 || true
    return
  fi

  if has_cmd python3; then
    python3 - "$json_file" "$asset_name" 2>/dev/null <<'PY' || true
import json, sys
with open(sys.argv[1], "r", encoding="utf-8") as f:
    data = json.load(f)
for a in data.get("assets") or []:
    if a.get("name") == sys.argv[2]:
        d = (a.get("digest") or "").strip().lower()
        if d.startswith("sha256:") and len(d) == 71:
            print(d.split(":", 1)[1])
        break
PY
    return
  fi
}

# Resolves target release tag and optional GitHub API asset SHA-256 digest.
# Sets globals: RESOLVED_TAG, API_ASSET_SHA256
resolve_github_release() {
  local requested_version="$1"
  local asset_name="$2"
  local tmp_dir="$3"

  RESOLVED_TAG=""
  API_ASSET_SHA256=""

  local api_url=""
  if [[ "$requested_version" == "latest" ]]; then
    api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
  else
    if [[ "$requested_version" != v* ]]; then
      requested_version="v${requested_version}"
    fi
    validate_tag_format "$requested_version" || die "Invalid version tag format: '${requested_version}' (expected vX.Y.Z)"
    api_url="https://api.github.com/repos/${GITHUB_REPO}/releases/tags/${requested_version}"
  fi

  local api_json="${tmp_dir}/release.json"
  if secure_curl -H "Accept: application/vnd.github+json" -H "User-Agent: u2b-installer" "$api_url" -o "$api_json" 2>/dev/null; then
    RESOLVED_TAG="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$api_json" | head -n 1)"
    API_ASSET_SHA256="$(extract_api_asset_sha256 "$api_json" "$asset_name")"
  fi

  # Fallback if GitHub REST API rate-limit (60/hr unauthenticated) was hit
  if [[ -z "$RESOLVED_TAG" ]]; then
    if [[ "$requested_version" == "latest" ]]; then
      local effective_url=""
      effective_url="$(secure_curl -I -o /dev/null -w '%{url_effective}' "https://github.com/${GITHUB_REPO}/releases/latest" 2>/dev/null || true)"
      if [[ "$effective_url" == *"/releases/tag/"* ]]; then
        RESOLVED_TAG="${effective_url##*/releases/tag/}"
        RESOLVED_TAG="${RESOLVED_TAG%%\?*}"
        RESOLVED_TAG="${RESOLVED_TAG%%#*}"
      fi
    else
      RESOLVED_TAG="$requested_version"
    fi
  fi

  [[ -n "$RESOLVED_TAG" ]] || die "Could not resolve latest release from https://github.com/${GITHUB_REPO}/releases. Please check your network connection."
  validate_tag_format "$RESOLVED_TAG" || die "Resolved release tag '${RESOLVED_TAG}' failed security format validation."
}

verify_tar_entries_safe() {
  local archive="$1"
  local entries entry
  entries="$(tar -tzf "$archive")" || die "Failed to inspect tar archive '${archive}' (corrupted archive)"
  while IFS= read -r entry; do
    if [[ -z "$entry" ]]; then
      continue
    fi
    if [[ "$entry" == /* ]] || [[ "$entry" == *".."* ]]; then
      die "SECURITY ALERT: Unsafe archive entry path detected in tarball: '${entry}'"
    fi
  done <<EOF
$entries
EOF
}

download_and_install_release() {
  local os_name="$1"
  local arch="$2"
  local target_dest="$3"
  local requested_version="$4"
  local skip_verify="$5"
  local require_attestation="$6"

  case "${os_name}/${arch}" in
    darwin/arm64|darwin/amd64|linux/amd64|linux/arm64)
      ;;
    *)
      die "No prebuilt binary available for ${os_name}/${arch}. Run with --build to compile from Go source."
      ;;
  esac

  local tar_name="u2b-${os_name}-${arch}.tar.gz"
  local tmp_dir
  tmp_dir="$(mktemp -d)"
  # Ensure temporary files are always cleaned up
  trap 'rm -rf -- "'"$tmp_dir"'"' EXIT

  log "Querying GitHub Releases (${GITHUB_REPO})..."
  resolve_github_release "$requested_version" "$tar_name" "$tmp_dir"
  log "Target release version: ${RESOLVED_TAG} (${os_name}/${arch})"

  local release_base="https://github.com/${GITHUB_REPO}/releases/download/${RESOLVED_TAG}"
  local archive_url="${release_base}/${tar_name}"
  local checksums_url="${release_base}/checksums.txt"
  local tmp_archive="${tmp_dir}/${tar_name}"
  local tmp_checksums="${tmp_dir}/checksums.txt"

  log "Downloading ${archive_url}"
  secure_curl "$archive_url" -o "$tmp_archive" || die "Failed to download release archive: ${archive_url}"
  [[ -s "$tmp_archive" ]] || die "Downloaded archive '${tar_name}' is empty."

  if [[ "$skip_verify" -eq 1 ]]; then
    warn "Security verification skipped (--skip-verify)!"
  else
    log "Running security verification..."

    # 1. Mandatory SHA-256 verification against official checksums.txt
    secure_curl "$checksums_url" -o "$tmp_checksums" || die "Security check failed: could not download ${checksums_url}"

    local expected_sha
    expected_sha="$(awk -v f="$tar_name" '$2 == f || $2 == ("*" f) { print tolower($1); exit }' "$tmp_checksums")"
    if ! [[ "$expected_sha" =~ ^[0-9a-f]{64}$ ]]; then
      die "Security check failed: valid SHA-256 entry for '${tar_name}' not found in checksums.txt"
    fi

    # 2. Cross-check against GitHub Releases API asset digest if available
    if [[ -n "$API_ASSET_SHA256" ]] && [[ "$API_ASSET_SHA256" != "$expected_sha" ]]; then
      die "SECURITY ALERT: GitHub API asset digest (${API_ASSET_SHA256}) does not match checksums.txt (${expected_sha})!"
    fi

    local actual_sha
    actual_sha="$(compute_sha256 "$tmp_archive")"
    if [[ "$actual_sha" != "$expected_sha" ]]; then
      die "SECURITY ALERT: SHA-256 checksum mismatch for ${tar_name}! Expected ${expected_sha}, got ${actual_sha}. Installation aborted."
    fi
    ok "SHA-256 integrity verified: ${actual_sha}"

    # 3. SLSA Build Provenance Attestation verification (via GitHub CLI if available)
    if has_cmd gh && gh attestation --help >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
      local att_out=""
      if att_out="$(gh attestation verify "$tmp_archive" --repo "$GITHUB_REPO" 2>&1)"; then
        ok "SLSA build provenance verified via GitHub Attestations (${GITHUB_REPO})"
      else
        if [[ "$require_attestation" -eq 1 ]]; then
          die "SECURITY ALERT: GitHub attestation verification failed: ${att_out}"
        elif printf '%s' "$att_out" | grep -qiE "no attestations|not found|404"; then
          warn "No SLSA attestation bundle found for release ${RESOLVED_TAG} (SHA-256 verified)."
        else
          die "SECURITY ALERT: GitHub attestation verification rejected ${tar_name}: ${att_out}"
        fi
      fi
    elif [[ "$require_attestation" -eq 1 ]]; then
      die "--require-attestation was set, but authenticated GitHub CLI ('gh auth login') is not available."
    fi
  fi

  # 4. Safe archive inspection & extraction (prevents Tar-Slip path traversal)
  verify_tar_entries_safe "$tmp_archive"
  local extract_dir="${tmp_dir}/extract"
  mkdir -p "$extract_dir"
  tar -xzf "$tmp_archive" -C "$extract_dir"

  local extracted_bin="${extract_dir}/u2b"
  [[ -f "$extracted_bin" ]] || die "Archive did not contain expected 'u2b' binary."
  chmod 0755 "$extracted_bin"

  # 5. Platform signature & pre-install execution sanity check
  if [[ "$os_name" == "darwin" ]]; then
    xattr -d com.apple.quarantine "$extracted_bin" 2>/dev/null || true
    if [[ "$skip_verify" -eq 0 ]] && has_cmd codesign; then
      codesign --verify "$extracted_bin" >/dev/null 2>&1 || die "SECURITY ALERT: macOS Mach-O code signature verification failed!"
      ok "macOS Mach-O code signature verified"
    fi
  fi

  "$extracted_bin" version >/dev/null 2>&1 || die "Binary sanity check ('u2b version') failed prior to installation."

  # 6. Atomic replacement
  local staged_dest="${target_dest}.tmp.$$"
  cp "$extracted_bin" "$staged_dest"
  chmod 0755 "$staged_dest"
  mv -f "$staged_dest" "$target_dest"

  trap - EXIT
  rm -rf -- "$tmp_dir"
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
  if [[ -n "${U2B_INSTALL_DIR:-}" ]]; then
    mkdir -p "$U2B_INSTALL_DIR"
    echo "$U2B_INSTALL_DIR"
    return
  fi

  if has_cmd u2b; then
    local existing_dir
    existing_dir="$(dirname "$(command -v u2b)")"
    if [[ -d "$existing_dir" ]] && [[ -w "$existing_dir" ]]; then
      echo "$existing_dir"
      return
    fi
  fi

  if [[ -d "/usr/local/bin" ]] && [[ -w "/usr/local/bin" ]]; then
    echo "/usr/local/bin"
    return
  fi

  if [[ -d "/opt/homebrew/bin" ]] && [[ -w "/opt/homebrew/bin" ]] && [[ ":$PATH:" == *":/opt/homebrew/bin:"* ]]; then
    echo "/opt/homebrew/bin"
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

  warn "Directory '$dir' is not in your current PATH."
  local shell_rc=""
  if [[ -n "${ZSH_VERSION:-}" ]] || [[ "${SHELL:-}" == *"zsh" ]]; then
    shell_rc="$HOME/.zshrc"
  elif [[ -n "${BASH_VERSION:-}" ]] || [[ "${SHELL:-}" == *"bash" ]]; then
    shell_rc="$HOME/.bashrc"
  fi

  if [[ -n "$shell_rc" ]]; then
    if [[ ! -f "$shell_rc" ]] || ! grep -Fq "$dir" "$shell_rc" 2>/dev/null; then
      log "Adding '$dir' to $shell_rc"
      printf '\n# u2b path\nexport PATH="%s:$PATH"\n' "$dir" >> "$shell_rc"
    fi
    log "Run 'source %s' or open a new terminal session to use 'u2b'." "$shell_rc"
  fi
}

build_from_source() {
  local target_dest="$1"

  if ! has_cmd go; then
    die "Go is not installed. Remove --build to download the official precompiled binary, or install Go (https://go.dev/dl/)."
  fi

  log "Building u2b from Go source..."
  if [[ -f "go.mod" ]] && grep -qE "^module github\.com/sixban6/u2b" "go.mod"; then
    go build -trimpath -ldflags "-s -w" -o "$target_dest" .
  else
    local build_dir
    build_dir="$(mktemp -d)"
    trap 'rm -rf -- "'"$build_dir"'"' EXIT
    git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "$build_dir"
    (cd "$build_dir" && go build -trimpath -ldflags "-s -w" -o "$target_dest" .)
    trap - EXIT
    rm -rf -- "$build_dir"
  fi
  chmod 0755 "$target_dest"
}

main() {
  local force_build="${U2B_BUILD:-0}"
  local version="${U2B_VERSION:-latest}"
  local skip_verify="${U2B_SKIP_VERIFY:-0}"
  local require_attestation="${U2B_REQUIRE_ATTESTATION:-0}"
  local skip_deps="${U2B_SKIP_DEPS:-0}"
  local skip_ext="${U2B_SKIP_EXT:-0}"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --build)
        force_build=1
        shift
        ;;
      --version)
        [[ $# -ge 2 ]] || die "Missing value for --version"
        version="$2"
        shift 2
        ;;
      --skip-verify)
        skip_verify=1
        shift
        ;;
      --require-attestation)
        require_attestation=1
        shift
        ;;
      --no-deps)
        skip_deps=1
        shift
        ;;
      --no-ext)
        skip_ext=1
        shift
        ;;
      -h|--help)
        cat <<EOF
u2b installer (https://github.com/${GITHUB_REPO})
Usage: ./install.sh [options]

Options:
  --version <tag>         Install specific GitHub Release version (default: latest)
  --require-attestation   Require GitHub Actions SLSA provenance verification via 'gh'
  --skip-verify           Skip SHA-256 & provenance checks (NOT recommended)
  --build                 Compile from Go source instead of downloading prebuilt binary
  --no-deps               Skip installing yt-dlp and ffmpeg
  --no-ext                Skip browser extension registration
  -h, --help              Show this help
EOF
        exit 0
        ;;
      *)
        die "Unknown option: $1 (use --help for usage)"
        ;;
    esac
  done

  local os_name arch
  os_name="$(detect_os)"
  arch="$(detect_arch)"
  log "Detected platform: ${os_name}/${arch}"

  case "$os_name" in
    darwin)
      if [[ "$skip_deps" -eq 0 ]]; then
        install_dependencies_mac
      fi
      ;;
    linux)
      if [[ "$skip_deps" -eq 0 ]]; then
        install_dependencies_linux
      fi
      ;;
    windows)
      die "On Windows, please run install.ps1 in PowerShell: irm https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.ps1 | iex"
      ;;
    *)
      die "Unsupported OS: ${os_name}"
      ;;
  esac

  local install_dir
  install_dir="$(choose_install_dir)"
  local target_bin="${install_dir}/u2b"

  if [[ "$force_build" -eq 1 ]]; then
    build_from_source "$target_bin"
  else
    download_and_install_release "$os_name" "$arch" "$target_bin" "$version" "$skip_verify" "$require_attestation"
  fi

  ensure_path "$install_dir"

  ok "u2b installed to: $target_bin"
  "$target_bin" version || true

  if [[ "$skip_ext" -eq 0 ]]; then
    log "Deploying u2b Brave/Chrome/Edge browser extension"
    "$target_bin" ext install || warn "Browser extension setup skipped"
  fi
}

main "$@"

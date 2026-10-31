# u2b Windows installer
# Usage (PowerShell, from repo root):
#   powershell -ExecutionPolicy Bypass -File .\install.ps1
# Optional: pass -Binary <path-to-u2b.exe> to install a prebuilt binary instead of building with Go.

param(
    [string]$Binary = ""
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8

function Log($msg)  { Write-Host "==> $msg" }
function Warn($msg) { Write-Host "Warning: $msg" -ForegroundColor Yellow }

function Has-Cmd($name) { return [bool](Get-Command $name -ErrorAction SilentlyContinue) }

function Refresh-Path {
    $machine = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $user    = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machine;$user"
}

# 1. Dependencies: yt-dlp (+ ffmpeg for merging / mp3)
if (Has-Cmd "yt-dlp") {
    Log "yt-dlp already installed: $((Get-Command yt-dlp).Source)"
} elseif (Has-Cmd "winget") {
    Log "Installing yt-dlp via winget"
    winget install --id yt-dlp.yt-dlp --exact --source winget --silent --accept-package-agreements --accept-source-agreements
} elseif (Has-Cmd "scoop") {
    scoop install yt-dlp
} elseif (Has-Cmd "choco") {
    choco install yt-dlp -y
} else {
    Warn "No winget/scoop/choco found. Install yt-dlp manually: https://github.com/yt-dlp/yt-dlp/wiki/Installation"
}

Refresh-Path
if (-not (Has-Cmd "ffmpeg")) {
    if (Has-Cmd "winget") {
        Log "Installing ffmpeg via winget (required for 1080p merging and MP3)"
        winget install --id Gyan.FFmpeg --exact --source winget --silent --accept-package-agreements --accept-source-agreements
    } else {
        Warn "ffmpeg not found; 1080p merge and MP3 extraction will fail until it is installed."
    }
}
Refresh-Path

# 2. Build or copy u2b.exe into %USERPROFILE%\u2b\bin
$binDir = Join-Path $env:USERPROFILE "u2b\bin"
New-Item -ItemType Directory -Force -Path $binDir | Out-Null
$target = Join-Path $binDir "u2b.exe"

if ($Binary -ne "") {
    Log "Installing prebuilt binary: $Binary"
    Copy-Item -Force $Binary $target
} elseif ((Test-Path "go.mod") -and (Has-Cmd "go")) {
    Log "Building u2b.exe from local repository"
    go build -o $target .
} elseif (Has-Cmd "go") {
    Log "Installing via go install"
    $env:GOPROXY = if ($env:GOPROXY) { $env:GOPROXY } else { "direct" }
    go install "git.sr.ht/~sixban/u2b_download@main"
    $gobin = (go env GOBIN); if (-not $gobin) { $gobin = Join-Path (go env GOPATH) "bin" }
    Copy-Item -Force (Join-Path $gobin "u2b_download.exe") $target
} else {
    throw "Go not found. Install Go (https://go.dev/dl/) or pass -Binary <path-to-u2b.exe>."
}
Copy-Item -Force $target (Join-Path $binDir "u2b_download.exe")

# 3. Add to user PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($userPath -split ";" | Where-Object { $_ -eq $binDir })) {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$binDir", "User")
    Log "Added $binDir to user PATH (open a new terminal to use 'u2b')"
}
$env:Path = "$env:Path;$binDir"

Log "Install complete: $target"

# 4. Browser extension + Native Messaging registration
Log "Deploying u2b Brave/Chrome/Edge browser extension"
& $target ext install

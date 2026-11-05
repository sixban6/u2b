# u2b Windows 11 / Windows 10 Installer
# Usage:
#   powershell -ExecutionPolicy Bypass -File .\install.ps1
# Or one-liner remote install:
#   irm https://raw.githubusercontent.com/sixban6/u2b/main/install.ps1 | iex

param(
    [string]$Version = "latest",
    [string]$Binary = "",
    [switch]$Build = $false
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8

$RepoOwner = "sixban6"
$RepoName  = "u2b"
$GitHubRepo = "$RepoOwner/$RepoName"
$LegacyGitHubRepo = "$RepoOwner/u2b_download"

function Log($msg)  { Write-Host "==> $msg" -ForegroundColor Cyan }
function Success($msg) { Write-Host "==> $msg" -ForegroundColor Green }
function Warn($msg) { Write-Host "Warning: $msg" -ForegroundColor Yellow }

function Has-Cmd($name) {
    return [bool](Get-Command $name -ErrorAction SilentlyContinue)
}

function Refresh-Path {
    $machine = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $user    = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machine;$user"
}

# 1. Detect Architecture
$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    $arch = "arm64"
}
Log "Detected OS: Windows ($arch)"

# 2. Dependencies: yt-dlp & ffmpeg
Refresh-Path
if (Has-Cmd "yt-dlp") {
    Log "yt-dlp already installed: $((Get-Command yt-dlp).Source)"
} elseif (Has-Cmd "winget") {
    Log "Installing yt-dlp via winget..."
    try {
        winget install --id yt-dlp.yt-dlp --exact --source winget --silent --accept-package-agreements --accept-source-agreements
    } catch {
        Warn "winget failed to install yt-dlp. Continuing..."
    }
} elseif (Has-Cmd "scoop") {
    scoop install yt-dlp
} elseif (Has-Cmd "choco") {
    choco install yt-dlp -y
} else {
    Warn "No package manager found. Install yt-dlp manually: https://github.com/yt-dlp/yt-dlp/wiki/Installation"
}

Refresh-Path
if (Has-Cmd "ffmpeg") {
    Log "ffmpeg already installed: $((Get-Command ffmpeg).Source)"
} elseif (Has-Cmd "winget") {
    Log "Installing ffmpeg via winget (required for 1080p merging and MP3)..."
    try {
        winget install --id Gyan.FFmpeg --exact --source winget --silent --accept-package-agreements --accept-source-agreements
    } catch {
        Warn "winget failed to install ffmpeg. Continuing..."
    }
} else {
    Warn "ffmpeg not found; 1080p merge and MP3 extraction will fail until ffmpeg is installed."
}
Refresh-Path

# 3. Setup install directory
$binDir = Join-Path $env:USERPROFILE "u2b\bin"
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
}
$target = Join-Path $binDir "u2b.exe"

$installed = $false

# 4. Binary installation
if ($Binary -ne "") {
    Log "Installing provided binary: $Binary"
    Copy-Item -Force $Binary $target
    $installed = $true
} elseif (-not $Build) {
    # Attempt downloading prebuilt release from GitHub
    $candidates = @("u2b-windows-$arch.zip")
    if ($arch -eq "arm64") {
        # Windows 11 ARM64 can seamlessly emulate x64 binaries as a fallback
        $candidates += "u2b-windows-amd64.zip"
    }

    foreach ($zipName in $candidates) {
        foreach ($repo in @($GitHubRepo, $LegacyGitHubRepo)) {
            $downloadUrl = if ($Version -eq "latest") {
                "https://github.com/$repo/releases/latest/download/$zipName"
            } else {
                "https://github.com/$repo/releases/download/$Version/$zipName"
            }

            Log "Downloading prebuilt release ($zipName)..."
            $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("u2b_" + [System.Guid]::NewGuid().ToString("N"))
            New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
            $tmpZip = Join-Path $tmpDir $zipName

            try {
                $webParams = @{
                    Uri = $downloadUrl
                    OutFile = $tmpZip
                    UseBasicParsing = $true
                }
                if ($env:GITHUB_TOKEN) {
                    $webParams.Headers = @{ Authorization = "token $env:GITHUB_TOKEN" }
                }
                [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13
                Invoke-WebRequest @webParams -ErrorAction Stop

                Expand-Archive -Path $tmpZip -DestinationPath $tmpDir -Force
                $extractedExe = Join-Path $tmpDir "u2b.exe"
                if (Test-Path $extractedExe) {
                    Copy-Item -Force $extractedExe $target
                    $installed = $true
                    Log "Precompiled binary installed successfully ($zipName)"
                    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
                    break
                }
            } catch {
                Warn "Could not download $downloadUrl ($($_.Exception.Message))"
            } finally {
                if (Test-Path $tmpDir) {
                    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
                }
            }
        }
        if ($installed) { break }
    }
}

# Fallback to Go build if not installed
if (-not $installed) {
    if (Has-Cmd "go") {
        Log "Building u2b from Go source..."
        if ((Test-Path "go.mod") -and (Select-String -Path "go.mod" -Pattern "u2b" -Quiet)) {
            go build -trimpath -ldflags "-s -w" -o $target .
            $installed = $true
        } else {
            $srcDir = Join-Path ([System.IO.Path]::GetTempPath()) ("u2b_src_" + [System.Guid]::NewGuid().ToString("N"))
            try {
                git clone --depth 1 "https://github.com/$GitHubRepo.git" $srcDir
            } catch {
                git clone --depth 1 "https://github.com/$LegacyGitHubRepo.git" $srcDir
            }
            Push-Location $srcDir
            try {
                go build -trimpath -ldflags "-s -w" -o $target .
                $installed = $true
            } finally {
                Pop-Location
                Remove-Item -Recurse -Force $srcDir -ErrorAction SilentlyContinue
            }
        }
    } else {
        throw "Failed to download prebuilt binary, and Go is not installed. Please install Go (https://go.dev/dl/) or verify internet connection to GitHub Releases."
    }
}

# 5. Add to user PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$paths = $userPath -split ";" | Where-Object { $_ -ne "" }
if (-not ($paths -contains $binDir)) {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$binDir", "User")
    Log "Added $binDir to User PATH"
}
$env:Path = "$binDir;$env:Path"

Success "Install complete: $target"
try {
    & $target version
} catch {}

# 6. Browser Extension Setup
Log "Deploying u2b Brave/Chrome/Edge browser extension..."
try {
    & $target ext install
} catch {
    Warn "Browser extension setup step skipped: $($_.Exception.Message)"
}

# u2b Rapid One-Click Installer (Windows 11 / Windows 10)
# Downloads the latest precompiled GitHub Release binary with mandatory SHA-256 & provenance security checks.
#
# One-liner remote install (PowerShell):
#   irm https://raw.githubusercontent.com/sixban6/u2b/main/install.ps1 | iex
#
# Local / parameterized usage:
#   powershell -ExecutionPolicy Bypass -File .\install.ps1 [-Version vX.Y.Z] [-Build] [-RequireAttestation]

param(
    [string]$Version = $(if ($env:U2B_VERSION) { $env:U2B_VERSION } else { "latest" }),
    [string]$Binary = "",
    [switch]$Build = $(if ($env:U2B_BUILD -eq "1") { $true } else { $false }),
    [switch]$SkipVerify = $(if ($env:U2B_SKIP_VERIFY -eq "1") { $true } else { $false }),
    [switch]$RequireAttestation = $(if ($env:U2B_REQUIRE_ATTESTATION -eq "1") { $true } else { $false }),
    [switch]$NoDeps = $(if ($env:U2B_SKIP_DEPS -eq "1") { $true } else { $false }),
    [switch]$NoExt = $(if ($env:U2B_SKIP_EXT -eq "1") { $true } else { $false })
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8

$RepoOwner  = "sixban6"
$RepoName   = "u2b"
$GitHubRepo = "$RepoOwner/$RepoName"

function Log($msg)     { Write-Host "==> $msg" -ForegroundColor Cyan }
function Ok($msg)      { Write-Host "✓ $msg" -ForegroundColor Green }
function Warn($msg)    { Write-Host "Warning: $msg" -ForegroundColor Yellow }

function Has-Cmd($name) {
    return [bool](Get-Command $name -ErrorAction SilentlyContinue)
}

function Enable-SecureTls {
    $tls = [Net.SecurityProtocolType]::Tls12
    if ([Enum]::GetNames([Net.SecurityProtocolType]) -contains "Tls13") {
        $tls = $tls -bor [Net.SecurityProtocolType]::Tls13
    }
    [Net.ServicePointManager]::SecurityProtocol = $tls
}

function Detect-WindowsArch {
    try {
        $osArch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToUpperInvariant()
        if ($osArch -eq "ARM64") { return "arm64" }
        if ($osArch -eq "X64")   { return "amd64" }
    } catch {}

    if ($env:PROCESSOR_ARCHITEW6432 -eq "ARM64" -or $env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
        return "arm64"
    }
    return "amd64"
}

function Test-ValidTag([string]$Tag) {
    return ($Tag -match '^v\d+\.\d+\.\d+([.-][0-9A-Za-z.-]+)?$')
}

function Get-GitHubHeaders {
    $headers = @{
        "User-Agent" = "u2b-installer"
        "Accept"     = "application/vnd.github+json"
    }
    if ($env:GITHUB_TOKEN) {
        $headers["Authorization"] = "Bearer $env:GITHUB_TOKEN"
    }
    return $headers
}

function Resolve-GitHubRelease([string]$RequestedVersion, [string]$AssetName) {
    $resolvedTag = ""
    $apiSha256   = ""

    $targetVer = $RequestedVersion.Trim()
    if ($targetVer -ne "latest") {
        if (-not $targetVer.StartsWith("v")) {
            $targetVer = "v$targetVer"
        }
        if (-not (Test-ValidTag $targetVer)) {
            throw "Invalid version tag format: '$targetVer' (expected vX.Y.Z)"
        }
    }

    $apiUrl = if ($targetVer -eq "latest") {
        "https://api.github.com/repos/$GitHubRepo/releases/latest"
    } else {
        "https://api.github.com/repos/$GitHubRepo/releases/tags/$targetVer"
    }

    try {
        $prevProgress = $ProgressPreference
        $ProgressPreference = "SilentlyContinue"
        try {
            $release = Invoke-RestMethod -Uri $apiUrl -Headers (Get-GitHubHeaders) -Method Get -ErrorAction Stop
            if ($release -and $release.tag_name) {
                $resolvedTag = [string]$release.tag_name
            }
            if ($release -and $release.assets) {
                foreach ($a in $release.assets) {
                    if ($a.name -eq $AssetName -and $a.digest) {
                        $digestStr = ([string]$a.digest).Trim().ToLowerInvariant()
                        if ($digestStr -match '^sha256:([0-9a-f]{64})$') {
                            $apiSha256 = $Matches[1]
                        }
                        break
                    }
                }
            }
        } finally {
            $ProgressPreference = $prevProgress
        }
    } catch {
        # Fallback if GitHub REST API rate-limit (60/hr unauthenticated) was hit
        if ($targetVer -eq "latest") {
            try {
                $req = [System.Net.HttpWebRequest]::Create("https://github.com/$GitHubRepo/releases/latest")
                $req.Method = "HEAD"
                $req.AllowAutoRedirect = $true
                $req.UserAgent = "u2b-installer"
                $resp = $req.GetResponse()
                $finalUri = $resp.ResponseUri.AbsoluteUri
                $resp.Close()
                if ($finalUri -match '/releases/tag/([^/?#]+)') {
                    $resolvedTag = $Matches[1]
                }
            } catch {}
        } else {
            $resolvedTag = $targetVer
        }
    }

    if (-not $resolvedTag) {
        throw "Could not resolve latest release from https://github.com/$GitHubRepo/releases. Please check your internet connection."
    }
    if (-not (Test-ValidTag $resolvedTag)) {
        throw "Resolved release tag '$resolvedTag' failed security format validation."
    }

    return @{
        Tag       = $resolvedTag
        ApiSha256 = $apiSha256
    }
}

function Invoke-SecureDownload([string]$Url, [string]$OutFile) {
    if (-not $Url.StartsWith("https://github.com/$GitHubRepo/releases/download/")) {
        throw "SECURITY ALERT: Refusing download from untrusted URL: $Url"
    }
    $headers = @{ "User-Agent" = "u2b-installer" }
    if ($env:GITHUB_TOKEN) {
        $headers["Authorization"] = "Bearer $env:GITHUB_TOKEN"
    }
    $prevProgress = $ProgressPreference
    $ProgressPreference = "SilentlyContinue"
    try {
        Invoke-WebRequest -Uri $Url -OutFile $OutFile -Headers $headers -UseBasicParsing -ErrorAction Stop
    } finally {
        $ProgressPreference = $prevProgress
    }
}

function Assert-SafeZipEntries([string]$ZipPath) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [System.IO.Compression.ZipFile]::OpenRead($ZipPath)
    try {
        foreach ($entry in $archive.Entries) {
            $fullName = $entry.FullName
            if ([System.IO.Path]::IsPathRooted($fullName) -or ($fullName -match '(^|[\\/])\.\.([\\/]|$)')) {
                throw "SECURITY ALERT: Unsafe path traversal entry detected in zip archive: '$fullName'"
            }
        }
    } finally {
        $archive.Dispose()
    }
}

function Install-VerifiedGitHubRelease([string]$Arch, [string]$TargetExe, [string]$RequestedVersion) {
    $zipName = "u2b-windows-$Arch.zip"
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("u2b_" + [System.Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null

    try {
        Log "Querying GitHub Releases ($GitHubRepo)..."
        $rel = Resolve-GitHubRelease -RequestedVersion $RequestedVersion -AssetName $zipName
        $tag = $rel.Tag
        $apiSha = $rel.ApiSha256
        Log "Target release version: $tag (windows/$Arch)"

        $baseUrl      = "https://github.com/$GitHubRepo/releases/download/$tag"
        $archiveUrl   = "$baseUrl/$zipName"
        $checksumsUrl = "$baseUrl/checksums.txt"
        $tmpZip       = Join-Path $tmpDir $zipName
        $tmpChecksums = Join-Path $tmpDir "checksums.txt"

        Log "Downloading $archiveUrl"
        Invoke-SecureDownload -Url $archiveUrl -OutFile $tmpZip
        if (-not (Test-Path $tmpZip) -or ((Get-Item $tmpZip).Length -eq 0)) {
            throw "Downloaded archive '$zipName' is missing or empty."
        }

        if ($SkipVerify) {
            Warn "Security verification skipped (-SkipVerify)!"
        } else {
            Log "Running security verification..."

            # 1. Mandatory SHA-256 verification against official checksums.txt
            Invoke-SecureDownload -Url $checksumsUrl -OutFile $tmpChecksums
            $expectedSha = ""
            foreach ($line in (Get-Content -Path $tmpChecksums -Encoding UTF8)) {
                $trimmed = $line.Trim()
                if ($trimmed -match '^([0-9a-fA-F]{64})\s+\*?(\S+)$') {
                    if ($Matches[2] -eq $zipName) {
                        $expectedSha = $Matches[1].ToLowerInvariant()
                        break
                    }
                }
            }
            if ($expectedSha -notmatch '^[0-9a-f]{64}$') {
                throw "Security check failed: valid SHA-256 entry for '$zipName' not found in checksums.txt"
            }

            # 2. Cross-check against GitHub Releases API asset digest if available
            if ($apiSha -and ($apiSha -ne $expectedSha)) {
                throw "SECURITY ALERT: GitHub API asset digest ($apiSha) does not match checksums.txt ($expectedSha)!"
            }

            $actualSha = (Get-FileHash -Path $tmpZip -Algorithm SHA256).Hash.ToLowerInvariant()
            if ($actualSha -ne $expectedSha) {
                throw "SECURITY ALERT: SHA-256 checksum mismatch for $zipName! Expected $expectedSha, got $actualSha. Installation aborted."
            }
            Ok "SHA-256 integrity verified: $actualSha"

            # 3. SLSA Build Provenance Attestation check (via GitHub CLI if available)
            if (Has-Cmd "gh") {
                $ghReady = $false
                try {
                    & gh attestation --help *> $null
                    if ($LASTEXITCODE -eq 0) {
                        & gh auth status *> $null
                        if ($LASTEXITCODE -eq 0) { $ghReady = $true }
                    }
                } catch {}

                if ($ghReady) {
                    $attOut = (& gh attestation verify $tmpZip --repo $GitHubRepo 2>&1 | Out-String)
                    if ($LASTEXITCODE -eq 0) {
                        Ok "SLSA build provenance verified via GitHub Attestations ($GitHubRepo)"
                    } elseif ($RequireAttestation) {
                        throw "SECURITY ALERT: GitHub attestation verification failed: $attOut"
                    } elseif ($attOut -match 'no attestations|not found|404') {
                        Warn "No SLSA attestation bundle found for release $tag (SHA-256 verified)."
                    } else {
                        throw "SECURITY ALERT: GitHub attestation verification rejected ${zipName}: $attOut"
                    }
                } elseif ($RequireAttestation) {
                    throw "-RequireAttestation was set, but authenticated GitHub CLI ('gh auth login') is not available."
                }
            } elseif ($RequireAttestation) {
                throw "-RequireAttestation was set, but GitHub CLI ('gh') is not installed."
            }
        }

        # 4. Safe zip inspection & extraction (prevents Zip-Slip path traversal)
        Assert-SafeZipEntries -ZipPath $tmpZip
        $extractDir = Join-Path $tmpDir "extract"
        New-Item -ItemType Directory -Force -Path $extractDir | Out-Null

        $prevProgress = $ProgressPreference
        $ProgressPreference = "SilentlyContinue"
        try {
            Expand-Archive -Path $tmpZip -DestinationPath $extractDir -Force
        } finally {
            $ProgressPreference = $prevProgress
        }

        $extractedExe = Join-Path $extractDir "u2b.exe"
        if (-not (Test-Path $extractedExe)) {
            throw "Archive did not contain expected 'u2b.exe' binary."
        }

        Unblock-File -Path $extractedExe -ErrorAction SilentlyContinue

        # 5. Pre-install execution sanity check
        & $extractedExe version *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "Binary sanity check ('u2b.exe version') failed prior to installation."
        }

        # 6. Safe atomic replacement (handles running u2b.exe locked by Windows)
        if (Test-Path $TargetExe) {
            $oldBackup = "$TargetExe.old"
            Remove-Item -Force $oldBackup -ErrorAction SilentlyContinue
            try {
                Copy-Item -Force $extractedExe $TargetExe -ErrorAction Stop
            } catch {
                Move-Item -Force $TargetExe $oldBackup -ErrorAction Stop
                Copy-Item -Force $extractedExe $TargetExe -ErrorAction Stop
                Remove-Item -Force $oldBackup -ErrorAction SilentlyContinue
            }
        } else {
            Copy-Item -Force $extractedExe $TargetExe -ErrorAction Stop
        }
    } finally {
        if (Test-Path $tmpDir) {
            Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
        }
    }
}

function Refresh-Path {
    $machine = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $user    = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machine;$user"
}

Enable-SecureTls

# 1. Detect Architecture
$arch = Detect-WindowsArch
Log "Detected platform: windows/$arch"

# 2. Dependencies: yt-dlp & ffmpeg
if (-not $NoDeps) {
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
}

# 3. Setup install directory
$binDir = if ($env:U2B_INSTALL_DIR) { $env:U2B_INSTALL_DIR } else { Join-Path $env:USERPROFILE "u2b\bin" }
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
}
$target = Join-Path $binDir "u2b.exe"

# 4. Binary installation
if ($Binary -ne "") {
    Log "Installing local binary: $Binary"
    Copy-Item -Force $Binary $target
} elseif ($Build) {
    if (-not (Has-Cmd "go")) {
        throw "Go is not installed. Omit -Build to download the official precompiled binary, or install Go (https://go.dev/dl/)."
    }
    Log "Building u2b from Go source..."
    if ((Test-Path "go.mod") -and (Select-String -Path "go.mod" -Pattern "^module github\.com/sixban6/u2b" -Quiet)) {
        go build -trimpath -ldflags "-s -w" -o $target .
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    } else {
        $srcDir = Join-Path ([System.IO.Path]::GetTempPath()) ("u2b_src_" + [System.Guid]::NewGuid().ToString("N"))
        try {
            git clone --depth 1 "https://github.com/$GitHubRepo.git" $srcDir
            if ($LASTEXITCODE -ne 0) { throw "git clone failed" }
            Push-Location $srcDir
            try {
                go build -trimpath -ldflags "-s -w" -o $target .
                if ($LASTEXITCODE -ne 0) { throw "go build failed" }
            } finally {
                Pop-Location
            }
        } finally {
            if (Test-Path $srcDir) {
                Remove-Item -Recurse -Force $srcDir -ErrorAction SilentlyContinue
            }
        }
    }
} else {
    Install-VerifiedGitHubRelease -Arch $arch -TargetExe $target -RequestedVersion $Version
}

# 5. Add to user PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$paths = if ($userPath) { $userPath -split ";" | Where-Object { $_ -ne "" } } else { @() }
if (-not ($paths -contains $binDir)) {
    $newUserPath = if ($userPath) { "$userPath;$binDir" } else { $binDir }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
    Log "Added $binDir to User PATH"
}
$env:Path = "$binDir;$env:Path"

Ok "u2b installed to: $target"
try {
    & $target version
} catch {}

# 6. Browser Extension Setup
if (-not $NoExt) {
    Log "Deploying u2b Brave/Chrome/Edge browser extension..."
    try {
        & $target ext install
    } catch {
        Warn "Browser extension setup step skipped: $($_.Exception.Message)"
    }
}

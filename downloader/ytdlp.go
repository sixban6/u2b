package downloader

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sixban6/u2b/config"
)

// OutdatedThresholdDays is the fallback age threshold when online version check is unavailable.
const OutdatedThresholdDays = 30

// YtdlpExecutor handles yt-dlp command execution
type YtdlpExecutor struct {
	Config           *config.Config
	ytdlpPath        string
	progressCallback ProgressCallback
	upgradeMu        sync.Mutex
	upgradeAttempted bool
}

// YtdlpOptions represents options for yt-dlp execution
type YtdlpOptions struct {
	URL              string
	Quality          string
	Format           string
	AudioQuality     string
	OutputDir        string
	OutputTemplate   string
	AuthArgs         []string
	ExtraArgs        []string
	DownloadPlaylist bool // Whether to download entire playlist
}

// NewYtdlpExecutor creates a new yt-dlp executor
func NewYtdlpExecutor(cfg *config.Config) (*YtdlpExecutor, error) {
	executor := &YtdlpExecutor{
		Config: cfg,
	}

	// Determine yt-dlp path
	ytdlpPath := cfg.Advanced.YtdlpPath
	if ytdlpPath == "" {
		// Try to find yt-dlp in PATH
		path, err := exec.LookPath("yt-dlp")
		if err != nil {
			// Try common installation paths
			commonPaths := []string{
				"/usr/local/bin/yt-dlp",
				"/usr/bin/yt-dlp",
				"/opt/homebrew/bin/yt-dlp",
				filepath.Join(os.Getenv("HOME"), ".local/bin/yt-dlp"),
			}

			for _, commonPath := range commonPaths {
				if _, err := os.Stat(commonPath); err == nil {
					ytdlpPath = commonPath
					break
				}
			}

			if ytdlpPath == "" {
				return nil, fmt.Errorf("yt-dlp not found in PATH or common locations. Please install yt-dlp or specify path in config")
			}
		} else {
			ytdlpPath = path
		}
	}

	executor.ytdlpPath = ytdlpPath

	// Verify yt-dlp is executable
	if err := executor.verifyYtdlp(); err != nil {
		return nil, fmt.Errorf("yt-dlp verification failed: %w", err)
	}

	return executor, nil
}

// verifyYtdlp verifies that yt-dlp is working
func (y *YtdlpExecutor) verifyYtdlp() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := y.GetVersion(ctx)
	return err
}

// GetVersion returns the installed yt-dlp version string (e.g. "2026.08.19")
func (y *YtdlpExecutor) GetVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, y.ytdlpPath, "--version")
	HideConsole(cmd)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run yt-dlp --version: %w", err)
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("yt-dlp returned empty version")
	}

	return version, nil
}

// ParseYtdlpVersionDate parses a yt-dlp version string (e.g. "2026.08.19" or "2026.8.19.post1") into time.Time.
func ParseYtdlpVersionDate(version string) (time.Time, error) {
	cleaned := strings.TrimSpace(version)
	if cleaned == "" {
		return time.Time{}, fmt.Errorf("empty version string")
	}

	parts := strings.Split(cleaned, ".")
	if len(parts) < 3 {
		return time.Time{}, fmt.Errorf("invalid yt-dlp version format: %s", version)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid year in version %s: %w", version, err)
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month in version %s: %w", version, err)
	}
	dayStr := parts[2]
	for i, r := range dayStr {
		if r < '0' || r > '9' {
			dayStr = dayStr[:i]
			break
		}
	}
	day, err := strconv.Atoi(dayStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day in version %s: %w", version, err)
	}

	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// CompareYtdlpVersions compares two yt-dlp version strings.
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
func CompareYtdlpVersions(v1, v2 string) int {
	t1, err1 := ParseYtdlpVersionDate(v1)
	t2, err2 := ParseYtdlpVersionDate(v2)
	if err1 == nil && err2 == nil {
		if t1.Before(t2) {
			return -1
		}
		if t1.After(t2) {
			return 1
		}
		return 0
	}

	s1 := strings.TrimSpace(v1)
	s2 := strings.TrimSpace(v2)
	if s1 < s2 {
		return -1
	}
	if s1 > s2 {
		return 1
	}
	return 0
}

// fetchLatestVersion queries GitHub Releases or Homebrew for the latest available yt-dlp version.
func (y *YtdlpExecutor) fetchLatestVersion(ctx context.Context) (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest", nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "u2b/1.0")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var release struct {
					TagName string `json:"tag_name"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&release); err == nil && strings.TrimSpace(release.TagName) != "" {
					return strings.TrimSpace(release.TagName), nil
				}
			}
		}
	}

	// Fallback: check brew info if available
	if brewPath, err := exec.LookPath("brew"); err == nil {
		cmd := exec.CommandContext(checkCtx, brewPath, "info", "yt-dlp", "--json=v2")
		if out, err := cmd.Output(); err == nil {
			var brewInfo struct {
				Formulae []struct {
					Versions struct {
						Stable string `json:"stable"`
					} `json:"versions"`
				} `json:"formulae"`
			}
			if err := json.Unmarshal(out, &brewInfo); err == nil && len(brewInfo.Formulae) > 0 {
				stable := strings.TrimSpace(brewInfo.Formulae[0].Versions.Stable)
				if stable != "" {
					return stable, nil
				}
			}
		}
	}

	return "", fmt.Errorf("unable to query latest yt-dlp version")
}

// IsVersionOutdated checks whether the given local yt-dlp version is outdated compared to the latest release or age threshold.
func (y *YtdlpExecutor) IsVersionOutdated(ctx context.Context, localVersion string) (outdated bool, latestVersion string, reason string) {
	latest, err := y.fetchLatestVersion(ctx)
	if err == nil && latest != "" {
		if CompareYtdlpVersions(localVersion, latest) < 0 {
			return true, latest, fmt.Sprintf("local version %s is older than latest release %s", localVersion, latest)
		}
		return false, latest, fmt.Sprintf("local version %s is up to date (latest: %s)", localVersion, latest)
	}

	// Offline/fallback check: compare local version date against current date
	if vDate, parseErr := ParseYtdlpVersionDate(localVersion); parseErr == nil {
		ageDays := int(time.Since(vDate).Hours() / 24)
		if ageDays > OutdatedThresholdDays {
			return true, "", fmt.Sprintf("local version %s is %d days old (> %d days)", localVersion, ageDays, OutdatedThresholdDays)
		}
		return false, "", fmt.Sprintf("local version %s is recent (%d days old)", localVersion, ageDays)
	}

	// If version cannot be parsed or checked, allow attempting an upgrade as fallback
	return true, "", fmt.Sprintf("unable to verify age of local version %s", localVersion)
}

// TryUpgradeIfOutdated checks if local yt-dlp is outdated upon download failure and upgrades it once as a fallback.
func (y *YtdlpExecutor) TryUpgradeIfOutdated(ctx context.Context) (bool, error) {
	y.upgradeMu.Lock()
	defer y.upgradeMu.Unlock()

	if y.upgradeAttempted {
		return false, nil
	}
	y.upgradeAttempted = true

	fmt.Fprintf(os.Stderr, "\n⚠️  Download failed. Checking local yt-dlp version for fallback upgrade...\n")

	localVersion, err := y.GetVersion(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Could not determine local yt-dlp version: %v\n", err)
		return false, err
	}

	outdated, latestVersion, reason := y.IsVersionOutdated(ctx, localVersion)
	if !outdated {
		fmt.Fprintf(os.Stderr, "ℹ️  yt-dlp check: %s. Skipping upgrade.\n", reason)
		return false, nil
	}

	fmt.Fprintf(os.Stderr, "🔄 yt-dlp check: %s. Upgrading yt-dlp...\n", reason)

	if err := y.upgradeYtdlpBinary(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "❌ yt-dlp fallback upgrade failed: %v\n", err)
		return false, err
	}

	newVersion, verErr := y.GetVersion(ctx)
	if verErr == nil && newVersion != "" {
		if newVersion != localVersion {
			fmt.Fprintf(os.Stderr, "✅ yt-dlp upgraded successfully: %s -> %s\n\n", localVersion, newVersion)
			return true, nil
		}
		if latestVersion != "" && CompareYtdlpVersions(newVersion, latestVersion) >= 0 {
			fmt.Fprintf(os.Stderr, "✅ yt-dlp is now at version %s\n\n", newVersion)
			return true, nil
		}
	}

	fmt.Fprintf(os.Stderr, "ℹ️  yt-dlp upgrade finished (current version: %s)\n\n", newVersion)
	return newVersion != "" && newVersion != localVersion, nil
}

// upgradeYtdlpBinary runs the appropriate upgrade command for the current platform/installation.
func (y *YtdlpExecutor) upgradeYtdlpBinary(ctx context.Context) error {
	isHomebrew := strings.Contains(y.ytdlpPath, "homebrew") || strings.Contains(y.ytdlpPath, "Cellar")

	if isHomebrew || runtime.GOOS == "darwin" {
		if brewPath, err := exec.LookPath("brew"); err == nil {
			cmd := exec.CommandContext(ctx, brewPath, "upgrade", "yt-dlp")
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}

	// Try yt-dlp -U (works for standalone release binaries)
	selfUpdateCmd := exec.CommandContext(ctx, y.ytdlpPath, "-U")
	HideConsole(selfUpdateCmd)
	if out, err := selfUpdateCmd.CombinedOutput(); err == nil {
		outStr := string(out)
		if !strings.Contains(strings.ToLower(outStr), "installed yt-dlp with") &&
			!strings.Contains(strings.ToLower(outStr), "cannot update") {
			return nil
		}
	}

	// Platform-specific package manager fallbacks
	switch runtime.GOOS {
	case "darwin", "linux":
		if brewPath, err := exec.LookPath("brew"); err == nil {
			cmd := exec.CommandContext(ctx, brewPath, "upgrade", "yt-dlp")
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		if pipxPath, err := exec.LookPath("pipx"); err == nil {
			cmd := exec.CommandContext(ctx, pipxPath, "upgrade", "yt-dlp")
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		if pipPath, err := exec.LookPath("pip3"); err == nil {
			cmd := exec.CommandContext(ctx, pipPath, "install", "-U", "yt-dlp")
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	case "windows":
		if wingetPath, err := exec.LookPath("winget"); err == nil {
			cmd := exec.CommandContext(ctx, wingetPath, "upgrade", "--id", "yt-dlp.yt-dlp", "--exact", "--source", "winget", "--silent", "--accept-package-agreements", "--accept-source-agreements")
			HideConsole(cmd)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		if chocoPath, err := exec.LookPath("choco"); err == nil {
			cmd := exec.CommandContext(ctx, chocoPath, "upgrade", "yt-dlp", "-y")
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		if scoopPath, err := exec.LookPath("scoop"); err == nil {
			cmd := exec.CommandContext(ctx, scoopPath, "update", "yt-dlp")
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}

	return fmt.Errorf("all yt-dlp upgrade methods failed")
}

// Download performs the actual download
func (y *YtdlpExecutor) Download(ctx context.Context, options *YtdlpOptions) (*DownloadResult, error) {
	startTime := time.Now()

	// Build command arguments
	args := y.BuildDownloadArgs(options)

	// Create command (wrapped in macOS Seatbelt sandbox when on Darwin)
	noSandbox := y.Config != nil && y.Config.Advanced.NoSandbox
	cmd := WrapCommandInSandbox(
		ctx,
		y.ytdlpPath,
		args,
		options.OutputDir,
		ExtractCookieFilesFromArgs(options.AuthArgs),
		noSandbox,
	)
	HideConsole(cmd)

	// Set up output capture for progress monitoring
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	// Monitor progress
	result := &DownloadResult{Success: false}
	var errorLines []string
	var outputLines []string
	var wg sync.WaitGroup
	wg.Add(2)

	// Read stderr for errors and progress
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			errorLines = append(errorLines, line)

			// Parse progress from stderr if callback is set
			if y.progressCallback != nil {
				progress := y.parseProgress(line)
				if progress != nil {
					y.progressCallback(progress)
				}
			}
		}
	}()

	// Read stdout for output and progress
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputLines = append(outputLines, line)

			if y.progressCallback != nil {
				progress := y.parseProgress(line)
				if progress != nil {
					y.progressCallback(progress)
				}
			}
		}
	}()

	// Wait for pipes to finish reading before cmd.Wait()
	wg.Wait()

	// Wait for completion
	err = cmd.Wait()
	downloadTime := time.Since(startTime).Seconds()
	result.DownloadTime = downloadTime
	errorOutput := strings.Join(errorLines, "\n")

	if err != nil {
		result.Error = fmt.Sprintf("yt-dlp failed: %v\nOutput: %s", err, errorOutput)
		trimmedErr := strings.TrimSpace(errorOutput)
		if trimmedErr == "" {
			return result, fmt.Errorf("download failed: %w", err)
		}
		return result, fmt.Errorf("download failed: %w: %s", err, trimmedErr)
	}

	// Parse result from output (combine stdout and stderr lines so Merger messages are always caught)
	allLines := append([]string{}, outputLines...)
	allLines = append(allLines, errorLines...)
	if err := y.ParseDownloadResult(result, allLines, options); err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Success = true
	return result, nil
}

// GetPlaylistInfo retrieves playlist information
func (y *YtdlpExecutor) GetPlaylistInfo(ctx context.Context, url string, authArgs []string) (*config.VideoInfo, error) {
	args := []string{
		"--dump-json",
		"--no-download",
		"--no-warnings",
		"--flat-playlist", // Get playlist info only
	}

	// Add authentication arguments
	if len(authArgs) > 0 {
		args = append(args, authArgs...)
	}

	// Add YouTube-specific arguments
	args = append(args, "--extractor-args", fmt.Sprintf("youtube:client=%s", y.Config.YouTube.Client), "--", url)

	noSandbox := y.Config != nil && y.Config.Advanced.NoSandbox
	cmd := WrapCommandInSandbox(ctx, y.ytdlpPath, args, "", ExtractCookieFilesFromArgs(authArgs), noSandbox)
	HideConsole(cmd)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	output, err := cmd.Output()
	if err != nil {
		errDetail := strings.TrimSpace(stderrBuf.String())
		if errDetail == "" {
			errDetail = strings.TrimSpace(string(output))
		}
		return nil, fmt.Errorf("failed to get playlist info: %w: %s", err, errDetail)
	}

	// Parse JSON output - handle multiple JSON objects
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var playlistInfo config.VideoInfo

	// Use the first/main JSON object which should be the playlist info
	if len(lines) > 0 {
		if err := json.Unmarshal([]byte(lines[0]), &playlistInfo); err != nil {
			return nil, fmt.Errorf("failed to parse playlist info JSON: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no playlist info returned")
	}

	return &playlistInfo, nil
}

// GetVideoInfo retrieves video information
func (y *YtdlpExecutor) GetVideoInfo(ctx context.Context, url string, authArgs []string) (*config.VideoInfo, error) {
	args := []string{
		"--dump-json",
		"--no-download",
		"--no-warnings",
		"--no-playlist", // Get info for single video only
	}

	// Add authentication arguments
	if len(authArgs) > 0 {
		args = append(args, authArgs...)
	}

	// Add YouTube-specific arguments
	args = append(args, "--extractor-args", fmt.Sprintf("youtube:client=%s", y.Config.YouTube.Client), "--", url)

	noSandbox := y.Config != nil && y.Config.Advanced.NoSandbox
	cmd := WrapCommandInSandbox(ctx, y.ytdlpPath, args, "", ExtractCookieFilesFromArgs(authArgs), noSandbox)
	HideConsole(cmd)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	output, err := cmd.Output()
	if err != nil {
		errDetail := strings.TrimSpace(stderrBuf.String())
		if errDetail == "" {
			errDetail = strings.TrimSpace(string(output))
		}
		return nil, fmt.Errorf("failed to get video info: %w: %s", err, errDetail)
	}

	// Parse JSON output - handle multiple JSON objects
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, fmt.Errorf("no video info returned for URL %q", url)
	}
	lines := strings.Split(trimmed, "\n")
	var videoInfo config.VideoInfo

	// Use the first/main JSON object
	if err := json.Unmarshal([]byte(lines[0]), &videoInfo); err != nil {
		return nil, fmt.Errorf("failed to parse video info JSON: %w", err)
	}

	// Extract available qualities from formats
	videoInfo.AvailableQualities = y.extractQualities(videoInfo.Formats)

	return &videoInfo, nil
}

// FindFFmpegLocation searches for ffmpeg executable and returns its directory.
func FindFFmpegLocation() string {
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		if abs, err := filepath.Abs(path); err == nil {
			return filepath.Dir(abs)
		}
	}

	homeDir, _ := os.UserHomeDir()
	var candidates []string

	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			packagesDir := filepath.Join(localAppData, "Microsoft", "WinGet", "Packages")
			if entries, err := os.ReadDir(packagesDir); err == nil {
				for _, entry := range entries {
					if entry.IsDir() && strings.Contains(strings.ToLower(entry.Name()), "ffmpeg") {
						base := filepath.Join(packagesDir, entry.Name())
						candidates = append(candidates, filepath.Join(base, "bin"))
						if subs, err := os.ReadDir(base); err == nil {
							for _, sub := range subs {
								if sub.IsDir() {
									candidates = append(candidates,
										filepath.Join(base, sub.Name(), "bin"),
										filepath.Join(base, sub.Name()),
									)
								}
							}
						}
					}
				}
			}
		}
		if homeDir != "" {
			candidates = append(candidates,
				filepath.Join(homeDir, "scoop", "apps", "ffmpeg", "current", "bin"),
				filepath.Join(homeDir, "scoop", "shims"),
			)
		}
		if programData := os.Getenv("ProgramData"); programData != "" {
			candidates = append(candidates, filepath.Join(programData, "chocolatey", "bin"))
		}
	} else {
		candidates = append(candidates,
			"/opt/homebrew/bin",
			"/usr/local/bin",
			"/usr/bin",
		)
	}

	for _, c := range candidates {
		ffExe := "ffmpeg"
		if runtime.GOOS == "windows" {
			ffExe = "ffmpeg.exe"
		}
		if _, err := os.Stat(filepath.Join(c, ffExe)); err == nil {
			return c
		}
	}
	return ""
}

// BuildDownloadArgs builds command line arguments for download
func (y *YtdlpExecutor) BuildDownloadArgs(options *YtdlpOptions) []string {
	args := []string{
		"--no-warnings",
		"--newline",
		"--output", filepath.Join(options.OutputDir, options.OutputTemplate),
	}

	if ffmpegLoc := FindFFmpegLocation(); ffmpegLoc != "" {
		args = append(args, "--ffmpeg-location", ffmpegLoc)
	}

	// Add playlist handling
	if !options.DownloadPlaylist {
		args = append(args, "--no-playlist") // Default to single video
	}

	isAudioOnlyMP3 := strings.EqualFold(options.Format, config.OutputFormatMP3)

	if isAudioOnlyMP3 {
		args = append(args,
			"--format", "bestaudio/best",
			"--extract-audio",
			"--audio-format", config.OutputFormatMP3,
		)
		if audioQuality := normalizeAudioQualityForYtdlp(options.AudioQuality); audioQuality != "" {
			args = append(args, "--audio-quality", audioQuality)
		}
	} else if options.Quality != "" {
		normQ := config.NormalizeQuality(options.Quality)
		if normQ == config.QualityBest {
			args = append(args,
				"--format", "bestvideo+bestaudio[ext=m4a]/bestvideo+bestaudio/best",
				"--format-sort", "res,fps,vcodec:h264,aext:m4a",
				"--merge-output-format", "mp4",
			)
		} else if normQ == config.QualityWorst {
			args = append(args, "--format", "worstvideo+worstaudio/worst")
		} else {
			height, ok := config.QualityToHeight(normQ)
			if !ok {
				height = "1080"
			}
			args = append(args,
				"--format", fmt.Sprintf("bestvideo[height<=%s]+bestaudio[ext=m4a]/bestvideo[height<=%s]+bestaudio/best[height<=%s]/best", height, height, height),
				"--format-sort", "res,fps,vcodec:h264,aext:m4a",
				"--merge-output-format", "mp4",
			)
		}
	}

	// Add authentication arguments
	if len(options.AuthArgs) > 0 {
		args = append(args, options.AuthArgs...)
	}

	// Add YouTube-specific arguments
	args = append(args, "--extractor-args", fmt.Sprintf("youtube:client=%s", y.Config.YouTube.Client))

	// Add proxy if configured
	if y.Config.Proxy.Enabled && y.Config.Proxy.URL != "" {
		args = append(args, "--proxy", y.Config.Proxy.URL)
	}

	// Add user agent if specified
	if y.Config.Advanced.UserAgent != "" {
		args = append(args, "--user-agent", y.Config.Advanced.UserAgent)
	}

	// Add retry count
	if y.Config.Advanced.RetryCount > 0 {
		args = append(args, "--retries", strconv.Itoa(y.Config.Advanced.RetryCount))
	}

	// Add extra arguments from config
	if len(y.Config.Advanced.ExtraArgs) > 0 {
		args = append(args, y.Config.Advanced.ExtraArgs...)
	}

	// Add extra arguments from options
	if len(options.ExtraArgs) > 0 {
		args = append(args, options.ExtraArgs...)
	}

	// Add URL last after '--' option terminator to prevent flag injection
	args = append(args, "--", options.URL)

	return args
}

func normalizeAudioQualityForYtdlp(raw string) string {
	quality := strings.TrimSpace(strings.ToLower(raw))
	switch quality {
	case "":
		return ""
	case "best":
		return "0"
	case "worst":
		return "10"
	default:
		return strings.TrimSpace(raw)
	}
}

// parseProgress parses progress information from yt-dlp output
func (y *YtdlpExecutor) parseProgress(line string) *DownloadProgress {
	// yt-dlp progress line format: [download] 12.5% of 100.00MiB at 1.23MiB/s ETA 00:42
	progressRegex := regexp.MustCompile(`\[download\]\s+(\d+\.?\d*)%\s+of\s+~?([\d.]+\w+)\s+at\s+([\d.]+\w+/s)\s+ETA\s+([\d:]+)`)
	matches := progressRegex.FindStringSubmatch(line)

	if len(matches) != 5 {
		return nil
	}

	percentage, _ := strconv.ParseFloat(matches[1], 64)

	return &DownloadProgress{
		Status:     "downloading",
		Percentage: percentage,
		Speed:      matches[3],
		ETA:        matches[4],
	}
}

// ParseDownloadResult parses the download result from output
func (y *YtdlpExecutor) ParseDownloadResult(result *DownloadResult, outputLines []string, options *YtdlpOptions) error {
	mergedFilePath := ""

	// Find the downloaded/merged file information
	for _, line := range outputLines {
		// Look for merged output line: [Merger] Merging formats into "..."
		if strings.Contains(line, "[Merger] Merging formats into ") {
			parts := strings.SplitN(line, "[Merger] Merging formats into ", 2)
			if len(parts) > 1 {
				mergedFilePath = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			}
		}

		// Look for audio extraction output line: [ExtractAudio] Destination: ...
		if strings.Contains(line, "[ExtractAudio] Destination:") {
			parts := strings.SplitN(line, "Destination:", 2)
			if len(parts) > 1 {
				mergedFilePath = strings.TrimSpace(parts[1])
			}
		}

		// Look for already downloaded line: [download] /path/to/file.mp4 has already been downloaded
		if strings.Contains(line, "[download] ") && strings.Contains(line, " has already been downloaded") {
			trimmed := strings.TrimPrefix(strings.TrimSpace(line), "[download] ")
			trimmed = strings.TrimSuffix(trimmed, " has already been downloaded")
			result.FilePath = strings.TrimSpace(trimmed)
		}

		// Look for destination file line
		if strings.Contains(line, "[download] Destination:") {
			parts := strings.SplitN(line, "Destination:", 2)
			if len(parts) > 1 {
				result.FilePath = strings.TrimSpace(parts[1])
			}
		}

		// Look for file size information
		if strings.Contains(line, "[download]") && strings.Contains(line, "100%") {
			// Extract file size and other info from final progress line
			sizeRegex := regexp.MustCompile(`of\s+~?([\d.]+\w+)`)
			sizeMatches := sizeRegex.FindStringSubmatch(line)
			if len(sizeMatches) > 1 {
				// Convert size to bytes (simplified)
				sizeStr := sizeMatches[1]
				if strings.Contains(sizeStr, "MiB") {
					if sizeMB, err := strconv.ParseFloat(strings.ReplaceAll(sizeStr, "MiB", ""), 64); err == nil {
						result.FileSize = int64(sizeMB * 1024 * 1024)
					}
				}
			}
		}
	}

	if mergedFilePath != "" {
		result.FilePath = mergedFilePath
	}

	// Get file info if we have a file path
	if result.FilePath != "" {
		if stat, err := os.Stat(result.FilePath); err == nil {
			result.FileSize = stat.Size()
		}
	}

	result.Quality = options.Quality
	if result.FilePath != "" {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(result.FilePath)), ".")
		if ext != "" {
			result.Format = ext
		}
	}
	if result.Format == "" {
		if strings.EqualFold(options.Format, config.OutputFormatMP3) {
			result.Format = config.OutputFormatMP3
		} else {
			result.Format = "mp4"
		}
	}

	return nil
}

// extractQualities extracts available qualities from format list
func (y *YtdlpExecutor) extractQualities(formats []config.FormatInfo) []string {
	qualityMap := make(map[string]bool)

	for _, format := range formats {
		if format.Height > 0 {
			quality := fmt.Sprintf("%d", format.Height)
			qualityMap[quality] = true

			// Add standard quality names
			switch format.Height {
			case 2160:
				qualityMap[config.Quality2160p] = true
			case 1440:
				qualityMap[config.Quality1440p] = true
			case 1080:
				qualityMap[config.Quality1080p] = true
			case 720:
				qualityMap[config.Quality720p] = true
			case 480:
				qualityMap[config.Quality480p] = true
			case 360:
				qualityMap[config.Quality360p] = true
			case 240:
				qualityMap[config.Quality240p] = true
			}
		}
	}

	// Convert to slice
	qualities := make([]string, 0, len(qualityMap))
	for quality := range qualityMap {
		qualities = append(qualities, quality)
	}

	return qualities
}

// SetProgressCallback sets the progress callback
func (y *YtdlpExecutor) SetProgressCallback(callback ProgressCallback) {
	y.progressCallback = callback
}

// Close cleans up resources
func (y *YtdlpExecutor) Close() error {
	// No cleanup needed for this implementation
	return nil
}

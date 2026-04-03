package downloader

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~sixban/u2b_download/config"
)

// YtdlpExecutor handles yt-dlp command execution
type YtdlpExecutor struct {
	config           *config.Config
	ytdlpPath        string
	progressCallback ProgressCallback
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
		config: cfg,
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

	cmd := exec.CommandContext(ctx, y.ytdlpPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run yt-dlp --version: %w", err)
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return fmt.Errorf("yt-dlp returned empty version")
	}

	return nil
}

// Download performs the actual download
func (y *YtdlpExecutor) Download(ctx context.Context, options *YtdlpOptions) (*DownloadResult, error) {
	startTime := time.Now()

	// Build command arguments
	args := y.buildDownloadArgs(options)

	// Create command
	cmd := exec.CommandContext(ctx, y.ytdlpPath, args...)

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
	errorOutput := ""

	// Read stderr for errors
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			errorOutput += line + "\n"

			// Parse progress from stderr if callback is set
			if y.progressCallback != nil {
				progress := y.parseProgress(line)
				if progress != nil {
					y.progressCallback(progress)
				}
			}
		}
	}()

	// Read stdout for output
	outputLines := []string{}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputLines = append(outputLines, line)
		}
	}()

	// Wait for completion
	err = cmd.Wait()
	downloadTime := time.Since(startTime).Seconds()
	result.DownloadTime = downloadTime

	if err != nil {
		result.Error = fmt.Sprintf("yt-dlp failed: %v\nOutput: %s", err, errorOutput)
		trimmedErr := strings.TrimSpace(errorOutput)
		if trimmedErr == "" {
			return result, fmt.Errorf("download failed: %w", err)
		}
		return result, fmt.Errorf("download failed: %w: %s", err, trimmedErr)
	}

	// Parse result from output
	if err := y.parseDownloadResult(result, outputLines, options); err != nil {
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
	args = append(args, "--extractor-args", fmt.Sprintf("youtube:client=%s", y.config.YouTube.Client), url)

	cmd := exec.CommandContext(ctx, y.ytdlpPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get playlist info: %w: %s", err, strings.TrimSpace(string(output)))
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
	args = append(args, "--extractor-args", fmt.Sprintf("youtube:client=%s", y.config.YouTube.Client), url)

	cmd := exec.CommandContext(ctx, y.ytdlpPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get video info: %w: %s", err, strings.TrimSpace(string(output)))
	}

	// Parse JSON output - handle multiple JSON objects
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var videoInfo config.VideoInfo

	// Use the first/main JSON object
	if len(lines) > 0 {
		if err := json.Unmarshal([]byte(lines[0]), &videoInfo); err != nil {
			return nil, fmt.Errorf("failed to parse video info JSON: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no video info returned")
	}

	// Extract available qualities from formats
	videoInfo.AvailableQualities = y.extractQualities(videoInfo.Formats)

	return &videoInfo, nil
}

// buildDownloadArgs builds command line arguments for download
func (y *YtdlpExecutor) buildDownloadArgs(options *YtdlpOptions) []string {
	args := []string{
		"--no-warnings",
		"--newline",
		"--output", filepath.Join(options.OutputDir, options.OutputTemplate),
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
		if options.Quality == config.QualityBest {
			args = append(args, "--format", "best")
		} else if options.Quality == config.QualityWorst {
			args = append(args, "--format", "worst")
		} else {
			// Parse quality to height and use simple format selection
			height := options.Quality
			if height == "hd1080" || height == config.Quality1080p {
				height = "1080"
			} else if height == "hd720" || height == config.Quality720p {
				height = "720"
			} else if height == "sd480" || height == config.Quality480p {
				height = "480"
			} else if height == "sd360" || height == config.Quality360p {
				height = "360"
			} else if height == "sd240" || height == config.Quality240p {
				height = "240"
			}
			// Use a more compatible format selector
			args = append(args, "--format", fmt.Sprintf("best[height<=%s]/best", height))
		}
	}

	// Add authentication arguments
	if len(options.AuthArgs) > 0 {
		args = append(args, options.AuthArgs...)
	}

	// Add YouTube-specific arguments
	args = append(args, "--extractor-args", fmt.Sprintf("youtube:client=%s", y.config.YouTube.Client))

	// Add proxy if configured
	if y.config.Proxy.Enabled && y.config.Proxy.URL != "" {
		args = append(args, "--proxy", y.config.Proxy.URL)
	}

	// Add user agent if specified
	if y.config.Advanced.UserAgent != "" {
		args = append(args, "--user-agent", y.config.Advanced.UserAgent)
	}

	// Add retry count
	if y.config.Advanced.RetryCount > 0 {
		args = append(args, "--retries", strconv.Itoa(y.config.Advanced.RetryCount))
	}

	// Add extra arguments from config
	if len(y.config.Advanced.ExtraArgs) > 0 {
		args = append(args, y.config.Advanced.ExtraArgs...)
	}

	// Add extra arguments from options
	if len(options.ExtraArgs) > 0 {
		args = append(args, options.ExtraArgs...)
	}

	// Add URL last
	args = append(args, options.URL)

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
	progressRegex := regexp.MustCompile(`\[download\]\s+(\d+\.?\d*)%\s+of\s+([\d.]+\w+)\s+at\s+([\d.]+\w+/s)\s+ETA\s+([\d:]+)`)
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

// parseDownloadResult parses the download result from output
func (y *YtdlpExecutor) parseDownloadResult(result *DownloadResult, outputLines []string, options *YtdlpOptions) error {
	// Find the downloaded file information
	for _, line := range outputLines {
		// Look for destination file line
		if strings.Contains(line, "[download] Destination:") {
			parts := strings.Split(line, "Destination: ")
			if len(parts) > 1 {
				result.FilePath = strings.TrimSpace(parts[1])
			}
		}

		// Look for file size information
		if strings.Contains(line, "[download]") && strings.Contains(line, "100%") {
			// Extract file size and other info from final progress line
			sizeRegex := regexp.MustCompile(`of\s+([\d.]+\w+)`)
			sizeMatches := sizeRegex.FindStringSubmatch(line)
			if len(sizeMatches) > 1 {
				// Convert size to bytes (simplified)
				sizeStr := sizeMatches[1]
				if strings.Contains(sizeStr, "MiB") {
					if sizeMB, err := strconv.ParseFloat(strings.Replace(sizeStr, "MiB", "", -1), 64); err == nil {
						result.FileSize = int64(sizeMB * 1024 * 1024)
					}
				}
			}
		}
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

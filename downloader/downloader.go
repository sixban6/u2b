package downloader

import (
	"context"
	"fmt"
	"strings"

	"git.sr.ht/~sixban/u2b_download/config"
)

// Downloader defines the interface for video downloaders
type Downloader interface {
	// Download downloads a video from the given URL with specified options
	Download(ctx context.Context, url string, options *config.DownloadOptions) (*DownloadResult, error)

	// ValidateAuth validates the current authentication configuration
	ValidateAuth(ctx context.Context) error

	// GetVideoInfo retrieves video information without downloading
	GetVideoInfo(ctx context.Context, url string) (*config.VideoInfo, error)

	// GetPlaylistInfo retrieves playlist information without downloading
	GetPlaylistInfo(ctx context.Context, url string) (*config.VideoInfo, error)

	// GetAvailableQualities returns available video qualities for a URL
	GetAvailableQualities(ctx context.Context, url string) ([]string, error)

	// Close cleans up resources
	Close() error
}

// AuthProvider defines the interface for authentication providers
type AuthProvider interface {
	// GetAuthArgs returns authentication arguments for yt-dlp
	GetAuthArgs() ([]string, error)

	// ValidateAuth validates the authentication setup
	ValidateAuth() error

	// GetCookies returns cookies as a string (for debugging/logging)
	GetCookies() (string, error)
}

// QualitySelector defines the interface for quality selection
type QualitySelector interface {
	// SelectBestQuality selects the best available quality based on preferences
	SelectBestQuality(available []string, preferred string, fallback string) (string, error)

	// ParseQuality parses and validates quality string
	ParseQuality(quality string) (string, error)
}

// DownloadResult represents the result of a download operation
type DownloadResult struct {
	VideoInfo    *config.VideoInfo `json:"video_info"`
	FilePath     string            `json:"file_path"`
	FileSize     int64             `json:"file_size"`
	Duration     float64           `json:"duration"`
	Quality      string            `json:"quality"`
	Format       string            `json:"format"`
	DownloadTime float64           `json:"download_time_seconds"`
	Success      bool              `json:"success"`
	Error        string            `json:"error,omitempty"`
}

// ProgressCallback is called during download to report progress
type ProgressCallback func(progress *DownloadProgress)

// DownloadProgress represents download progress information
type DownloadProgress struct {
	VideoID         string  `json:"video_id"`
	Title           string  `json:"title"`
	Filename        string  `json:"filename"`
	Status          string  `json:"status"`     // downloading, finished, error
	Percentage      float64 `json:"percentage"` // 0-100
	Speed           string  `json:"speed"`      // human readable speed
	ETA             string  `json:"eta"`        // estimated time remaining
	DownloadedBytes int64   `json:"downloaded_bytes"`
	TotalBytes      int64   `json:"total_bytes"`
}

// YouTubeDownloader implements the Downloader interface using yt-dlp
type YouTubeDownloader struct {
	config             *config.Config
	authProvider       AuthProvider
	qualitySelector    QualitySelector
	ytdlp              *YtdlpExecutor
	useBrowserFallback bool
}

// NewYouTubeDownloader creates a new YouTube downloader instance
func NewYouTubeDownloader(cfg *config.Config) (*YouTubeDownloader, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Create auth provider
	authProvider, err := NewAuthProvider(cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth provider: %w", err)
	}

	// Create quality selector
	qualitySelector := NewQualitySelector()

	// Create yt-dlp executor
	ytdlp, err := NewYtdlpExecutor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create yt-dlp executor: %w", err)
	}

	return &YouTubeDownloader{
		config:          cfg,
		authProvider:    authProvider,
		qualitySelector: qualitySelector,
		ytdlp:           ytdlp,
	}, nil
}

// SanitizeURL removes shell-escaped backslashes (e.g. \? \= \& from zsh url-quote-magic) in HTTP(S) URLs.
func SanitizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return strings.ReplaceAll(trimmed, `\`, "")
	}
	return trimmed
}

// Download implements the Downloader interface
func (d *YouTubeDownloader) Download(ctx context.Context, url string, options *config.DownloadOptions) (*DownloadResult, error) {
	url = SanitizeURL(url)
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}
	if options == nil {
		return nil, fmt.Errorf("download options cannot be nil")
	}
	options.URL = url

	// Validate authentication before starting download
	if err := d.ValidateAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication validation failed: %w", err)
	}

	// Get video info first
	videoInfo, err := d.GetVideoInfo(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get video info: %w", err)
	}

	requestedFormat := strings.ToLower(strings.TrimSpace(options.Format))
	isAudioOnlyMP3 := requestedFormat == config.OutputFormatMP3

	selectedQuality := ""
	if !isAudioOnlyMP3 {
		// Select quality only for video downloads.
		availableQualities := videoInfo.AvailableQualities
		selectedQuality, err = d.qualitySelector.SelectBestQuality(
			availableQualities,
			options.Quality,
			d.config.Quality.Fallback,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to select quality: %w", err)
		}
	}

	// Prepare download options
	downloadOptions := &YtdlpOptions{
		URL:            url,
		Quality:        selectedQuality,
		Format:         requestedFormat,
		AudioQuality:   d.config.Quality.AudioQuality,
		OutputDir:      options.OutputDir,
		OutputTemplate: options.OutputTemplate,
		AuthArgs:       nil, // will be set by auth provider
		ExtraArgs:      options.ExtraArgs,
	}

	// Add authentication arguments
	authArgs, err := d.getEffectiveAuthArgs()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth args: %w", err)
	}
	downloadOptions.AuthArgs = authArgs

	// Perform download
	result, err := d.ytdlp.Download(ctx, downloadOptions)
	if err != nil {
		fallbackArgs, ok := d.getBrowserFallbackAuthArgs()
		if ok && !d.useBrowserFallback && NeedsBrowserAuthRetry(err) {
			d.useBrowserFallback = true
			downloadOptions.AuthArgs = fallbackArgs
			result, err = d.ytdlp.Download(ctx, downloadOptions)
		}
	}
	if err != nil {
		// Fallback: check if local yt-dlp is outdated, upgrade, and retry once
		if upgraded, _ := d.ytdlp.TryUpgradeIfOutdated(ctx); upgraded {
			result, err = d.ytdlp.Download(ctx, downloadOptions)
		}
	}
	if err != nil {
		formattedErr := FormatAuthError(err, d.config.Auth.Browser)
		return &DownloadResult{
			VideoInfo: videoInfo,
			Success:   false,
			Error:     formattedErr.Error(),
		}, formattedErr
	}

	result.VideoInfo = videoInfo
	return result, nil
}

// ValidateAuth implements the Downloader interface
func (d *YouTubeDownloader) ValidateAuth(ctx context.Context) error {
	return d.authProvider.ValidateAuth()
}

// GetVideoInfo implements the Downloader interface
func (d *YouTubeDownloader) GetVideoInfo(ctx context.Context, url string) (*config.VideoInfo, error) {
	url = SanitizeURL(url)
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	// Get authentication arguments
	authArgs, err := d.getEffectiveAuthArgs()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth args: %w", err)
	}

	// Get video info using yt-dlp
	videoInfo, err := d.ytdlp.GetVideoInfo(ctx, url, authArgs)
	if err == nil {
		return videoInfo, nil
	}

	fallbackArgs, ok := d.getBrowserFallbackAuthArgs()
	if ok && !d.useBrowserFallback && NeedsBrowserAuthRetry(err) {
		d.useBrowserFallback = true
		authArgs = fallbackArgs
		videoInfo, err = d.ytdlp.GetVideoInfo(ctx, url, authArgs)
		if err == nil {
			return videoInfo, nil
		}

		// If initial browser failed due to file lock or decryption, try alternate installed browsers (edge, chrome, firefox)
		if isCookieLockOrAuthError(err) {
			alternates := []string{"edge", "chrome", "firefox"}
			for _, alt := range alternates {
				if strings.EqualFold(alt, d.config.Auth.Browser) {
					continue
				}
				altArgs := []string{"--cookies-from-browser", alt}
				if altInfo, altErr := d.ytdlp.GetVideoInfo(ctx, url, altArgs); altErr == nil {
					d.config.Auth.Browser = alt
					return altInfo, nil
				}
			}
		}
	}

	// Fallback: check if local yt-dlp is outdated, upgrade, and retry once
	if upgraded, _ := d.ytdlp.TryUpgradeIfOutdated(ctx); upgraded {
		info, uErr := d.ytdlp.GetVideoInfo(ctx, url, authArgs)
		if uErr == nil {
			return info, nil
		}
		err = uErr
	}

	return nil, FormatAuthError(err, d.config.Auth.Browser)
}

// GetPlaylistInfo implements the Downloader interface
func (d *YouTubeDownloader) GetPlaylistInfo(ctx context.Context, url string) (*config.VideoInfo, error) {
	url = SanitizeURL(url)
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	// Get authentication arguments
	authArgs, err := d.getEffectiveAuthArgs()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth args: %w", err)
	}

	// Get playlist info using yt-dlp
	playlistInfo, err := d.ytdlp.GetPlaylistInfo(ctx, url, authArgs)
	if err == nil {
		return playlistInfo, nil
	}

	fallbackArgs, ok := d.getBrowserFallbackAuthArgs()
	if ok && !d.useBrowserFallback && NeedsBrowserAuthRetry(err) {
		d.useBrowserFallback = true
		authArgs = fallbackArgs
		playlistInfo, err = d.ytdlp.GetPlaylistInfo(ctx, url, authArgs)
		if err == nil {
			return playlistInfo, nil
		}

		if isCookieLockOrAuthError(err) {
			alternates := []string{"edge", "chrome", "firefox"}
			for _, alt := range alternates {
				if strings.EqualFold(alt, d.config.Auth.Browser) {
					continue
				}
				altArgs := []string{"--cookies-from-browser", alt}
				if altInfo, altErr := d.ytdlp.GetPlaylistInfo(ctx, url, altArgs); altErr == nil {
					d.config.Auth.Browser = alt
					return altInfo, nil
				}
			}
		}
	}

	// Fallback: check if local yt-dlp is outdated, upgrade, and retry once
	if upgraded, _ := d.ytdlp.TryUpgradeIfOutdated(ctx); upgraded {
		pInfo, uErr := d.ytdlp.GetPlaylistInfo(ctx, url, authArgs)
		if uErr == nil {
			return pInfo, nil
		}
		err = uErr
	}

	return nil, FormatAuthError(err, d.config.Auth.Browser)
}

func (d *YouTubeDownloader) getEffectiveAuthArgs() ([]string, error) {
	if d.useBrowserFallback {
		if fallbackArgs, ok := d.getBrowserFallbackAuthArgs(); ok {
			return fallbackArgs, nil
		}
	}
	return d.authProvider.GetAuthArgs()
}

// GetAvailableQualities implements the Downloader interface
func (d *YouTubeDownloader) GetAvailableQualities(ctx context.Context, url string) ([]string, error) {
	videoInfo, err := d.GetVideoInfo(ctx, url)
	if err != nil {
		return nil, err
	}

	return videoInfo.AvailableQualities, nil
}

// Close implements the Downloader interface
func (d *YouTubeDownloader) Close() error {
	if d.ytdlp != nil {
		return d.ytdlp.Close()
	}
	return nil
}

// SetProgressCallback sets a callback for download progress updates
func (d *YouTubeDownloader) SetProgressCallback(callback ProgressCallback) {
	if d.ytdlp != nil {
		d.ytdlp.SetProgressCallback(callback)
	}
}

func (d *YouTubeDownloader) getBrowserFallbackAuthArgs() ([]string, bool) {
	if d.config == nil {
		return nil, false
	}
	if d.config.Auth.Method != config.AuthMethodNone {
		return nil, false
	}
	if d.config.Auth.Browser == "" {
		return nil, false
	}

	return []string{"--cookies-from-browser", d.config.Auth.Browser}, true
}

func NeedsBrowserAuthRetry(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sign in to confirm you're not a bot") ||
		strings.Contains(msg, "sign in to confirm you’re not a bot") ||
		strings.Contains(msg, "use --cookies-from-browser") ||
		strings.Contains(msg, "use --cookies")
}

func isCookieLockOrAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "could not copy chrome cookie database") ||
		(strings.Contains(msg, "permission denied") && strings.Contains(msg, "cookies")) ||
		strings.Contains(msg, "failed to decrypt with dpapi") ||
		strings.Contains(msg, "sharing violation")
}

// FormatAuthError converts cryptic yt-dlp cookie lock errors into clear actionable instructions
func FormatAuthError(err error, browser string) error {
	if err == nil {
		return nil
	}
	if !isCookieLockOrAuthError(err) {
		return err
	}

	currentBrowser := strings.ToUpper(browser)
	if currentBrowser == "" {
		currentBrowser = "BRAVE / CHROME"
	}

	return fmt.Errorf("检测到浏览器 Cookie 数据库读取受阻（%s 当前正在运行并独占锁定了 Cookie 数据库）。\n\n"+
		"💡 解决方案（任选一种即可）：\n"+
		"   1. 完全退出 %s 浏览器后台进程后重新运行命令；\n"+
		"   2. 或在浏览器中使用 u2b 扩展右键一键下载（扩展直接在内存中传递 Cookie，无需关闭浏览器）；\n"+
		"   3. 或通过浏览器扩展导出 cookies.txt，执行：u2b --cookies-file ./cookies.txt <URL>\n\n"+
		"原始报错: %w", currentBrowser, currentBrowser, err)
}

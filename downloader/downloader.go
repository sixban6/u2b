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
	config          *config.Config
	authProvider    AuthProvider
	qualitySelector QualitySelector
	ytdlp           *YtdlpExecutor
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

// Download implements the Downloader interface
func (d *YouTubeDownloader) Download(ctx context.Context, url string, options *config.DownloadOptions) (*DownloadResult, error) {
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	// Validate authentication before starting download
	if err := d.ValidateAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication validation failed: %w", err)
	}

	// Get video info first
	videoInfo, err := d.GetVideoInfo(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get video info: %w", err)
	}

	// Select quality
	availableQualities := videoInfo.AvailableQualities
	selectedQuality, err := d.qualitySelector.SelectBestQuality(
		availableQualities,
		options.Quality,
		d.config.Quality.Fallback,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to select quality: %w", err)
	}

	// Prepare download options
	downloadOptions := &YtdlpOptions{
		URL:            url,
		Quality:        selectedQuality,
		OutputDir:      options.OutputDir,
		OutputTemplate: options.OutputTemplate,
		AuthArgs:       nil, // will be set by auth provider
		ExtraArgs:      options.ExtraArgs,
	}

	// Add authentication arguments
	authArgs, err := d.authProvider.GetAuthArgs()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth args: %w", err)
	}
	downloadOptions.AuthArgs = authArgs

	// Perform download
	result, err := d.ytdlp.Download(ctx, downloadOptions)
	if err != nil {
		fallbackArgs, ok := d.getBrowserFallbackAuthArgs()
		if ok && NeedsBrowserAuthRetry(err) {
			downloadOptions.AuthArgs = fallbackArgs
			result, err = d.ytdlp.Download(ctx, downloadOptions)
		}
	}
	if err != nil {
		return &DownloadResult{
			VideoInfo: videoInfo,
			Success:   false,
			Error:     err.Error(),
		}, err
	}

	return result, nil
}

// ValidateAuth implements the Downloader interface
func (d *YouTubeDownloader) ValidateAuth(ctx context.Context) error {
	return d.authProvider.ValidateAuth()
}

// GetVideoInfo implements the Downloader interface
func (d *YouTubeDownloader) GetVideoInfo(ctx context.Context, url string) (*config.VideoInfo, error) {
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	// Get authentication arguments
	authArgs, err := d.authProvider.GetAuthArgs()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth args: %w", err)
	}

	// Get video info using yt-dlp
	videoInfo, err := d.ytdlp.GetVideoInfo(ctx, url, authArgs)
	if err == nil {
		return videoInfo, nil
	}

	fallbackArgs, ok := d.getBrowserFallbackAuthArgs()
	if ok && NeedsBrowserAuthRetry(err) {
		return d.ytdlp.GetVideoInfo(ctx, url, fallbackArgs)
	}

	return nil, err
}

// GetPlaylistInfo implements the Downloader interface
func (d *YouTubeDownloader) GetPlaylistInfo(ctx context.Context, url string) (*config.VideoInfo, error) {
	if url == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	// Get authentication arguments
	authArgs, err := d.authProvider.GetAuthArgs()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth args: %w", err)
	}

	// Get playlist info using yt-dlp
	playlistInfo, err := d.ytdlp.GetPlaylistInfo(ctx, url, authArgs)
	if err == nil {
		return playlistInfo, nil
	}

	fallbackArgs, ok := d.getBrowserFallbackAuthArgs()
	if ok && NeedsBrowserAuthRetry(err) {
		return d.ytdlp.GetPlaylistInfo(ctx, url, fallbackArgs)
	}

	return nil, err
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

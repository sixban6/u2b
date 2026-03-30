package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"git.sr.ht/~sixban/u2b_download/config"
	"git.sr.ht/~sixban/u2b_download/downloader"
	"github.com/spf13/cobra"
)

var (
	configPath  string
	verbose     bool
	outputDir   string
	fileName    string
	quality     string
	format      string
	authMethod  string
	browser     string
	cookiesFile string
	proxy       string
	concurrent  int
	retries     int
	timeout     string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "u2b_download",
	Short: "A powerful YouTube downloader with membership channel support",
	Long: `u2b_download is a command-line tool for downloading YouTube videos, 
including content from membership channels. It uses yt-dlp under the hood
and supports various authentication methods including browser cookies.

Examples:
  u2b_download "https://www.youtube.com/watch?v=VIDEO_ID"
  u2b_download download -o ./downloads -n "Star Sky" "https://www.youtube.com/watch?v=VIDEO_ID"
  u2b_download download -q hd1080 "https://www.youtube.com/watch?v=VIDEO_ID"
  u2b_download download --quality hd720 --output-dir ./videos "URL"
  u2b_download info "https://www.youtube.com/watch?v=VIDEO_ID"
  u2b_download config init`,
	Version: "1.0.0",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runRoot,
}

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download [URL]",
	Short: "Download video from URL",
	Long: `Download video from the specified URL. Supports YouTube membership videos
when properly configured with authentication.

The quality parameter accepts the following values:
  - hd2160, hd1440, hd1080, hd720, sd480, sd360, sd240
  - best, worst
  - Custom heights like 1080, 720, etc.

Examples:
  u2b_download download -q hd1080 "https://www.youtube.com/watch?v=VIDEO_ID"
  u2b_download download --quality best --output-dir ./downloads "URL"`,
	Args: cobra.ExactArgs(1),
	RunE: runDownload,
}

// infoCmd represents the info command
var infoCmd = &cobra.Command{
	Use:   "info [URL]",
	Short: "Get video information without downloading",
	Long: `Retrieve information about a video including available qualities,
duration, title, and other metadata without downloading the video.`,
	Args: cobra.ExactArgs(1),
	RunE: runInfo,
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  `Manage configuration files and settings.`,
}

// initConfigCmd represents the config init command
var initConfigCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file",
	Long: `Create a default configuration file. If a config file already exists,
it will be backed up before creating a new one.`,
	RunE: runInitConfig,
}

// validateConfigCmd represents the config validate command
var validateConfigCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long:  `Validate the current configuration file for errors and test authentication.`,
	RunE:  runValidateConfig,
}

// downloadSeriesCmd represents the download-series command
var downloadSeriesCmd = &cobra.Command{
	Use:   "download-series [playlist_url_or_name]",
	Short: "Download entire video series/playlist",
	Long: `Download all videos from a YouTube playlist or series. You can provide either:
  - A playlist URL (https://www.youtube.com/playlist?list=...)
  - A video URL with playlist parameter (&list=...)
  - Or a series name to search for

Examples:
  u2b_download download-series "https://www.youtube.com/watch?v=VIDEO_ID&list=PLAYLIST_ID"
  u2b_download download-series "《会员课》 日内交易理论与实盘"
  u2b_download download-series -q hd1080 "PLAYLIST_URL"`,
	Args: cobra.ExactArgs(1),
	RunE: runDownloadSeries,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Download command flags
	downloadCmd.Flags().StringVarP(&quality, "quality", "q", "", "video quality (hd1080, hd720, best, etc.)")
	downloadCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "output directory")
	downloadCmd.Flags().StringVarP(&fileName, "filename", "n", "", "set output file name (without extension)")
	downloadCmd.Flags().StringVar(&format, "format", "", "output format")
	downloadCmd.Flags().StringVar(&authMethod, "auth-method", "", "authentication method (none, browser, cookies_file)")
	downloadCmd.Flags().StringVar(&browser, "browser", "", "browser for cookie extraction (chrome, firefox, etc.)")
	downloadCmd.Flags().StringVar(&cookiesFile, "cookies-file", "", "path to cookies file")
	downloadCmd.Flags().StringVar(&proxy, "proxy", "", "proxy URL")
	downloadCmd.Flags().IntVar(&concurrent, "concurrent", 0, "number of concurrent downloads")
	downloadCmd.Flags().IntVar(&retries, "retries", 0, "number of retry attempts")
	downloadCmd.Flags().StringVar(&timeout, "timeout", "", "download timeout (e.g., 30m, 1h)")

	// Download series command flags
	downloadSeriesCmd.Flags().StringVarP(&quality, "quality", "q", "", "video quality (hd1080, hd720, best, etc.)")
	downloadSeriesCmd.Flags().StringVarP(&outputDir, "output-dir", "o", "", "output directory")
	downloadSeriesCmd.Flags().StringVar(&authMethod, "auth-method", "", "authentication method (none, browser, cookies_file)")
	downloadSeriesCmd.Flags().StringVar(&browser, "browser", "", "browser for cookie extraction")
	downloadSeriesCmd.Flags().IntVar(&concurrent, "concurrent", 0, "number of concurrent downloads")

	// Add subcommands
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(downloadSeriesCmd)
	configCmd.AddCommand(initConfigCmd)
	configCmd.AddCommand(validateConfigCmd)
}

func runRoot(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	url := args[0]
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("unknown command %q for %q", url, cmd.CommandPath())
	}

	return runDownload(downloadCmd, args)
}

// runDownload executes the download command
func runDownload(cmd *cobra.Command, args []string) error {
	url := args[0]

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line flags
	overrideConfigWithFlags(cfg)

	// Create downloader
	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		return fmt.Errorf("failed to create downloader: %w", err)
	}
	defer d.Close()

	// Set up progress callback if verbose
	if verbose {
		d.SetProgressCallback(func(progress *downloader.DownloadProgress) {
			fmt.Printf("\r[%s] %.1f%% - %s - ETA: %s",
				progress.Status, progress.Percentage, progress.Speed, progress.ETA)
		})
	}

	// Prepare download options
	options := &config.DownloadOptions{
		URL:            url,
		Quality:        cfg.Quality.Preferred,
		OutputDir:      cfg.Download.OutputDir,
		OutputTemplate: cfg.Download.OutputTemplate,
		ExtraArgs:      cfg.Advanced.ExtraArgs,
	}

	// Override quality if specified
	if quality != "" {
		options.Quality = quality
	}
	if fileName != "" {
		customTemplate, err := BuildOutputTemplateForFilename(fileName)
		if err != nil {
			return err
		}
		options.OutputTemplate = customTemplate
	}

	// Create context with timeout
	ctx := context.Background()
	if cfg.Advanced.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Advanced.Timeout)
		defer cancel()
	}

	// Start download
	fmt.Printf("Starting download of: %s\n", url)
	fmt.Printf("Quality: %s\n", options.Quality)
	fmt.Printf("Output directory: %s\n", options.OutputDir)
	if fileName != "" {
		fmt.Printf("Output filename: %s\n", fileName)
	}

	result, err := d.Download(ctx, url, options)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Print results
	if verbose {
		fmt.Printf("\n")
	}
	fmt.Printf("Download completed successfully!\n")
	fmt.Printf("File: %s\n", result.FilePath)
	fmt.Printf("Size: %.2f MB\n", float64(result.FileSize)/(1024*1024))
	fmt.Printf("Duration: %.2f seconds\n", result.DownloadTime)

	if result.VideoInfo != nil {
		fmt.Printf("Title: %s\n", result.VideoInfo.Title)
		fmt.Printf("Uploader: %s\n", result.VideoInfo.Uploader)
		fmt.Printf("Duration: %.0f seconds\n", result.VideoInfo.Duration)
	}

	return nil
}

// runDownloadSeries executes the download-series command
func runDownloadSeries(cmd *cobra.Command, args []string) error {
	input := args[0]

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line flags
	overrideConfigWithFlags(cfg)

	// Create downloader
	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		return fmt.Errorf("failed to create downloader: %w", err)
	}
	defer d.Close()

	var playlistURL string

	// Check if input is a URL
	if strings.Contains(input, "youtube.com") || strings.Contains(input, "youtu.be") {
		playlistURL = input
	} else {
		// Search for the series name (for now, return error asking for URL)
		return fmt.Errorf("series name search not implemented yet. Please provide the full playlist URL:\n" +
			"Example: https://www.youtube.com/watch?v=0jZ06m0SZj8&list=PLRGWg7K87hcN525OoyR4jVaTyFym1HXx_")
	}

	// Get playlist info first
	ctx := context.Background()
	if cfg.Advanced.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Advanced.Timeout)
		defer cancel()
	}

	// Extract playlist title from URL to create folder
	fmt.Printf("Getting playlist information...\n")
	playlistInfo, err := d.GetPlaylistInfo(ctx, playlistURL)
	if err != nil {
		return fmt.Errorf("failed to get playlist info: %w", err)
	}

	// Create series folder
	seriesFolder := playlistInfo.PlaylistTitle
	if seriesFolder == "" {
		seriesFolder = "Unknown_Series"
	}

	// Clean folder name for file system
	seriesFolder = sanitizeFolderName(seriesFolder)
	seriesPath := filepath.Join(cfg.Download.OutputDir, seriesFolder)

	if err := os.MkdirAll(seriesPath, 0755); err != nil {
		return fmt.Errorf("failed to create series folder: %w", err)
	}

	fmt.Printf("Created series folder: %s\n", seriesPath)
	fmt.Printf("Downloading playlist: %s\n", playlistInfo.PlaylistTitle)
	fmt.Printf("Total videos: %d\n", playlistInfo.PlaylistCount)

	// Prepare download options for playlist
	options := &config.DownloadOptions{
		URL:            playlistURL,
		Quality:        cfg.Quality.Preferred,
		OutputDir:      seriesPath,                                          // Use series folder
		OutputTemplate: "%(playlist_index)02d - %(title)s [%(id)s].%(ext)s", // Numbered format
		ExtraArgs:      cfg.Advanced.ExtraArgs,
	}

	// Override quality if specified
	if quality != "" {
		options.Quality = quality
	}

	// Set up progress callback if verbose
	if verbose {
		d.SetProgressCallback(func(progress *downloader.DownloadProgress) {
			fmt.Printf("\r[%s] %.1f%% - %s - ETA: %s",
				progress.Status, progress.Percentage, progress.Speed, progress.ETA)
		})
	}

	// Download the entire playlist
	fmt.Printf("Starting playlist download...\n")

	// We need to modify the download function to support playlists
	// For now, let's use yt-dlp directly
	return downloadPlaylistDirect(ctx, cfg, playlistURL, seriesPath, options)
}

// sanitizeFolderName cleans a string to be safe for use as a folder name
func sanitizeFolderName(name string) string {
	// Replace problematic characters
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "*", "_")
	name = strings.ReplaceAll(name, "?", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "<", "_")
	name = strings.ReplaceAll(name, ">", "_")
	name = strings.ReplaceAll(name, "|", "_")

	// Trim whitespace
	name = strings.TrimSpace(name)

	// Limit length
	if len(name) > 100 {
		name = name[:100]
	}

	return name
}

// downloadPlaylistDirect uses yt-dlp directly to download playlist
func downloadPlaylistDirect(ctx context.Context, cfg *config.Config, url, outputDir string, options *config.DownloadOptions) error {
	// Find yt-dlp path
	ytdlpPath := cfg.Advanced.YtdlpPath
	if ytdlpPath == "" {
		path, err := exec.LookPath("yt-dlp")
		if err != nil {
			return fmt.Errorf("yt-dlp not found in PATH")
		}
		ytdlpPath = path
	}

	// Build arguments
	args := []string{
		"--cookies-from-browser", "brave",
		"--output", filepath.Join(outputDir, "%(playlist_index)02d - %(title)s [%(id)s].%(ext)s"),
		"--extractor-args", "youtube:client=web_creator",
	}

	// Add quality selection
	if options.Quality != "" {
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
		args = append(args, "--format", fmt.Sprintf("best[height<=%s]/best", height))
	}

	args = append(args, url)

	// Execute yt-dlp
	cmd := exec.CommandContext(ctx, ytdlpPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Executing: %s %s\n", ytdlpPath, strings.Join(args, " "))

	return cmd.Run()
}

// runInfo executes the info command
func runInfo(cmd *cobra.Command, args []string) error {
	url := args[0]

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create downloader
	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		return fmt.Errorf("failed to create downloader: %w", err)
	}
	defer d.Close()

	// Get video info
	ctx := context.Background()
	videoInfo, err := d.GetVideoInfo(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to get video info: %w", err)
	}

	// Print video information
	fmt.Printf("Video Information:\n")
	fmt.Printf("  ID: %s\n", videoInfo.ID)
	fmt.Printf("  Title: %s\n", videoInfo.Title)
	fmt.Printf("  Uploader: %s\n", videoInfo.Uploader)
	fmt.Printf("  Duration: %.0f seconds (%.2f minutes)\n", videoInfo.Duration, videoInfo.Duration/60)
	fmt.Printf("  Upload Date: %s\n", videoInfo.UploadDate)

	if len(videoInfo.Description) > 200 {
		fmt.Printf("  Description: %s...\n", videoInfo.Description[:200])
	} else {
		fmt.Printf("  Description: %s\n", videoInfo.Description)
	}

	fmt.Printf("  Available Qualities: %s\n", strings.Join(videoInfo.AvailableQualities, ", "))

	if verbose && len(videoInfo.Formats) > 0 {
		fmt.Printf("\nDetailed Format Information:\n")
		for i, format := range videoInfo.Formats {
			if i >= 10 { // Limit output
				fmt.Printf("  ... and %d more formats\n", len(videoInfo.Formats)-i)
				break
			}
			fmt.Printf("  Format %s: %dx%d, %s/%s\n",
				format.FormatID, format.Width, format.Height, format.VCodec, format.ACodec)
		}
	}

	return nil
}

// runInitConfig executes the config init command
func runInitConfig(cmd *cobra.Command, args []string) error {
	configFile := configPath
	if configFile == "" {
		configFile = "./config.yaml"
	}

	// Check if config already exists
	if _, err := os.Stat(configFile); err == nil {
		// Backup existing config
		backupFile := configFile + ".backup." + time.Now().Format("20060102-150405")
		if err := os.Rename(configFile, backupFile); err != nil {
			return fmt.Errorf("failed to backup existing config: %w", err)
		}
		fmt.Printf("Existing config backed up to: %s\n", backupFile)
	}

	// Create new config
	if err := config.CreateDefaultConfig(configFile); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	fmt.Printf("Configuration file created: %s\n", configFile)
	fmt.Printf("Please edit the file to configure authentication and other settings.\n")

	return nil
}

// runValidateConfig executes the config validate command
func runValidateConfig(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Configuration validation:\n")
	fmt.Printf("  Config file: %s\n", getEffectiveConfigPath())
	fmt.Printf("  Auth method: %s\n", cfg.Auth.Method)

	if cfg.Auth.Method == config.AuthMethodNone {
		fmt.Printf("  Auth mode: none (public videos)\n")
	} else if cfg.Auth.Method == config.AuthMethodBrowser {
		fmt.Printf("  Browser: %s\n", cfg.Auth.Browser)
	} else if cfg.Auth.Method == config.AuthMethodCookiesFile {
		fmt.Printf("  Cookies file: %s\n", cfg.Auth.CookiesFile)
	}

	// Test authentication
	fmt.Printf("\nTesting authentication...\n")
	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		return fmt.Errorf("failed to create downloader: %w", err)
	}
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.ValidateAuth(ctx); err != nil {
		fmt.Printf("  ❌ Authentication validation failed: %s\n", err)
		return fmt.Errorf("authentication validation failed")
	}

	fmt.Printf("  ✅ Authentication is working correctly\n")
	fmt.Printf("\nConfiguration is valid!\n")

	return nil
}

// loadConfig loads configuration from file
func loadConfig() (*config.Config, error) {
	return config.LoadConfig(configPath)
}

// getEffectiveConfigPath returns the config path that would be used
func getEffectiveConfigPath() string {
	if configPath != "" {
		return configPath
	}
	return config.GetConfigPath()
}

// overrideConfigWithFlags overrides config values with command line flags
func overrideConfigWithFlags(cfg *config.Config) {
	if outputDir != "" {
		cfg.Download.OutputDir = outputDir
	}
	if authMethod != "" {
		cfg.Auth.Method = authMethod
	}
	if browser != "" {
		cfg.Auth.Browser = browser
	}
	if cookiesFile != "" {
		cfg.Auth.CookiesFile = cookiesFile
	}
	if proxy != "" {
		cfg.Proxy.URL = proxy
		cfg.Proxy.Enabled = true
	}
	if concurrent > 0 {
		cfg.Download.ConcurrentDownloads = concurrent
	}
	if retries > 0 {
		cfg.Advanced.RetryCount = retries
	}
	if timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			cfg.Advanced.Timeout = duration
		}
	}
	if verbose {
		cfg.Advanced.Verbose = true
	}

	// Ensure output directory exists
	if cfg.Download.OutputDir != "" {
		if err := os.MkdirAll(cfg.Download.OutputDir, 0755); err == nil {
			if absPath, err := filepath.Abs(cfg.Download.OutputDir); err == nil {
				cfg.Download.OutputDir = absPath
			}
		}
	}
}

// BuildOutputTemplateForFilename builds a yt-dlp output template from a custom filename.
// It keeps the extension dynamic (e.g. ".%(ext)s") so actual downloaded format is preserved.
func BuildOutputTemplateForFilename(name string) (string, error) {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	ext := filepath.Ext(cleaned)
	if ext != "" {
		cleaned = strings.TrimSuffix(cleaned, ext)
	}

	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	cleaned = strings.TrimSpace(replacer.Replace(cleaned))
	if cleaned == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	return cleaned + ".%(ext)s", nil
}

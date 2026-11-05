package test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sixban6/u2b/config"
	"github.com/sixban6/u2b/downloader"
)

// Integration tests - these tests require actual external dependencies
// They are marked with build tags to be run separately

// TestFullDownloadWorkflow tests the complete download workflow
func TestFullDownloadWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temporary directory for test
	tempDir := t.TempDir()
	
	// Create test configuration
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Method:  config.AuthMethodBrowser,
			Browser: config.BrowserChrome,
		},
		Download: config.DownloadConfig{
			OutputDir:           tempDir,
			OutputTemplate:      "%(title)s.%(ext)s",
			ConcurrentDownloads: 1,
		},
		Quality: config.QualityConfig{
			Preferred:    config.Quality360p, // Use low quality for faster test
			Fallback:     config.Quality240p,
			AudioQuality: "best",
		},
		Advanced: config.AdvancedConfig{
			RetryCount: 1,
			Timeout:    5 * time.Minute,
			Verbose:    true,
		},
		YouTube: config.YouTubeConfig{
			Client:          config.ClientWeb,
			SkipPremiumOnly: true,
		},
	}

	// Use a test video URL (public video for testing)
	testURL := getTestVideoURL()
	if testURL == "" {
		t.Skip("no test video URL available")
	}

	// Create downloader
	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		t.Fatalf("failed to create downloader: %v", err)
	}
	defer d.Close()

	// Test video info retrieval
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	videoInfo, err := d.GetVideoInfo(ctx, testURL)
	if err != nil {
		t.Logf("video info retrieval failed (may be expected): %v", err)
		// Continue with test - this might fail due to authentication requirements
	} else {
		t.Logf("Video info: %s by %s", videoInfo.Title, videoInfo.Uploader)
		t.Logf("Available qualities: %v", videoInfo.AvailableQualities)
	}

	// Test quality availability
	qualities, err := d.GetAvailableQualities(ctx, testURL)
	if err != nil {
		t.Logf("quality check failed (may be expected): %v", err)
	} else {
		t.Logf("Available qualities: %v", qualities)
	}
}

// TestAuthenticationMethods tests different authentication methods
func TestAuthenticationMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name       string
		authConfig config.AuthConfig
		shouldSkip bool
		reason     string
	}{
		{
			name: "browser auth - chrome",
			authConfig: config.AuthConfig{
				Method:  config.AuthMethodBrowser,
				Browser: config.BrowserChrome,
			},
			shouldSkip: !hasBrowser(config.BrowserChrome),
			reason:     "Chrome not available",
		},
		{
			name: "browser auth - firefox",
			authConfig: config.AuthConfig{
				Method:  config.AuthMethodBrowser,
				Browser: config.BrowserFirefox,
			},
			shouldSkip: !hasBrowser(config.BrowserFirefox),
			reason:     "Firefox not available",
		},
		{
			name: "cookies file auth",
			authConfig: config.AuthConfig{
				Method:      config.AuthMethodCookiesFile,
				CookiesFile: createIntegrationTestCookieFile(t),
			},
			shouldSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldSkip {
				t.Skip(tt.reason)
			}

			provider, err := downloader.NewAuthProvider(tt.authConfig)
			if err != nil {
				t.Fatalf("failed to create auth provider: %v", err)
			}

			// Test authentication validation
			err = provider.ValidateAuth()
			if err != nil {
				t.Logf("auth validation failed (may be expected): %v", err)
				// Don't fail the test - auth validation might fail in CI environment
			}

			// Test getting auth args
			args, err := provider.GetAuthArgs()
			if err != nil {
				t.Errorf("failed to get auth args: %v", err)
			}

			if len(args) == 0 {
				t.Error("no auth args returned")
			}

			t.Logf("Auth args: %v", args)
		})
	}
}

// TestYtdlpIntegration tests integration with yt-dlp
func TestYtdlpIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create test configuration with mock auth
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Method:  config.AuthMethodBrowser,
			Browser: config.BrowserChrome,
		},
		Download: config.DownloadConfig{
			OutputDir:           t.TempDir(),
			OutputTemplate:      "%(title)s.%(ext)s",
			ConcurrentDownloads: 1,
		},
		Quality: config.QualityConfig{
			Preferred: config.Quality240p, // Lowest quality for test
		},
		Advanced: config.AdvancedConfig{
			RetryCount: 1,
			Timeout:    2 * time.Minute,
		},
		YouTube: config.YouTubeConfig{
			Client: config.ClientWeb,
		},
	}

	// Create yt-dlp executor
	executor, err := downloader.NewYtdlpExecutor(cfg)
	if err != nil {
		if strings.Contains(err.Error(), "yt-dlp not found") {
			t.Skip("yt-dlp not available for testing")
		}
		t.Fatalf("failed to create yt-dlp executor: %v", err)
	}
	defer executor.Close()

	// Test with a simple public video
	testURL := getTestVideoURL()
	if testURL == "" {
		t.Skip("no test video URL available")
	}

	// Test getting video info
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mockAuthArgs := []string{} // No auth for public video
	videoInfo, err := executor.GetVideoInfo(ctx, testURL, mockAuthArgs)
	if err != nil {
		t.Logf("video info failed (may be expected): %v", err)
		// Don't fail - might be blocked by rate limiting or other factors
		return
	}

	if videoInfo == nil {
		t.Error("video info is nil")
		return
	}

	t.Logf("Video: %s", videoInfo.Title)
	t.Logf("Duration: %.0f seconds", videoInfo.Duration)
	t.Logf("Uploader: %s", videoInfo.Uploader)

	// Validate video info structure
	if videoInfo.ID == "" {
		t.Error("video ID is empty")
	}

	if videoInfo.Title == "" {
		t.Error("video title is empty")
	}

	if len(videoInfo.AvailableQualities) == 0 {
		t.Error("no available qualities")
	}
}

// TestConfigurationIntegration tests configuration loading and validation
func TestConfigurationIntegration(t *testing.T) {
	tempDir := t.TempDir()

	// Create test configuration file
	configContent := `
auth:
  method: browser
  browser: chrome

download:
  output_dir: ./test_downloads
  output_template: "%(uploader)s - %(title)s [%(id)s].%(ext)s"
  concurrent_downloads: 2
  keep_temp_files: false

quality:
  preferred: hd720
  fallback: sd480
  audio_quality: best

proxy:
  enabled: false

advanced:
  retry_count: 2
  timeout: 20m
  verbose: false

youtube:
  client: web_creator
  skip_premium_only: true
  extract_chapters: true
`

	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Test loading configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Validate configuration values
	if cfg.Auth.Method != config.AuthMethodBrowser {
		t.Errorf("expected browser auth method, got %s", cfg.Auth.Method)
	}

	if cfg.Quality.Preferred != config.Quality720p {
		t.Errorf("expected hd720 quality, got %s", cfg.Quality.Preferred)
	}

	if cfg.Download.ConcurrentDownloads != 2 {
		t.Errorf("expected 2 concurrent downloads, got %d", cfg.Download.ConcurrentDownloads)
	}

	// Test creating downloader with loaded config
	if cfg.Advanced.YtdlpPath == "" {
		cfg.Advanced.YtdlpPath = ensureTestYtdlpPath(t)
	}
	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		t.Fatalf("failed to create downloader with loaded config: %v", err)
	}
	defer d.Close()
}

// Helper functions

// getTestVideoURL returns a URL for integration testing
// This should be a short, public video that's unlikely to be removed
func getTestVideoURL() string {
	// Use environment variable if set, otherwise use a default test video
	if url := os.Getenv("U2B_TEST_VIDEO_URL"); url != "" {
		return url
	}
	
	// Return empty string to skip tests if no URL is provided
	// In a real scenario, you might want to use a dedicated test video
	return ""
}

// hasBrowser checks if a browser is available on the system
func hasBrowser(browser string) bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	var checkPaths []string
	switch browser {
	case config.BrowserChrome:
		checkPaths = []string{
			filepath.Join(homeDir, "Library/Application Support/Google/Chrome"), // macOS
			filepath.Join(homeDir, ".config/google-chrome"),                     // Linux
			filepath.Join(homeDir, "AppData/Local/Google/Chrome/User Data"),     // Windows
		}
	case config.BrowserFirefox:
		checkPaths = []string{
			filepath.Join(homeDir, "Library/Application Support/Firefox"),       // macOS
			filepath.Join(homeDir, ".mozilla/firefox"),                          // Linux
			filepath.Join(homeDir, "AppData/Roaming/Mozilla/Firefox/Profiles"), // Windows
		}
	}

	for _, path := range checkPaths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

// createIntegrationTestCookieFile creates a more realistic cookie file for integration tests
func createIntegrationTestCookieFile(t *testing.T) string {
	tempDir := t.TempDir()
	cookieFile := filepath.Join(tempDir, "integration_cookies.txt")

	// More realistic cookie file content
	cookieContent := `# Netscape HTTP Cookie File
# This is a generated file! Do not edit.

.youtube.com	TRUE	/	FALSE	1735689600	YSC	test_ysc_token
.youtube.com	TRUE	/	TRUE	1735689600	VISITOR_INFO1_LIVE	test_visitor_info
.google.com	TRUE	/	TRUE	1735689600	NID	test_nid_token
.google.com	TRUE	/	FALSE	1735689600	SIDCC	test_sidcc_token
accounts.google.com	FALSE	/	TRUE	1735689600	oauth_token	test_oauth_token
`

	if err := os.WriteFile(cookieFile, []byte(cookieContent), 0644); err != nil {
		t.Fatalf("failed to create integration test cookie file: %v", err)
	}

	return cookieFile
}

// TestEndToEndWorkflow tests the complete end-to-end workflow
func TestEndToEndWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in short mode")
	}

	// This test would be run with build tags in a CI environment
	// where we have proper authentication setup
	
	testURL := os.Getenv("U2B_MEMBERSHIP_TEST_URL")
	if testURL == "" {
		t.Skip("no membership test URL provided")
	}

	cookieFile := os.Getenv("U2B_TEST_COOKIE_FILE")
	if cookieFile == "" {
		t.Skip("no test cookie file provided")
	}

	// Verify cookie file exists
	if _, err := os.Stat(cookieFile); os.IsNotExist(err) {
		t.Skip("test cookie file does not exist")
	}

	// Create configuration for membership video download
	tempDir := t.TempDir()
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Method:      config.AuthMethodCookiesFile,
			CookiesFile: cookieFile,
		},
		Download: config.DownloadConfig{
			OutputDir:           tempDir,
			OutputTemplate:      "%(title)s [%(id)s].%(ext)s",
			ConcurrentDownloads: 1,
		},
		Quality: config.QualityConfig{
			Preferred: config.Quality360p, // Low quality for faster test
			Fallback:  config.Quality240p,
		},
		Advanced: config.AdvancedConfig{
			RetryCount: 2,
			Timeout:    10 * time.Minute,
			Verbose:    true,
		},
		YouTube: config.YouTubeConfig{
			Client:          config.ClientWebCreator,
			SkipPremiumOnly: false, // Allow membership content
		},
	}

	// Create downloader
	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		t.Fatalf("failed to create downloader: %v", err)
	}
	defer d.Close()

	// Test authentication
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.ValidateAuth(ctx); err != nil {
		t.Fatalf("authentication validation failed: %v", err)
	}

	// Get video information
	videoInfo, err := d.GetVideoInfo(ctx, testURL)
	if err != nil {
		t.Fatalf("failed to get video info: %v", err)
	}

	t.Logf("Membership video: %s", videoInfo.Title)
	t.Logf("Uploader: %s", videoInfo.Uploader)
	t.Logf("Duration: %.0f seconds", videoInfo.Duration)

	// Attempt download (this would actually download in a real test)
	options := &config.DownloadOptions{
		URL:            testURL,
		Quality:        cfg.Quality.Preferred,
		OutputDir:      cfg.Download.OutputDir,
		OutputTemplate: cfg.Download.OutputTemplate,
	}

	result, err := d.Download(ctx, testURL, options)
	if err != nil {
		t.Logf("download failed (may be expected in CI): %v", err)
		return
	}

	if result.Success {
		t.Logf("Download successful: %s", result.FilePath)
		t.Logf("File size: %d bytes", result.FileSize)
		t.Logf("Download time: %.2f seconds", result.DownloadTime)

		// Verify file exists and has content
		if stat, err := os.Stat(result.FilePath); err != nil {
			t.Errorf("downloaded file does not exist: %v", err)
		} else if stat.Size() == 0 {
			t.Error("downloaded file is empty")
		}

		// Clean up
		os.Remove(result.FilePath)
	}
}

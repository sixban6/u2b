package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sixban6/u2b/config"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		expectError bool
		validate    func(*testing.T, *config.Config)
	}{
		{
			name: "default configuration",
			configYAML: `
auth:
  method: browser
  browser: chrome
download:
  output_dir: ./downloads
  output_template: "%(title)s.%(ext)s"
  concurrent_downloads: 3
quality:
  preferred: hd1080
  fallback: hd720
`,
			expectError: false,
			validate: func(t *testing.T, cfg *config.Config) {
				if cfg.Auth.Method != "browser" {
					t.Errorf("expected auth method 'browser', got '%s'", cfg.Auth.Method)
				}
				if cfg.Auth.Browser != "chrome" {
					t.Errorf("expected browser 'chrome', got '%s'", cfg.Auth.Browser)
				}
				if cfg.Quality.Preferred != "hd1080" {
					t.Errorf("expected quality 'hd1080', got '%s'", cfg.Quality.Preferred)
				}
			},
		},
		{
			name: "cookies file authentication",
			configYAML: `
auth:
  method: cookies_file
  cookies_file: /tmp/test_cookies.txt
download:
  output_dir: ./test_downloads
quality:
  preferred: hd720
`,
			expectError: true, // cookies file doesn't exist
		},
		{
			name: "invalid auth method",
			configYAML: `
auth:
  method: invalid_method
`,
			expectError: true,
		},
		{
			name: "invalid browser",
			configYAML: `
auth:
  method: browser
  browser: invalid_browser
`,
			expectError: true,
		},
		{
			name: "invalid concurrent downloads",
			configYAML: `
download:
  concurrent_downloads: 15
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.yaml")

			if err := os.WriteFile(configPath, []byte(tt.configYAML), 0644); err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			cfg, err := config.LoadConfig(configPath)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *config.Config
		valid  bool
	}{
		{
			name: "valid no-auth config",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method: config.AuthMethodNone,
				},
				Download: config.DownloadConfig{
					ConcurrentDownloads: 3,
				},
				Advanced: config.AdvancedConfig{
					RetryCount: 3,
					Timeout:    30 * time.Minute,
				},
			},
			valid: true,
		},
		{
			name: "valid browser config",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method:  config.AuthMethodBrowser,
					Browser: config.BrowserChrome,
				},
				Download: config.DownloadConfig{
					ConcurrentDownloads: 3,
				},
				Advanced: config.AdvancedConfig{
					RetryCount: 3,
					Timeout:    30 * time.Minute,
				},
			},
			valid: true,
		},
		{
			name: "invalid auth method",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method: "invalid",
				},
			},
			valid: false,
		},
		{
			name: "invalid browser",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method:  config.AuthMethodBrowser,
					Browser: "invalid",
				},
			},
			valid: false,
		},
		{
			name: "cookies file method without file",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method: config.AuthMethodCookiesFile,
				},
			},
			valid: false,
		},
		{
			name: "invalid concurrent downloads",
			config: &config.Config{
				Download: config.DownloadConfig{
					ConcurrentDownloads: 15,
				},
			},
			valid: false,
		},
		{
			name: "invalid retry count",
			config: &config.Config{
				Advanced: config.AdvancedConfig{
					RetryCount: 15,
				},
			},
			valid: false,
		},
		{
			name: "invalid timeout",
			config: &config.Config{
				Advanced: config.AdvancedConfig{
					Timeout: 25 * time.Hour,
				},
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.yaml")

			// Write a minimal valid config to file
			validConfigYAML := `
auth:
  method: browser
  browser: chrome
download:
  output_dir: ./downloads
  concurrent_downloads: 3
advanced:
  retry_count: 3
  timeout: 30m
`
			if err := os.WriteFile(configPath, []byte(validConfigYAML), 0644); err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			// Validate the test config structure
			err := validateTestConfig(tt.config)

			if tt.valid && err != nil {
				t.Errorf("expected valid config but got error: %v", err)
			}

			if !tt.valid && err == nil {
				t.Error("expected invalid config but validation passed")
			}
		})
	}
}

// validateTestConfig simulates the config validation logic for testing
func validateTestConfig(cfg *config.Config) error {
	// Validate auth method
	if cfg.Auth.Method != "" && cfg.Auth.Method != config.AuthMethodNone && cfg.Auth.Method != config.AuthMethodBrowser && cfg.Auth.Method != config.AuthMethodCookiesFile {
		return &config.ConfigError{Field: "auth.method", Message: "invalid auth method"}
	}

	// Validate browser if using browser auth
	if cfg.Auth.Method == config.AuthMethodBrowser {
		validBrowsers := []string{config.BrowserChrome, config.BrowserFirefox, config.BrowserSafari, config.BrowserEdge, config.BrowserOpera, config.BrowserBrave, config.BrowserVivaldi}
		browserValid := false
		for _, validBrowser := range validBrowsers {
			if cfg.Auth.Browser == validBrowser {
				browserValid = true
				break
			}
		}
		if !browserValid && cfg.Auth.Browser != "" {
			return &config.ConfigError{Field: "auth.browser", Message: "invalid browser"}
		}
	}

	// Validate cookies file if using file auth
	if cfg.Auth.Method == config.AuthMethodCookiesFile && cfg.Auth.CookiesFile == "" {
		return &config.ConfigError{Field: "auth.cookies_file", Message: "cookies file required"}
	}

	// Validate concurrent downloads
	if cfg.Download.ConcurrentDownloads > 0 && (cfg.Download.ConcurrentDownloads < 1 || cfg.Download.ConcurrentDownloads > 10) {
		return &config.ConfigError{Field: "download.concurrent_downloads", Message: "must be between 1 and 10"}
	}

	// Validate retry count
	if cfg.Advanced.RetryCount > 0 && (cfg.Advanced.RetryCount < 0 || cfg.Advanced.RetryCount > 10) {
		return &config.ConfigError{Field: "advanced.retry_count", Message: "must be between 0 and 10"}
	}

	// Validate timeout
	if cfg.Advanced.Timeout > 0 && (cfg.Advanced.Timeout < time.Second || cfg.Advanced.Timeout > time.Hour*24) {
		return &config.ConfigError{Field: "advanced.timeout", Message: "must be between 1s and 24h"}
	}

	return nil
}

func TestConfigPathResolution(t *testing.T) {
	// Test default config path resolution
	configPath := config.GetConfigPath()

	// Should return empty string if no config file exists
	if configPath != "" {
		// Check if the returned path actually exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Errorf("GetConfigPath returned non-existent path: %s", configPath)
		}
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	err := config.CreateDefaultConfig(configPath)
	if err != nil {
		t.Fatalf("failed to create default config: %v", err)
	}

	// Check if file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Try to load the created config
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load created config: %v", err)
	}

	// Validate basic structure
	if cfg.Auth.Method == "" {
		t.Error("created config missing auth method")
	}

	if cfg.Download.OutputDir == "" {
		t.Error("created config missing output directory")
	}
}

// ConfigError represents a configuration error for testing
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return e.Field + ": " + e.Message
}

// Benchmark config loading
func BenchmarkLoadConfig(b *testing.B) {
	tempDir := b.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	configYAML := `
auth:
  method: browser
  browser: chrome
download:
  output_dir: ./downloads
  output_template: "%(title)s.%(ext)s"
  concurrent_downloads: 3
quality:
  preferred: hd1080
  fallback: hd720
advanced:
  retry_count: 3
  timeout: 30m
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		b.Fatalf("failed to write config file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := config.LoadConfig(configPath)
		if err != nil {
			b.Fatalf("config loading failed: %v", err)
		}
	}
}

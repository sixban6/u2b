package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ConfigError represents a configuration validation error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error in field '%s': %s", e.Field, e.Message)
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{}
	
	// Set defaults
	setDefaults()
	
	if configPath != "" {
		// Use provided config path
		viper.SetConfigFile(configPath)
	} else {
		// Search for config in current directory and home directory
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.u2b_download")
		viper.AddConfigPath("/etc/u2b_download")
	}
	
	// Enable environment variable support
	viper.SetEnvPrefix("U2B")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	
	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, use defaults
			fmt.Fprintf(os.Stderr, "Warning: Config file not found, using defaults\n")
		} else {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}
	
	// Unmarshal config
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	
	// Validate config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	
	// Process paths
	if err := processConfigPaths(config); err != nil {
		return nil, fmt.Errorf("error processing config paths: %w", err)
	}
	
	return config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Auth defaults
	viper.SetDefault("auth.method", AuthMethodBrowser)
	viper.SetDefault("auth.browser", BrowserChrome)
	viper.SetDefault("auth.cookies_file", "")
	
	// Download defaults
	viper.SetDefault("download.output_dir", "./downloads")
	viper.SetDefault("download.output_template", "%(uploader)s - %(title)s [%(id)s].%(ext)s")
	viper.SetDefault("download.concurrent_downloads", 3)
	viper.SetDefault("download.keep_temp_files", false)
	
	// Quality defaults
	viper.SetDefault("quality.preferred", Quality1080p)
	viper.SetDefault("quality.fallback", Quality720p)
	viper.SetDefault("quality.audio_quality", "best")
	
	// Proxy defaults
	viper.SetDefault("proxy.enabled", false)
	viper.SetDefault("proxy.url", "")
	
	// Advanced defaults
	viper.SetDefault("advanced.user_agent", "")
	viper.SetDefault("advanced.retry_count", 3)
	viper.SetDefault("advanced.timeout", "30m")
	viper.SetDefault("advanced.verbose", false)
	viper.SetDefault("advanced.ytdlp_path", "")
	viper.SetDefault("advanced.extra_args", []string{})
	
	// YouTube defaults
	viper.SetDefault("youtube.client", ClientWebCreator)
	viper.SetDefault("youtube.skip_premium_only", true)
	viper.SetDefault("youtube.extract_chapters", true)
}

// validateConfig validates the loaded configuration
func validateConfig(config *Config) error {
	// Validate auth method
	if config.Auth.Method != AuthMethodBrowser && config.Auth.Method != AuthMethodCookiesFile {
		return fmt.Errorf("invalid auth method: %s (must be 'browser' or 'cookies_file')", config.Auth.Method)
	}
	
	// Validate browser if using browser auth
	if config.Auth.Method == AuthMethodBrowser {
		validBrowsers := []string{BrowserChrome, BrowserFirefox, BrowserSafari, BrowserEdge, BrowserOpera, BrowserBrave, BrowserVivaldi}
		if !contains(validBrowsers, config.Auth.Browser) {
			return fmt.Errorf("invalid browser: %s (must be one of: %s)", config.Auth.Browser, strings.Join(validBrowsers, ", "))
		}
	}
	
	// Validate cookies file if using file auth
	if config.Auth.Method == AuthMethodCookiesFile {
		if config.Auth.CookiesFile == "" {
			return fmt.Errorf("cookies_file must be specified when using 'cookies_file' auth method")
		}
		if _, err := os.Stat(config.Auth.CookiesFile); os.IsNotExist(err) {
			return fmt.Errorf("cookies file does not exist: %s", config.Auth.CookiesFile)
		}
	}
	
	// Validate concurrent downloads
	if config.Download.ConcurrentDownloads < 1 || config.Download.ConcurrentDownloads > 10 {
		return fmt.Errorf("concurrent_downloads must be between 1 and 10, got %d", config.Download.ConcurrentDownloads)
	}
	
	// Validate retry count
	if config.Advanced.RetryCount < 0 || config.Advanced.RetryCount > 10 {
		return fmt.Errorf("retry_count must be between 0 and 10, got %d", config.Advanced.RetryCount)
	}
	
	// Validate timeout
	if config.Advanced.Timeout < time.Second || config.Advanced.Timeout > time.Hour*24 {
		return fmt.Errorf("timeout must be between 1s and 24h, got %v", config.Advanced.Timeout)
	}
	
	return nil
}

// processConfigPaths processes and expands file paths in the configuration
func processConfigPaths(config *Config) error {
	var err error
	
	// Expand output directory
	config.Download.OutputDir, err = expandPath(config.Download.OutputDir)
	if err != nil {
		return fmt.Errorf("error expanding output_dir: %w", err)
	}
	
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(config.Download.OutputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %w", err)
	}
	
	// Expand cookies file path if specified
	if config.Auth.CookiesFile != "" {
		config.Auth.CookiesFile, err = expandPath(config.Auth.CookiesFile)
		if err != nil {
			return fmt.Errorf("error expanding cookies_file: %w", err)
		}
	}
	
	// Expand yt-dlp path if specified
	if config.Advanced.YtdlpPath != "" {
		config.Advanced.YtdlpPath, err = expandPath(config.Advanced.YtdlpPath)
		if err != nil {
			return fmt.Errorf("error expanding ytdlp_path: %w", err)
		}
	}
	
	return nil
}

// expandPath expands ~ to home directory and returns absolute path
func expandPath(path string) (string, error) {
	if path == "" {
		return path, nil
	}
	
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(homeDir, path[2:])
	}
	
	return filepath.Abs(path)
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetConfigPath returns the default config file path
func GetConfigPath() string {
	// Check current directory first
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	
	// Check home directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, ".u2b_download", "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}
	
	// Check system directory
	systemConfigPath := "/etc/u2b_download/config.yaml"
	if _, err := os.Stat(systemConfigPath); err == nil {
		return systemConfigPath
	}
	
	return ""
}

// CreateDefaultConfig creates a default config file at the specified path
func CreateDefaultConfig(configPath string) error {
	// Create directory if needed
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}
	
	// Read example config
	exampleConfig := filepath.Join(filepath.Dir(configPath), "config.yaml.example")
	if _, err := os.Stat(exampleConfig); os.IsNotExist(err) {
		// Example config not found, create a minimal one
		config := &Config{}
		setDefaults()
		viper.Unmarshal(config)
		
		err := viper.WriteConfigAs(configPath)
		if err != nil {
			return fmt.Errorf("error writing default config: %w", err)
		}
		return nil
	}
	
	// Copy example config
	input, err := os.ReadFile(exampleConfig)
	if err != nil {
		return fmt.Errorf("error reading example config: %w", err)
	}
	
	if err := os.WriteFile(configPath, input, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	
	return nil
}
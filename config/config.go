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

// LoadConfig loads configuration from file and environment variables.
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
	cfg := DefaultConfig()
	applyDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/u2b")
		v.AddConfigPath("$HOME/.u2b")
		v.AddConfigPath("/etc/u2b")
		v.AddConfigPath("$HOME/.u2b_download")
	}

	v.SetEnvPrefix("U2B")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Fprintf(os.Stderr, "Warning: Config file not found, using defaults\n")
		} else {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	if err := processConfigPaths(cfg); err != nil {
		return nil, fmt.Errorf("error processing config paths: %w", err)
	}

	return cfg, nil
}

// DefaultConfig returns the default configuration values.
func DefaultConfig() *Config {
	return &Config{
		Auth: AuthConfig{
			Method:      AuthMethodNone,
			Browser:     BrowserBrave,
			CookiesFile: "",
		},
		Download: DownloadConfig{
			OutputDir:           "./downloads",
			OutputTemplate:      "%(uploader)s - %(title)s [%(id)s].%(ext)s",
			ConcurrentDownloads: 3,
			KeepTempFiles:       false,
		},
		Quality: QualityConfig{
			Preferred:    Quality1080p,
			Fallback:     Quality720p,
			AudioQuality: "best",
		},
		Proxy: ProxyConfig{
			Enabled: false,
			URL:     "",
		},
		Advanced: AdvancedConfig{
			UserAgent:  "",
			RetryCount: 3,
			Timeout:    30 * time.Minute,
			Verbose:    false,
			YtdlpPath:  "",
			ExtraArgs:  []string{},
		},
		YouTube: YouTubeConfig{
			Client:          ClientWebCreator,
			SkipPremiumOnly: true,
			ExtractChapters: true,
		},
		Server: ServerConfig{
			Listen: "0.0.0.0:8989",
			Token:  "",
		},
	}
}

func applyDefaults(v *viper.Viper) {
	def := DefaultConfig()

	v.SetDefault("auth.method", def.Auth.Method)
	v.SetDefault("auth.browser", def.Auth.Browser)
	v.SetDefault("auth.cookies_file", def.Auth.CookiesFile)

	v.SetDefault("download.output_dir", def.Download.OutputDir)
	v.SetDefault("download.output_template", def.Download.OutputTemplate)
	v.SetDefault("download.concurrent_downloads", def.Download.ConcurrentDownloads)
	v.SetDefault("download.keep_temp_files", def.Download.KeepTempFiles)

	v.SetDefault("quality.preferred", def.Quality.Preferred)
	v.SetDefault("quality.fallback", def.Quality.Fallback)
	v.SetDefault("quality.audio_quality", def.Quality.AudioQuality)

	v.SetDefault("proxy.enabled", def.Proxy.Enabled)
	v.SetDefault("proxy.url", def.Proxy.URL)

	v.SetDefault("advanced.user_agent", def.Advanced.UserAgent)
	v.SetDefault("advanced.retry_count", def.Advanced.RetryCount)
	v.SetDefault("advanced.timeout", def.Advanced.Timeout.String())
	v.SetDefault("advanced.verbose", def.Advanced.Verbose)
	v.SetDefault("advanced.ytdlp_path", def.Advanced.YtdlpPath)
	v.SetDefault("advanced.extra_args", def.Advanced.ExtraArgs)

	v.SetDefault("youtube.client", def.YouTube.Client)
	v.SetDefault("youtube.skip_premium_only", def.YouTube.SkipPremiumOnly)
	v.SetDefault("youtube.extract_chapters", def.YouTube.ExtractChapters)

	v.SetDefault("server.listen", def.Server.Listen)
	v.SetDefault("server.token", def.Server.Token)
}

// validateConfig validates the loaded configuration.
func validateConfig(cfg *Config) error {
	if cfg.Auth.Method != AuthMethodNone && cfg.Auth.Method != AuthMethodBrowser && cfg.Auth.Method != AuthMethodCookiesFile {
		return fmt.Errorf("invalid auth method: %s (must be 'none', 'browser' or 'cookies_file')", cfg.Auth.Method)
	}

	if cfg.Auth.Method == AuthMethodBrowser {
		validBrowsers := []string{BrowserChrome, BrowserFirefox, BrowserSafari, BrowserEdge, BrowserOpera, BrowserBrave, BrowserVivaldi}
		if !contains(validBrowsers, cfg.Auth.Browser) {
			return fmt.Errorf("invalid browser: %s (must be one of: %s)", cfg.Auth.Browser, strings.Join(validBrowsers, ", "))
		}
	}

	if cfg.Auth.Method == AuthMethodCookiesFile {
		if cfg.Auth.CookiesFile == "" {
			return fmt.Errorf("cookies_file must be specified when using 'cookies_file' auth method")
		}
		if _, err := os.Stat(cfg.Auth.CookiesFile); os.IsNotExist(err) {
			return fmt.Errorf("cookies file does not exist: %s", cfg.Auth.CookiesFile)
		}
	}

	if cfg.Download.ConcurrentDownloads < 1 || cfg.Download.ConcurrentDownloads > 10 {
		return fmt.Errorf("concurrent_downloads must be between 1 and 10, got %d", cfg.Download.ConcurrentDownloads)
	}

	if cfg.Advanced.RetryCount < 0 || cfg.Advanced.RetryCount > 10 {
		return fmt.Errorf("retry_count must be between 0 and 10, got %d", cfg.Advanced.RetryCount)
	}

	if cfg.Advanced.Timeout < time.Second || cfg.Advanced.Timeout > time.Hour*24 {
		return fmt.Errorf("timeout must be between 1s and 24h, got %v", cfg.Advanced.Timeout)
	}

	return nil
}

// processConfigPaths processes and expands file paths in the configuration.
func processConfigPaths(cfg *Config) error {
	var err error

	if cfg.Quality.Preferred != "" {
		cfg.Quality.Preferred = NormalizeQuality(cfg.Quality.Preferred)
	}
	if cfg.Quality.Fallback != "" {
		cfg.Quality.Fallback = NormalizeQuality(cfg.Quality.Fallback)
	}

	cfg.Download.OutputDir, err = expandPath(cfg.Download.OutputDir)
	if err != nil {
		return fmt.Errorf("error expanding output_dir: %w", err)
	}
	if err := os.MkdirAll(cfg.Download.OutputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory: %w", err)
	}

	if cfg.Auth.CookiesFile != "" {
		cfg.Auth.CookiesFile, err = expandPath(cfg.Auth.CookiesFile)
		if err != nil {
			return fmt.Errorf("error expanding cookies_file: %w", err)
		}
	}

	if cfg.Advanced.YtdlpPath != "" {
		cfg.Advanced.YtdlpPath, err = expandPath(cfg.Advanced.YtdlpPath)
		if err != nil {
			return fmt.Errorf("error expanding ytdlp_path: %w", err)
		}
	}

	return nil
}

// expandPath expands ~ to home directory and returns absolute path.
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

// contains checks if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetConfigPath returns the default config file path.
func GetConfigPath() string {
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		for _, sub := range []string{".u2b", "u2b", ".u2b_download"} {
			configPath := filepath.Join(homeDir, sub, "config.yaml")
			if _, err := os.Stat(configPath); err == nil {
				return configPath
			}
		}
	}

	systemConfigPath := "/etc/u2b/config.yaml"
	if _, err := os.Stat(systemConfigPath); err == nil {
		return systemConfigPath
	}

	return ""
}

// CreateDefaultConfig creates a default config file at the specified path.
func CreateDefaultConfig(configPath string) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	exampleConfig := filepath.Join(filepath.Dir(configPath), "config.yaml.example")
	if _, err := os.Stat(exampleConfig); err == nil {
		input, readErr := os.ReadFile(exampleConfig)
		if readErr != nil {
			return fmt.Errorf("error reading example config: %w", readErr)
		}
		if writeErr := os.WriteFile(configPath, input, 0644); writeErr != nil {
			return fmt.Errorf("error writing config file: %w", writeErr)
		}
		return nil
	}

	defaultContent := `auth:
  method: none
  browser: brave
  cookies_file: ""

download:
  output_dir: ./downloads
  output_template: "%(uploader)s - %(title)s [%(id)s].%(ext)s"
  concurrent_downloads: 3
  keep_temp_files: false

quality:
  preferred: hd1080
  fallback: hd720
  audio_quality: best

proxy:
  enabled: false
  url: ""

advanced:
  user_agent: ""
  retry_count: 3
  timeout: 30m
  verbose: false
  ytdlp_path: ""
  extra_args: []

youtube:
  client: web_creator
  skip_premium_only: true
  extract_chapters: true

server:
  listen: "0.0.0.0:8989"
  token: ""
`

	if err := os.WriteFile(configPath, []byte(defaultContent), 0644); err != nil {
		return fmt.Errorf("error writing default config: %w", err)
	}

	return nil
}

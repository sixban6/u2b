package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sixban6/u2b/config"
)

func TestLoadConfig_UsesExpectedDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Minimal config file: all other values should fall back to defaults.
	content := "download:\n  output_template: \"%(title)s.%(ext)s\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Auth.Method != config.AuthMethodNone {
		t.Fatalf("expected default auth method %q, got %q", config.AuthMethodNone, cfg.Auth.Method)
	}
	if cfg.Auth.Browser != config.BrowserBrave {
		t.Fatalf("expected default browser %q, got %q", config.BrowserBrave, cfg.Auth.Browser)
	}
	if cfg.Download.ConcurrentDownloads != 3 {
		t.Fatalf("expected default concurrent_downloads 3, got %d", cfg.Download.ConcurrentDownloads)
	}
	if cfg.Quality.Preferred != config.Quality1080p {
		t.Fatalf("expected default preferred %q, got %q", config.Quality1080p, cfg.Quality.Preferred)
	}
	if cfg.Quality.Fallback != config.Quality720p {
		t.Fatalf("expected default fallback %q, got %q", config.Quality720p, cfg.Quality.Fallback)
	}
	if cfg.Advanced.Timeout != 30*time.Minute {
		t.Fatalf("expected default timeout 30m, got %v", cfg.Advanced.Timeout)
	}
	if cfg.YouTube.Client != config.ClientWebCreator {
		t.Fatalf("expected default client %q, got %q", config.ClientWebCreator, cfg.YouTube.Client)
	}
}

func TestCreateDefaultConfig_WritesUsableDefaults(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	if err := config.CreateDefaultConfig(configPath); err != nil {
		t.Fatalf("failed to create default config: %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load created config: %v", err)
	}

	if cfg.Auth.Method != config.AuthMethodNone {
		t.Fatalf("expected auth.method %q, got %q", config.AuthMethodNone, cfg.Auth.Method)
	}
	if cfg.Download.OutputDir == "" {
		t.Fatal("expected non-empty output_dir")
	}
	if cfg.Quality.Preferred != config.Quality1080p {
		t.Fatalf("expected quality.preferred %q, got %q", config.Quality1080p, cfg.Quality.Preferred)
	}
}

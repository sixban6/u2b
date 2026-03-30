package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sr.ht/~sixban/u2b_download/config"
	"git.sr.ht/~sixban/u2b_download/downloader"
)

func TestNewYouTubeDownloader(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
	}{
		{
			name: "valid no-auth config",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method: config.AuthMethodNone,
				},
				Download: config.DownloadConfig{
					OutputDir:           "./test_downloads",
					OutputTemplate:      "%(title)s.%(ext)s",
					ConcurrentDownloads: 3,
				},
				Quality: config.QualityConfig{
					Preferred: config.Quality1080p,
					Fallback:  config.Quality720p,
				},
				Advanced: config.AdvancedConfig{
					RetryCount: 3,
					Timeout:    30 * time.Minute,
				},
				YouTube: config.YouTubeConfig{
					Client: config.ClientWebCreator,
				},
			},
			expectError: false,
		},
		{
			name: "valid browser config",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method:  config.AuthMethodBrowser,
					Browser: config.BrowserChrome,
				},
				Download: config.DownloadConfig{
					OutputDir:           "./test_downloads",
					OutputTemplate:      "%(title)s.%(ext)s",
					ConcurrentDownloads: 3,
				},
				Quality: config.QualityConfig{
					Preferred: config.Quality1080p,
					Fallback:  config.Quality720p,
				},
				Advanced: config.AdvancedConfig{
					RetryCount: 3,
					Timeout:    30 * time.Minute,
				},
				YouTube: config.YouTubeConfig{
					Client: config.ClientWebCreator,
				},
			},
			expectError: false,
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "invalid auth method",
			config: &config.Config{
				Auth: config.AuthConfig{
					Method: "invalid",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := downloader.NewYouTubeDownloader(tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				if d != nil {
					d.Close()
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if d == nil {
				t.Error("expected downloader instance but got nil")
			}

			if d != nil {
				d.Close()
			}
		})
	}
}

func TestAuthProvider(t *testing.T) {
	tests := []struct {
		name        string
		authConfig  config.AuthConfig
		expectError bool
		validate    func(*testing.T, downloader.AuthProvider)
	}{
		{
			name: "no auth",
			authConfig: config.AuthConfig{
				Method: config.AuthMethodNone,
			},
			expectError: false,
			validate: func(t *testing.T, provider downloader.AuthProvider) {
				args, err := provider.GetAuthArgs()
				if err != nil {
					t.Errorf("GetAuthArgs failed: %v", err)
				}
				if len(args) != 0 {
					t.Errorf("expected no auth args, got: %v", args)
				}
			},
		},
		{
			name: "browser auth - chrome",
			authConfig: config.AuthConfig{
				Method:  config.AuthMethodBrowser,
				Browser: config.BrowserChrome,
			},
			expectError: false,
			validate: func(t *testing.T, provider downloader.AuthProvider) {
				args, err := provider.GetAuthArgs()
				if err != nil {
					t.Errorf("GetAuthArgs failed: %v", err)
				}
				if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "chrome" {
					t.Errorf("unexpected auth args: %v", args)
				}
			},
		},
		{
			name: "browser auth - firefox",
			authConfig: config.AuthConfig{
				Method:  config.AuthMethodBrowser,
				Browser: config.BrowserFirefox,
			},
			expectError: false,
			validate: func(t *testing.T, provider downloader.AuthProvider) {
				args, err := provider.GetAuthArgs()
				if err != nil {
					t.Errorf("GetAuthArgs failed: %v", err)
				}
				if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "firefox" {
					t.Errorf("unexpected auth args: %v", args)
				}
			},
		},
		{
			name: "cookies file auth - valid file",
			authConfig: config.AuthConfig{
				Method:      config.AuthMethodCookiesFile,
				CookiesFile: createTestCookieFile(t),
			},
			expectError: false,
			validate: func(t *testing.T, provider downloader.AuthProvider) {
				args, err := provider.GetAuthArgs()
				if err != nil {
					t.Errorf("GetAuthArgs failed: %v", err)
				}
				if len(args) != 2 || args[0] != "--cookies" {
					t.Errorf("unexpected auth args: %v", args)
				}
			},
		},
		{
			name: "invalid browser",
			authConfig: config.AuthConfig{
				Method:  config.AuthMethodBrowser,
				Browser: "invalid",
			},
			expectError: true,
		},
		{
			name: "cookies file auth - nonexistent file",
			authConfig: config.AuthConfig{
				Method:      config.AuthMethodCookiesFile,
				CookiesFile: "/nonexistent/cookies.txt",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := downloader.NewAuthProvider(tt.authConfig)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if provider == nil {
				t.Error("expected auth provider but got nil")
				return
			}

			if tt.validate != nil {
				tt.validate(t, provider)
			}
		})
	}
}

func TestQualitySelector(t *testing.T) {
	selector := downloader.NewQualitySelector()

	tests := []struct {
		name            string
		available       []string
		preferred       string
		fallback        string
		expectedQuality string
		expectError     bool
	}{
		{
			name:            "exact match preferred",
			available:       []string{"240", "360", "720", "1080"},
			preferred:       config.Quality1080p,
			fallback:        config.Quality720p,
			expectedQuality: "1080",
			expectError:     false,
		},
		{
			name:            "fallback to fallback quality",
			available:       []string{"240", "360", "480", "720"},
			preferred:       config.Quality1080p,
			fallback:        config.Quality720p,
			expectedQuality: "720",
			expectError:     false,
		},
		{
			name:            "closest match when exact not available",
			available:       []string{"480", "1440"},
			preferred:       config.Quality1080p,
			fallback:        config.Quality720p,
			expectedQuality: "1440",
			expectError:     false,
		},
		{
			name:            "best quality selection",
			available:       []string{"360", "720", "1080", "1440"},
			preferred:       config.QualityBest,
			fallback:        config.Quality720p,
			expectedQuality: "1440",
			expectError:     false,
		},
		{
			name:            "worst quality selection",
			available:       []string{"360", "720", "1080"},
			preferred:       config.QualityWorst,
			fallback:        config.Quality720p,
			expectedQuality: "360",
			expectError:     false,
		},
		{
			name:        "no available qualities",
			available:   []string{},
			preferred:   config.Quality1080p,
			fallback:    config.Quality720p,
			expectError: true,
		},
		{
			name:        "invalid preferred quality",
			available:   []string{"720", "1080"},
			preferred:   "invalid_quality",
			fallback:    config.Quality720p,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selectedQuality, err := selector.SelectBestQuality(tt.available, tt.preferred, tt.fallback)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if selectedQuality != tt.expectedQuality {
				t.Errorf("expected quality '%s', got '%s'", tt.expectedQuality, selectedQuality)
			}
		})
	}
}

func TestParseQuality(t *testing.T) {
	selector := downloader.NewQualitySelector()

	tests := []struct {
		name           string
		input          string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "standard quality hd1080",
			input:          config.Quality1080p,
			expectedOutput: "1080",
			expectError:    false,
		},
		{
			name:           "numeric quality",
			input:          "720",
			expectedOutput: "720",
			expectError:    false,
		},
		{
			name:           "resolution format",
			input:          "1920x1080",
			expectedOutput: "1080",
			expectError:    false,
		},
		{
			name:           "best quality",
			input:          config.QualityBest,
			expectedOutput: config.QualityBest,
			expectError:    false,
		},
		{
			name:           "worst quality",
			input:          config.QualityWorst,
			expectedOutput: config.QualityWorst,
			expectError:    false,
		},
		{
			name:        "empty quality",
			input:       "",
			expectError: true,
		},
		{
			name:        "invalid quality",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "out of range quality",
			input:       "5000",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := selector.ParseQuality(tt.input)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if output != tt.expectedOutput {
				t.Errorf("expected output '%s', got '%s'", tt.expectedOutput, output)
			}
		})
	}
}

func TestMockAuthProvider(t *testing.T) {
	authArgs := []string{"--cookies-from-browser", "chrome"}
	validateErr := fmt.Errorf("validation failed")
	cookiesInfo := "test cookies info"

	provider := downloader.NewMockAuthProvider(authArgs, validateErr, cookiesInfo)

	// Test GetAuthArgs
	args, err := provider.GetAuthArgs()
	if err != nil {
		t.Errorf("GetAuthArgs failed: %v", err)
	}
	if len(args) != len(authArgs) {
		t.Errorf("expected %d auth args, got %d", len(authArgs), len(args))
	}

	// Test ValidateAuth
	err = provider.ValidateAuth()
	if err == nil {
		t.Error("expected validation error but got none")
	}

	// Test GetCookies
	cookies, err := provider.GetCookies()
	if err != nil {
		t.Errorf("GetCookies failed: %v", err)
	}
	if cookies != cookiesInfo {
		t.Errorf("expected cookies info '%s', got '%s'", cookiesInfo, cookies)
	}
}

// createTestCookieFile creates a temporary cookie file for testing
func createTestCookieFile(t *testing.T) string {
	tempDir := t.TempDir()
	cookieFile := filepath.Join(tempDir, "cookies.txt")

	cookieContent := `# Netscape HTTP Cookie File
.youtube.com	TRUE	/	FALSE	1234567890	session_token	test_session
.google.com	TRUE	/	TRUE	1234567890	auth_token	test_auth
`

	if err := os.WriteFile(cookieFile, []byte(cookieContent), 0644); err != nil {
		t.Fatalf("failed to create test cookie file: %v", err)
	}

	return cookieFile
}

// Benchmark tests
func BenchmarkQualitySelector(b *testing.B) {
	selector := downloader.NewQualitySelector()
	available := []string{"240", "360", "480", "720", "1080", "1440", "2160"}
	preferred := config.Quality1080p
	fallback := config.Quality720p

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := selector.SelectBestQuality(available, preferred, fallback)
		if err != nil {
			b.Fatalf("quality selection failed: %v", err)
		}
	}
}

func BenchmarkParseQuality(b *testing.B) {
	selector := downloader.NewQualitySelector()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := selector.ParseQuality(config.Quality1080p)
		if err != nil {
			b.Fatalf("quality parsing failed: %v", err)
		}
	}
}

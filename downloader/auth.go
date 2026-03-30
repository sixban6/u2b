package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.sr.ht/~sixban/u2b_download/config"
)

// BrowserAuthProvider handles authentication via browser cookies
type BrowserAuthProvider struct {
	browser string
}

// CookieFileAuthProvider handles authentication via cookie files
type CookieFileAuthProvider struct {
	cookiesFile string
}

// NewAuthProvider creates a new auth provider based on configuration
func NewAuthProvider(authConfig config.AuthConfig) (AuthProvider, error) {
	switch authConfig.Method {
	case config.AuthMethodBrowser:
		return NewBrowserAuthProvider(authConfig.Browser)
	case config.AuthMethodCookiesFile:
		return NewCookieFileAuthProvider(authConfig.CookiesFile)
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", authConfig.Method)
	}
}

// NewBrowserAuthProvider creates a new browser auth provider
func NewBrowserAuthProvider(browser string) (*BrowserAuthProvider, error) {
	if browser == "" {
		browser = config.BrowserChrome
	}
	
	// Validate browser
	validBrowsers := []string{
		config.BrowserChrome, config.BrowserFirefox, config.BrowserSafari, 
		config.BrowserEdge, config.BrowserOpera, config.BrowserBrave, config.BrowserVivaldi,
	}
	
	browserValid := false
	for _, validBrowser := range validBrowsers {
		if browser == validBrowser {
			browserValid = true
			break
		}
	}
	
	if !browserValid {
		return nil, fmt.Errorf("unsupported browser: %s (supported: %s)", 
			browser, strings.Join(validBrowsers, ", "))
	}
	
	return &BrowserAuthProvider{browser: browser}, nil
}

// GetAuthArgs returns yt-dlp authentication arguments for browser cookies
func (b *BrowserAuthProvider) GetAuthArgs() ([]string, error) {
	return []string{"--cookies-from-browser", b.browser}, nil
}

// ValidateAuth validates browser cookie access
func (b *BrowserAuthProvider) ValidateAuth() error {
	// Check if browser profile directories exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	
	var profilePaths []string
	
	switch b.browser {
	case config.BrowserChrome:
		profilePaths = []string{
			filepath.Join(homeDir, "Library/Application Support/Google/Chrome"), // macOS
			filepath.Join(homeDir, ".config/google-chrome"),                     // Linux
			filepath.Join(homeDir, "AppData/Local/Google/Chrome/User Data"),     // Windows
		}
	case config.BrowserFirefox:
		profilePaths = []string{
			filepath.Join(homeDir, "Library/Application Support/Firefox"),       // macOS
			filepath.Join(homeDir, ".mozilla/firefox"),                          // Linux
			filepath.Join(homeDir, "AppData/Roaming/Mozilla/Firefox/Profiles"), // Windows
		}
	case config.BrowserSafari:
		profilePaths = []string{
			filepath.Join(homeDir, "Library/Cookies"), // macOS
		}
	case config.BrowserEdge:
		profilePaths = []string{
			filepath.Join(homeDir, "Library/Application Support/Microsoft Edge"), // macOS
			filepath.Join(homeDir, ".config/microsoft-edge"),                     // Linux
			filepath.Join(homeDir, "AppData/Local/Microsoft/Edge/User Data"),     // Windows
		}
	case config.BrowserBrave:
		profilePaths = []string{
			filepath.Join(homeDir, "Library/Application Support/BraveSoftware/Brave-Browser"), // macOS
			filepath.Join(homeDir, ".config/BraveSoftware/Brave-Browser"),                     // Linux
			filepath.Join(homeDir, "AppData/Local/BraveSoftware/Brave-Browser/User Data"),     // Windows
		}
	case config.BrowserOpera:
		profilePaths = []string{
			filepath.Join(homeDir, "Library/Application Support/com.operasoftware.Opera"), // macOS
			filepath.Join(homeDir, ".config/opera"),                                       // Linux
			filepath.Join(homeDir, "AppData/Roaming/Opera Software/Opera Stable"),         // Windows
		}
	case config.BrowserVivaldi:
		profilePaths = []string{
			filepath.Join(homeDir, "Library/Application Support/Vivaldi"), // macOS
			filepath.Join(homeDir, ".config/vivaldi"),                     // Linux
			filepath.Join(homeDir, "AppData/Local/Vivaldi/User Data"),     // Windows
		}
	}
	
	// Check if any profile path exists
	pathFound := false
	for _, path := range profilePaths {
		if _, err := os.Stat(path); err == nil {
			pathFound = true
			break
		}
	}
	
	if !pathFound {
		return fmt.Errorf("browser profile not found for %s (checked paths: %s)", 
			b.browser, strings.Join(profilePaths, ", "))
	}
	
	return nil
}

// GetCookies returns cookies information for debugging
func (b *BrowserAuthProvider) GetCookies() (string, error) {
	return fmt.Sprintf("Using cookies from browser: %s", b.browser), nil
}

// NewCookieFileAuthProvider creates a new cookie file auth provider
func NewCookieFileAuthProvider(cookiesFile string) (*CookieFileAuthProvider, error) {
	if cookiesFile == "" {
		return nil, fmt.Errorf("cookies file path cannot be empty")
	}
	
	// Expand path
	if strings.HasPrefix(cookiesFile, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cookiesFile = filepath.Join(homeDir, cookiesFile[2:])
	}
	
	// Make path absolute
	absPath, err := filepath.Abs(cookiesFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	
	return &CookieFileAuthProvider{cookiesFile: absPath}, nil
}

// GetAuthArgs returns yt-dlp authentication arguments for cookie file
func (c *CookieFileAuthProvider) GetAuthArgs() ([]string, error) {
	return []string{"--cookies", c.cookiesFile}, nil
}

// ValidateAuth validates cookie file access
func (c *CookieFileAuthProvider) ValidateAuth() error {
	// Check if file exists
	if _, err := os.Stat(c.cookiesFile); os.IsNotExist(err) {
		return fmt.Errorf("cookies file does not exist: %s", c.cookiesFile)
	}
	
	// Check if file is readable
	file, err := os.Open(c.cookiesFile)
	if err != nil {
		return fmt.Errorf("cannot read cookies file: %w", err)
	}
	defer file.Close()
	
	// Basic validation of cookie file format
	content, err := os.ReadFile(c.cookiesFile)
	if err != nil {
		return fmt.Errorf("failed to read cookies file: %w", err)
	}
	
	contentStr := string(content)
	
	// Check for Netscape format or JSON format
	isNetscape := strings.Contains(contentStr, "# Netscape HTTP Cookie File") || 
		strings.Contains(contentStr, ".youtube.com") ||
		strings.Contains(contentStr, ".google.com")
	
	isJSON := strings.HasPrefix(strings.TrimSpace(contentStr), "[") && 
		strings.HasSuffix(strings.TrimSpace(contentStr), "]")
	
	if !isNetscape && !isJSON {
		return fmt.Errorf("cookies file does not appear to be in valid Netscape or JSON format")
	}
	
	// Check for required domains
	hasYoutubeCookies := strings.Contains(contentStr, "youtube.com")
	hasGoogleCookies := strings.Contains(contentStr, "google.com") || strings.Contains(contentStr, "accounts.google.com")
	
	if !hasYoutubeCookies {
		return fmt.Errorf("cookies file does not contain YouTube cookies")
	}
	
	if !hasGoogleCookies {
		// Warning but not fatal
		fmt.Fprintf(os.Stderr, "Warning: cookies file may not contain Google account cookies\n")
	}
	
	return nil
}

// GetCookies returns cookie file path for debugging
func (c *CookieFileAuthProvider) GetCookies() (string, error) {
	return fmt.Sprintf("Using cookies from file: %s", c.cookiesFile), nil
}

// MockAuthProvider is a mock implementation for testing
type MockAuthProvider struct {
	authArgs     []string
	validateErr  error
	cookiesInfo  string
}

// NewMockAuthProvider creates a new mock auth provider for testing
func NewMockAuthProvider(authArgs []string, validateErr error, cookiesInfo string) *MockAuthProvider {
	return &MockAuthProvider{
		authArgs:     authArgs,
		validateErr:  validateErr,
		cookiesInfo:  cookiesInfo,
	}
}

// GetAuthArgs implements AuthProvider interface
func (m *MockAuthProvider) GetAuthArgs() ([]string, error) {
	return m.authArgs, nil
}

// ValidateAuth implements AuthProvider interface
func (m *MockAuthProvider) ValidateAuth() error {
	return m.validateErr
}

// GetCookies implements AuthProvider interface
func (m *MockAuthProvider) GetCookies() (string, error) {
	return m.cookiesInfo, nil
}

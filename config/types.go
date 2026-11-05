package config

import (
	"strconv"
	"strings"
	"time"
)

// Config represents the complete configuration structure
type Config struct {
	Auth     AuthConfig     `yaml:"auth" mapstructure:"auth"`
	Download DownloadConfig `yaml:"download" mapstructure:"download"`
	Quality  QualityConfig  `yaml:"quality" mapstructure:"quality"`
	Proxy    ProxyConfig    `yaml:"proxy" mapstructure:"proxy"`
	Advanced AdvancedConfig `yaml:"advanced" mapstructure:"advanced"`
	YouTube  YouTubeConfig  `yaml:"youtube" mapstructure:"youtube"`
	Server   ServerConfig   `yaml:"server" mapstructure:"server"`
}

// ServerConfig defines HTTP/Mobile server settings
type ServerConfig struct {
	Listen string `yaml:"listen" mapstructure:"listen"` // e.g. "0.0.0.0:8989"
	Token  string `yaml:"token" mapstructure:"token"`   // optional API token
}

// AuthConfig defines authentication settings
type AuthConfig struct {
	Method      string `yaml:"method" mapstructure:"method"`             // "none", "browser" or "cookies_file"
	Browser     string `yaml:"browser" mapstructure:"browser"`           // chrome, firefox, etc.
	CookiesFile string `yaml:"cookies_file" mapstructure:"cookies_file"` // path to cookie file
}

// DownloadConfig defines download behavior settings
type DownloadConfig struct {
	OutputDir           string `yaml:"output_dir" mapstructure:"output_dir"`
	OutputTemplate      string `yaml:"output_template" mapstructure:"output_template"`
	ConcurrentDownloads int    `yaml:"concurrent_downloads" mapstructure:"concurrent_downloads"`
	KeepTempFiles       bool   `yaml:"keep_temp_files" mapstructure:"keep_temp_files"`
}

// QualityConfig defines video/audio quality preferences
type QualityConfig struct {
	Preferred    string `yaml:"preferred" mapstructure:"preferred"`         // preferred video quality
	Fallback     string `yaml:"fallback" mapstructure:"fallback"`           // fallback video quality
	AudioQuality string `yaml:"audio_quality" mapstructure:"audio_quality"` // audio quality preference
}

// ProxyConfig defines proxy settings
type ProxyConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	URL     string `yaml:"url" mapstructure:"url"`
}

// AdvancedConfig defines advanced settings
type AdvancedConfig struct {
	UserAgent  string        `yaml:"user_agent" mapstructure:"user_agent"`
	RetryCount int           `yaml:"retry_count" mapstructure:"retry_count"`
	Timeout    time.Duration `yaml:"timeout" mapstructure:"timeout"`
	Verbose    bool          `yaml:"verbose" mapstructure:"verbose"`
	YtdlpPath  string        `yaml:"ytdlp_path" mapstructure:"ytdlp_path"`
	ExtraArgs  []string      `yaml:"extra_args" mapstructure:"extra_args"`
}

// YouTubeConfig defines YouTube-specific settings
type YouTubeConfig struct {
	Client          string `yaml:"client" mapstructure:"client"`
	SkipPremiumOnly bool   `yaml:"skip_premium_only" mapstructure:"skip_premium_only"`
	ExtractChapters bool   `yaml:"extract_chapters" mapstructure:"extract_chapters"`
}

// DownloadOptions represents options for a single download operation
type DownloadOptions struct {
	URL            string
	Quality        string
	Format         string
	OutputDir      string
	OutputTemplate string
	Headers        map[string]string
	Cookies        string
	UserAgent      string
	Proxy          string
	ExtraArgs      []string
}

// VideoInfo represents information about a video
type VideoInfo struct {
	ID                 string                 `json:"id"`
	Title              string                 `json:"title"`
	Description        string                 `json:"description"`
	Uploader           string                 `json:"uploader"`
	Duration           float64                `json:"duration"`
	UploadDate         string                 `json:"upload_date"`
	Formats            []FormatInfo           `json:"formats"`
	Thumbnails         []ThumbnailInfo        `json:"thumbnails"`
	AvailableQualities []string               `json:"-"` // derived from formats
	Metadata           map[string]interface{} `json:"-"` // additional metadata

	// Playlist information
	PlaylistTitle string `json:"playlist_title"`
	PlaylistID    string `json:"playlist_id"`
	PlaylistIndex int    `json:"playlist_index"`
	PlaylistCount int    `json:"playlist_count"`
}

// FormatInfo represents a video format
type FormatInfo struct {
	FormatID   string      `json:"format_id"`
	URL        string      `json:"url"`
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	FPS        float64     `json:"fps"`
	VCodec     string      `json:"vcodec"`
	ACodec     string      `json:"acodec"`
	Filesize   int64       `json:"filesize"`
	Quality    interface{} `json:"quality"` // Can be string or number
	Resolution string      `json:"resolution"`
}

// ThumbnailInfo represents a thumbnail
type ThumbnailInfo struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// AuthMethod constants
const (
	AuthMethodNone        = "none"
	AuthMethodBrowser     = "browser"
	AuthMethodCookiesFile = "cookies_file"
)

// Supported browsers
const (
	BrowserChrome  = "chrome"
	BrowserFirefox = "firefox"
	BrowserSafari  = "safari"
	BrowserEdge    = "edge"
	BrowserOpera   = "opera"
	BrowserBrave   = "brave"
	BrowserVivaldi = "vivaldi"
)

// Video quality constants
const (
	Quality2160p = "hd2160"
	Quality1440p = "hd1440"
	Quality1080p = "hd1080"
	Quality720p  = "hd720"
	Quality480p  = "sd480"
	Quality360p  = "sd360"
	Quality240p  = "sd240"
	QualityBest  = "best"
	QualityWorst = "worst"
)

// NormalizeQuality maps human-friendly aliases (e.g. "4k", "2k", "1080p", "720p", "480p", "auto")
// to canonical quality identifiers while preserving raw numeric or resolution strings.
func NormalizeQuality(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "best", "auto", "max", "highest", "original", "原画":
		return QualityBest
	case "worst", "lowest":
		return QualityWorst
	case "4k", "2160p", "uhd", Quality2160p:
		return Quality2160p
	case "2k", "1440p", "qhd", Quality1440p:
		return Quality1440p
	case "1080p", "fhd", "fullhd", Quality1080p:
		return Quality1080p
	case "720p", "hd", Quality720p:
		return Quality720p
	case "480p", "sd", Quality480p:
		return Quality480p
	case "360p", Quality360p:
		return Quality360p
	case "240p", Quality240p:
		return Quality240p
	case "8k", "4320p":
		return "4320"
	}
	if strings.HasSuffix(lower, "p") {
		numPart := strings.TrimSuffix(lower, "p")
		if _, err := strconv.Atoi(numPart); err == nil {
			return numPart
		}
	}
	return trimmed
}

// QualityToHeight maps canonical or numeric quality strings to pixel height (e.g. "hd2160" -> "2160").
// Returns ("", false) for "best" / "worst" or unparseable strings.
func QualityToHeight(raw string) (string, bool) {
	norm := NormalizeQuality(raw)
	switch norm {
	case Quality2160p:
		return "2160", true
	case Quality1440p:
		return "1440", true
	case Quality1080p:
		return "1080", true
	case Quality720p:
		return "720", true
	case Quality480p:
		return "480", true
	case Quality360p:
		return "360", true
	case Quality240p:
		return "240", true
	case QualityBest, QualityWorst, "":
		return "", false
	}
	if h, err := strconv.Atoi(norm); err == nil && h >= 240 && h <= 4320 {
		return strconv.Itoa(h), true
	}
	if strings.Contains(norm, "x") {
		parts := strings.Split(norm, "x")
		if len(parts) == 2 {
			if h, err := strconv.Atoi(parts[1]); err == nil && h >= 240 && h <= 4320 {
				return strconv.Itoa(h), true
			}
		}
	}
	return "", false
}

// QualityDisplayLabel returns a concise user-facing label for notifications and badges.
func QualityDisplayLabel(raw string) string {
	switch NormalizeQuality(raw) {
	case QualityBest:
		return "原画最高"
	case Quality2160p, "2160":
		return "原画 4K"
	case Quality1440p, "1440":
		return "2K"
	case Quality1080p, "1080", "":
		return "1080p"
	case Quality720p, "720":
		return "720p"
	case Quality480p, "480":
		return "480p"
	case Quality360p, "360":
		return "360p"
	case Quality240p, "240":
		return "240p"
	default:
		if h, ok := QualityToHeight(raw); ok {
			return h + "p"
		}
		return raw
	}
}

// Output format constants
const (
	OutputFormatMP3 = "mp3"
)

// YouTube client types
const (
	ClientTV         = "tv"
	ClientWeb        = "web"
	ClientWebCreator = "web_creator"
	ClientAndroid    = "android"
	ClientIOS        = "ios"
)

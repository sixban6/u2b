package config

import "time"

// Config represents the complete configuration structure
type Config struct {
	Auth     AuthConfig     `yaml:"auth" mapstructure:"auth"`
	Download DownloadConfig `yaml:"download" mapstructure:"download"`
	Quality  QualityConfig  `yaml:"quality" mapstructure:"quality"`
	Proxy    ProxyConfig    `yaml:"proxy" mapstructure:"proxy"`
	Advanced AdvancedConfig `yaml:"advanced" mapstructure:"advanced"`
	YouTube  YouTubeConfig  `yaml:"youtube" mapstructure:"youtube"`
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

// YouTube client types
const (
	ClientTV         = "tv"
	ClientWeb        = "web"
	ClientWebCreator = "web_creator"
	ClientAndroid    = "android"
	ClientIOS        = "ios"
)

package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var httpURLInTextPattern = regexp.MustCompile(`https?://[A-Za-z0-9\-._~:/?#\[\]@!$&()*+,;=%]+`)

// AllowedMediaDomains lists mainstream Chinese and international video / short-video root domains.
// Subdomains of these entries (e.g. www.youtube.com, m.bilibili.com, v.douyin.com, vm.tiktok.com) are allowed.
var AllowedMediaDomains = []string{
	// US / International Video & Short-Video Platforms
	"youtube.com",
	"youtu.be",
	"youtube-nocookie.com",
	"x.com",
	"twitter.com",
	"tiktok.com",
	"instagram.com",
	"instagr.am",
	"facebook.com",
	"fb.watch",
	"threads.net",
	"vimeo.com",
	"twitch.tv",
	"reddit.com",
	"redd.it",
	"dailymotion.com",

	// Chinese Video & Short-Video Platforms
	"bilibili.com",
	"b23.tv",
	"douyin.com",
	"iesdouyin.com",
	"kuaishou.com",
	"chenzhongtech.com",
	"xiaohongshu.com",
	"xhslink.com",
	"weibo.com",
	"weibo.cn",
	"ixigua.com",
	"acfun.cn",
	"youku.com",
	"v.qq.com",
	"iqiyi.com",
	"mgtv.com",
	"tv.sohu.com",
	"haokan.baidu.com",
	"zhihu.com",
	"cctv.com",
	"yangshipin.cn",
}

const trailingSharePunctuation = "，。！？；：、）】》」』”’.,!?;:)]}"

// ExtractMediaURL extracts a clean HTTP(S) URL from raw input or app-copied share text
// (e.g. Bilibili / Douyin / Xiaohongshu share snippets) and removes shell-escaped backslashes.
func ExtractMediaURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		cleaned := strings.ReplaceAll(trimmed, `\`, "")
		// If there is no whitespace, return directly
		if !strings.ContainsAny(cleaned, " \t\r\n") {
			return strings.TrimRight(cleaned, trailingSharePunctuation)
		}
		trimmed = cleaned
	}

	match := httpURLInTextPattern.FindString(trimmed)
	if match == "" {
		return trimmed
	}
	match = strings.ReplaceAll(match, `\`, "")
	return strings.TrimRight(match, trailingSharePunctuation)
}

// ValidateAllowedMediaURL extracts, normalizes, and verifies that the URL points to an
// allowlisted video/short-video platform (blocking SSRF, local IPs, and arbitrary file URLs).
func ValidateAllowedMediaURL(raw string) (string, error) {
	extracted := ExtractMediaURL(raw)
	if extracted == "" {
		return "", fmt.Errorf("视频链接不能为空")
	}

	parsed, err := url.Parse(extracted)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("无效的视频链接格式: %q", raw)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("仅支持 http/https 视频链接")
	}

	if parsed.User != nil {
		return "", fmt.Errorf("拒绝包含认证信息的链接")
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return "", fmt.Errorf("无效的链接域名")
	}

	// Block all IP literals and local/intranet hostnames (SSRF protection)
	if net.ParseIP(host) != nil ||
		host == "localhost" ||
		strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".lan") ||
		strings.HasSuffix(host, ".arpa") {
		return "", fmt.Errorf("安全拦截：禁止访问本地或内网地址 (%s)", host)
	}

	if !IsAllowedMediaHost(host) {
		return "", fmt.Errorf("安全拦截：暂不支持域名 %q（仅允许 YouTube、B站、抖音、快手、小红书、TikTok、X 等主流视频平台）", host)
	}

	return parsed.String(), nil
}

// IsAllowedMediaHost checks whether host matches AllowedMediaDomains or a subdomain thereof.
func IsAllowedMediaHost(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if h == "" {
		return false
	}
	for _, domain := range AllowedMediaDomains {
		if h == domain || strings.HasSuffix(h, "."+domain) {
			return true
		}
	}
	return false
}

// IsYouTubeURL reports whether rawURL belongs to YouTube domains.
func IsYouTubeURL(rawURL string) bool {
	parsed, err := url.Parse(ExtractMediaURL(rawURL))
	if err != nil {
		return false
	}
	h := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	for _, yt := range []string{"youtube.com", "youtu.be", "youtube-nocookie.com"} {
		if h == yt || strings.HasSuffix(h, "."+yt) {
			return true
		}
	}
	return false
}

package cmd

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"git.sr.ht/~sixban/u2b_download/config"
	"git.sr.ht/~sixban/u2b_download/downloader"
	"github.com/spf13/cobra"
)

var nativeHostCmd = &cobra.Command{
	Use:    "native-host",
	Short:  "Internal Chromium/Brave Native Messaging Host endpoint",
	Hidden: true,
	Args:   cobra.ArbitraryArgs,
	RunE:   runNativeHost,
}

type PresetDir struct {
	Label    string `json:"label"`
	Path     string `json:"path"`
	IsCustom bool   `json:"isCustom,omitempty"`
}

type ExtensionPresetsConfig struct {
	VideoRootDir    string      `json:"videoRootDir"`
	AudioRootDir    string      `json:"audioRootDir"`
	DefaultVideoDir string      `json:"defaultVideoDir"`
	DefaultAudioDir string      `json:"defaultAudioDir"`
	CustomVideoDirs []PresetDir `json:"customVideoDirs,omitempty"`
	CustomAudioDirs []PresetDir `json:"customAudioDirs,omitempty"`
	HiddenDirs      []string    `json:"hiddenDirs,omitempty"`
	VideoDirs       []PresetDir `json:"videoDirs"`
	AudioDirs       []PresetDir `json:"audioDirs"`
}

type nativeIncomingMessage struct {
	Type       string                  `json:"type"`
	ID         string                  `json:"id"`
	RequestID  string                  `json:"requestId"`
	URL        string                  `json:"url"`
	Title      string                  `json:"title"`
	Format     string                  `json:"format"`
	Quality    string                  `json:"quality"`
	OutputDir  string                  `json:"outputDir"`
	Filename   string                  `json:"filename"`
	IsPlaylist bool                    `json:"isPlaylist"`
	FilePath   string                  `json:"filePath"`
	Prompt     string                  `json:"prompt"`
	MediaType  string                  `json:"mediaType"`
	Path       string                  `json:"path"`
	Name       string                  `json:"name"`
	Config     *ExtensionPresetsConfig `json:"config"`
}

func init() {
	rootCmd.AddCommand(nativeHostCmd)
}

func ensureMacGUIPath() {
	currentPath := os.Getenv("PATH")
	homeDir, _ := os.UserHomeDir()
	extraPaths := []string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
	}
	if homeDir != "" {
		extraPaths = append(extraPaths,
			filepath.Join(homeDir, ".local", "bin"),
			filepath.Join(homeDir, "go", "bin"),
		)
	}
	parts := strings.Split(currentPath, string(os.PathListSeparator))
	existing := make(map[string]bool, len(parts))
	for _, p := range parts {
		existing[p] = true
	}
	for _, p := range extraPaths {
		if !existing[p] {
			parts = append([]string{p}, parts...)
		}
	}
	_ = os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
}

func runNativeHost(cmd *cobra.Command, args []string) error {
	ensureMacGUIPath()

	// Keep dedicated reference to real stdout for Native Messaging framing,
	// and redirect os.Stdout to os.Stderr so library prints never corrupt framing.
	hostOut := os.Stdout
	os.Stdout = os.Stderr

	var writeMu sync.Mutex
	sendMsg := func(payload any) {
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		writeMu.Lock()
		defer writeMu.Unlock()

		var lenBuf [4]byte
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(data)))
		_, _ = hostOut.Write(lenBuf[:])
		_, _ = hostOut.Write(data)
	}

	var tasksMu sync.Mutex
	cancelMap := make(map[string]context.CancelFunc)
	var activeWg sync.WaitGroup

	// Send initial config immediately on connect
	sendConfig(sendMsg)

	for {
		var msgLen uint32
		if err := binary.Read(os.Stdin, binary.LittleEndian, &msgLen); err != nil {
			if err == io.EOF {
				break
			}
			return nil
		}

		if msgLen == 0 || msgLen > 10*1024*1024 {
			continue
		}

		buf := make([]byte, msgLen)
		if _, err := io.ReadFull(os.Stdin, buf); err != nil {
			break
		}

		var msg nativeIncomingMessage
		if err := json.Unmarshal(buf, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "ping", "get_config":
			sendConfig(sendMsg)

		case "save_presets":
			if msg.Config != nil {
				_ = saveExtensionPresets(msg.Config)
			}
			sendConfig(sendMsg)

		case "set_root_dir":
			go func(m nativeIncomingMessage) {
				targetPath := strings.TrimSpace(m.Path)
				if targetPath == "" {
					prompt := "选择视频库根目录（将自动扫描其下所有子文件夹）"
					if m.MediaType == "audio" {
						prompt = "选择音乐库根目录（将自动扫描其下所有子文件夹）"
					}
					targetPath = pickFolderDialog(prompt)
				}
				if targetPath != "" {
					presets := loadRawExtensionPresets()
					if m.MediaType == "audio" {
						presets.AudioRootDir = targetPath
						presets.DefaultAudioDir = resolveDefaultSubdir(targetPath)
					} else {
						presets.VideoRootDir = targetPath
						presets.DefaultVideoDir = resolveDefaultSubdir(targetPath)
					}
					_ = saveExtensionPresets(presets)
				}
				sendConfig(sendMsg)
			}(msg)

		case "create_category":
			catName := sanitizeFolderName(msg.Name)
			if catName != "" {
				presets := loadRawExtensionPresets()
				rootDir := presets.VideoRootDir
				if msg.MediaType == "audio" {
					rootDir = presets.AudioRootDir
				}
				if rootDir != "" {
					newDirPath := filepath.Join(rootDir, catName)
					_ = os.MkdirAll(newDirPath, 0755)
					presets.HiddenDirs = removeString(presets.HiddenDirs, newDirPath)
					_ = saveExtensionPresets(presets)
					sendMsg(map[string]any{
						"type":        "category_created",
						"requestId":   msg.RequestID,
						"mediaType":   msg.MediaType,
						"name":        catName,
						"createdPath": newDirPath,
					})
				}
			}
			sendConfig(sendMsg)

		case "remove_dir":
			targetPath := strings.TrimSpace(msg.Path)
			if targetPath != "" {
				presets := loadRawExtensionPresets()
				presets.CustomVideoDirs = filterOutPresetDir(presets.CustomVideoDirs, targetPath)
				presets.CustomAudioDirs = filterOutPresetDir(presets.CustomAudioDirs, targetPath)
				if !containsString(presets.HiddenDirs, targetPath) {
					presets.HiddenDirs = append(presets.HiddenDirs, targetPath)
				}
				_ = saveExtensionPresets(presets)
			}
			sendConfig(sendMsg)

		case "add_custom_dir":
			go func(m nativeIncomingMessage) {
				targetPath := strings.TrimSpace(m.Path)
				if targetPath == "" {
					prompt := "选择要添加到右键菜单的文件夹"
					targetPath = pickFolderDialog(prompt)
				}
				if targetPath != "" {
					_ = os.MkdirAll(targetPath, 0755)
					label := strings.TrimSpace(m.Name)
					if label == "" {
						label = filepath.Base(targetPath)
					}
					presets := loadRawExtensionPresets()
					presets.HiddenDirs = removeString(presets.HiddenDirs, targetPath)
					item := PresetDir{Label: label, Path: targetPath, IsCustom: true}
					if m.MediaType == "audio" {
						presets.CustomAudioDirs = appendUniquePresetDir(presets.CustomAudioDirs, item)
					} else {
						presets.CustomVideoDirs = appendUniquePresetDir(presets.CustomVideoDirs, item)
					}
					_ = saveExtensionPresets(presets)
				}
				sendConfig(sendMsg)
			}(msg)

		case "cancel":
			if msg.ID != "" {
				tasksMu.Lock()
				if cancel, ok := cancelMap[msg.ID]; ok {
					cancel()
					delete(cancelMap, msg.ID)
				}
				tasksMu.Unlock()
			}

		case "reveal_in_finder":
			go revealFileInOS(msg.FilePath)

		case "pick_folder":
			go func(reqID, prompt string) {
				picked := pickFolderDialog(prompt)
				sendMsg(map[string]any{
					"type":      "folder_picked",
					"requestId": reqID,
					"path":      picked,
				})
			}(msg.RequestID, msg.Prompt)

		case "download":
			activeWg.Add(1)
			go func(m nativeIncomingMessage) {
				defer activeWg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
				tasksMu.Lock()
				cancelMap[m.ID] = cancel
				tasksMu.Unlock()

				defer func() {
					tasksMu.Lock()
					delete(cancelMap, m.ID)
					tasksMu.Unlock()
					cancel()
				}()

				handleNativeDownload(ctx, m, sendMsg)
			}(msg)
		}
	}

	activeWg.Wait()
	return nil
}

func handleNativeDownload(ctx context.Context, msg nativeIncomingMessage, sendMsg func(any)) {
	url := sanitizeInputURL(msg.URL)
	if url == "" {
		sendMsg(map[string]any{
			"type":  "error",
			"id":    msg.ID,
			"error": "URL 不能为空",
		})
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	presets := loadExtensionPresets()
	outputDir := strings.TrimSpace(msg.OutputDir)
	if outputDir == "" {
		if strings.EqualFold(msg.Format, config.OutputFormatMP3) {
			outputDir = presets.DefaultAudioDir
		} else {
			outputDir = presets.DefaultVideoDir
		}
	}
	if expanded, err := filepath.Abs(outputDir); err == nil {
		outputDir = expanded
	}
	_ = os.MkdirAll(outputDir, 0755)
	cfg.Download.OutputDir = outputDir

	// Refresh config so if outputDir is a newly created subfolder under rootDir, it appears in menus right away!
	sendConfig(sendMsg)

	sendMsg(map[string]any{
		"type":       "started",
		"id":         msg.ID,
		"url":        url,
		"title":      msg.Title,
		"format":     msg.Format,
		"outputDir":  outputDir,
		"status":     "starting",
		"percentage": 0,
	})

	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		sendMsg(map[string]any{
			"type":  "error",
			"id":    msg.ID,
			"error": fmt.Sprintf("初始化下载器失败: %v", err),
		})
		return
	}
	defer d.Close()

	var lastSent time.Time
	d.SetProgressCallback(func(progress *downloader.DownloadProgress) {
		if progress == nil {
			return
		}
		if time.Since(lastSent) < 180*time.Millisecond && progress.Percentage < 99.9 {
			return
		}
		lastSent = time.Now()
		sendMsg(map[string]any{
			"type":       "progress",
			"id":         msg.ID,
			"status":     "downloading",
			"percentage": progress.Percentage,
			"speed":      progress.Speed,
			"eta":        progress.ETA,
			"outputDir":  outputDir,
		})
	})

	selectedFormat := ""
	if strings.EqualFold(msg.Format, config.OutputFormatMP3) {
		selectedFormat = config.OutputFormatMP3
	}

	selectedQuality := cfg.Quality.Preferred
	if msg.Quality != "" {
		selectedQuality = msg.Quality
	}

	if msg.IsPlaylist {
		options := &config.DownloadOptions{
			URL:            url,
			Quality:        selectedQuality,
			Format:         selectedFormat,
			OutputDir:      outputDir,
			OutputTemplate: "%(playlist_index)02d - %(title)s [%(id)s].%(ext)s",
			ExtraArgs:      cfg.Advanced.ExtraArgs,
		}
		if err := downloadPlaylistDirect(ctx, cfg, url, outputDir, options); err != nil {
			sendMsg(map[string]any{
				"type":  "error",
				"id":    msg.ID,
				"error": err.Error(),
			})
			return
		}
		sendMsg(map[string]any{
			"type":      "complete",
			"id":        msg.ID,
			"title":     msg.Title,
			"filePath":  outputDir,
			"outputDir": outputDir,
		})
		return
	}

	outputTemplate := cfg.Download.OutputTemplate
	if strings.TrimSpace(msg.Filename) != "" {
		if customTpl, err := BuildOutputTemplateForFilename(msg.Filename); err == nil {
			outputTemplate = customTpl
		}
	}

	options := &config.DownloadOptions{
		URL:            url,
		Quality:        selectedQuality,
		Format:         selectedFormat,
		OutputDir:      outputDir,
		OutputTemplate: outputTemplate,
		ExtraArgs:      cfg.Advanced.ExtraArgs,
	}

	result, err := d.Download(ctx, url, options)
	if err != nil {
		sendMsg(map[string]any{
			"type":  "error",
			"id":    msg.ID,
			"error": err.Error(),
		})
		return
	}

	finalTitle := msg.Title
	if result.VideoInfo != nil && result.VideoInfo.Title != "" {
		finalTitle = result.VideoInfo.Title
	}
	if strings.TrimSpace(msg.Filename) != "" {
		finalTitle = strings.TrimSpace(msg.Filename)
	}

	sendMsg(map[string]any{
		"type":      "complete",
		"id":        msg.ID,
		"title":     finalTitle,
		"filePath":  result.FilePath,
		"fileSize":  result.FileSize,
		"outputDir": outputDir,
		"duration":  result.DownloadTime,
	})
}

func sendConfig(sendMsg func(any)) {
	presets := loadExtensionPresets()
	sendMsg(map[string]any{
		"type":            "config",
		"version":         "1.0.0",
		"videoRootDir":    presets.VideoRootDir,
		"audioRootDir":    presets.AudioRootDir,
		"defaultVideoDir": presets.DefaultVideoDir,
		"defaultAudioDir": presets.DefaultAudioDir,
		"videoDirs":       presets.VideoDirs,
		"audioDirs":       presets.AudioDirs,
	})
}

func presetsFilePath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "u2b", "presets.json")
}

func loadRawExtensionPresets() *ExtensionPresetsConfig {
	path := presetsFilePath()
	if data, err := os.ReadFile(path); err == nil {
		var cfg ExtensionPresetsConfig
		if err := json.Unmarshal(data, &cfg); err == nil && (cfg.VideoRootDir != "" || cfg.AudioRootDir != "") {
			return &cfg
		}
	}
	return buildDefaultExtensionPresets()
}

// loadExtensionPresets loads the saved root/custom config and dynamically rescans subdirectories on every call.
func loadExtensionPresets() *ExtensionPresetsConfig {
	cfg := loadRawExtensionPresets()
	if cfg.VideoRootDir == "" || cfg.AudioRootDir == "" {
		def := buildDefaultExtensionPresets()
		if cfg.VideoRootDir == "" {
			cfg.VideoRootDir = def.VideoRootDir
		}
		if cfg.AudioRootDir == "" {
			cfg.AudioRootDir = def.AudioRootDir
		}
		if cfg.DefaultVideoDir == "" {
			cfg.DefaultVideoDir = def.DefaultVideoDir
		}
		if cfg.DefaultAudioDir == "" {
			cfg.DefaultAudioDir = def.DefaultAudioDir
		}
	}

	hiddenSet := make(map[string]bool, len(cfg.HiddenDirs))
	for _, h := range cfg.HiddenDirs {
		hiddenSet[h] = true
	}

	cfg.VideoDirs = scanAndMergeDirs(cfg.VideoRootDir, cfg.DefaultVideoDir, "视频默认 (u2b)", cfg.CustomVideoDirs, hiddenSet)
	cfg.AudioDirs = scanAndMergeDirs(cfg.AudioRootDir, cfg.DefaultAudioDir, "音乐默认 (u2b)", cfg.CustomAudioDirs, hiddenSet)
	return cfg
}

func saveExtensionPresets(cfg *ExtensionPresetsConfig) error {
	path := presetsFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// detectDefaultMediaRoots dynamically probes mounted external volumes and OS standard media directories
// without hardcoding any specific user volume name.
func detectDefaultMediaRoots() (videoRoot, audioRoot string) {
	homeDir, _ := os.UserHomeDir()

	// 1. Check mounted external volumes on macOS (/Volumes/*)
	if runtime.GOOS == "darwin" {
		if entries, err := os.ReadDir("/Volumes"); err == nil {
			videoCandidates := []string{"视频", "Videos", "Video"}
			audioCandidates := []string{"music", "Music", "音乐", "Audio"}

			for _, entry := range entries {
				name := entry.Name()
				if strings.HasPrefix(name, ".") || name == "Macintosh HD" || name == "Recovery" {
					continue
				}
				volPath := filepath.Join("/Volumes", name)
				if videoRoot == "" {
					for _, vc := range videoCandidates {
						cand := filepath.Join(volPath, vc)
						if info, err := os.Stat(cand); err == nil && info.IsDir() {
							videoRoot = cand
							break
						}
					}
				}
				if audioRoot == "" {
					for _, ac := range audioCandidates {
						cand := filepath.Join(volPath, ac)
						if info, err := os.Stat(cand); err == nil && info.IsDir() {
							audioRoot = cand
							break
						}
					}
				}
			}
		}
	}

	// 2. Fallback to standard OS media folders
	if videoRoot == "" {
		moviesDir := filepath.Join(homeDir, "Movies")
		videosDir := filepath.Join(homeDir, "Videos")
		if info, err := os.Stat(moviesDir); err == nil && info.IsDir() {
			videoRoot = filepath.Join(moviesDir, "u2b")
		} else if info, err := os.Stat(videosDir); err == nil && info.IsDir() {
			videoRoot = filepath.Join(videosDir, "u2b")
		} else {
			videoRoot = filepath.Join(homeDir, "Downloads", "u2b", "videos")
		}
		_ = os.MkdirAll(videoRoot, 0755)
	}

	if audioRoot == "" {
		musicDir := filepath.Join(homeDir, "Music")
		if info, err := os.Stat(musicDir); err == nil && info.IsDir() {
			audioRoot = filepath.Join(musicDir, "u2b")
		} else {
			audioRoot = filepath.Join(homeDir, "Downloads", "u2b", "music")
		}
		_ = os.MkdirAll(audioRoot, 0755)
	}

	return videoRoot, audioRoot
}

func resolveDefaultSubdir(rootDir string) string {
	u2bSub := filepath.Join(rootDir, "u2b")
	if info, err := os.Stat(u2bSub); err == nil && info.IsDir() {
		return u2bSub
	}
	if strings.EqualFold(filepath.Base(rootDir), "u2b") {
		return rootDir
	}
	return u2bSub
}

func buildDefaultExtensionPresets() *ExtensionPresetsConfig {
	videoRoot, audioRoot := detectDefaultMediaRoots()
	defaultVideo := resolveDefaultSubdir(videoRoot)
	defaultAudio := resolveDefaultSubdir(audioRoot)

	cfg := &ExtensionPresetsConfig{
		VideoRootDir:    videoRoot,
		AudioRootDir:    audioRoot,
		DefaultVideoDir: defaultVideo,
		DefaultAudioDir: defaultAudio,
	}
	cfg.VideoDirs = scanAndMergeDirs(videoRoot, defaultVideo, "视频默认 (u2b)", nil, nil)
	cfg.AudioDirs = scanAndMergeDirs(audioRoot, defaultAudio, "音乐默认 (u2b)", nil, nil)
	return cfg
}

func scanAndMergeDirs(rootDir, defaultDir, defaultLabel string, customDirs []PresetDir, hiddenSet map[string]bool) []PresetDir {
	var result []PresetDir
	seen := make(map[string]bool)

	if defaultDir != "" && !hiddenSet[defaultDir] {
		result = append(result, PresetDir{
			Label: defaultLabel,
			Path:  defaultDir,
		})
		seen[defaultDir] = true
	}

	if entries, err := os.ReadDir(rootDir); err == nil {
		var names []string
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			full := filepath.Join(rootDir, name)
			if seen[full] || hiddenSet[full] {
				continue
			}
			seen[full] = true
			result = append(result, PresetDir{
				Label: name,
				Path:  full,
			})
		}
	}

	for _, c := range customDirs {
		if c.Path == "" || seen[c.Path] || hiddenSet[c.Path] {
			continue
		}
		seen[c.Path] = true
		result = append(result, PresetDir{
			Label:    c.Label,
			Path:     c.Path,
			IsCustom: true,
		})
	}

	return result
}

func appendUniquePresetDir(list []PresetDir, item PresetDir) []PresetDir {
	for i, existing := range list {
		if existing.Path == item.Path {
			list[i] = item
			return list
		}
	}
	return append(list, item)
}

func filterOutPresetDir(list []PresetDir, targetPath string) []PresetDir {
	var out []PresetDir
	for _, item := range list {
		if item.Path != targetPath {
			out = append(out, item)
		}
	}
	return out
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func removeString(list []string, target string) []string {
	var out []string
	for _, s := range list {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}

func revealFileInOS(targetPath string) {
	cleaned := strings.TrimSpace(targetPath)
	if cleaned == "" {
		return
	}

	info, err := os.Stat(cleaned)
	if err != nil {
		dir := filepath.Dir(cleaned)
		if _, dErr := os.Stat(dir); dErr == nil {
			cleaned = dir
			info, _ = os.Stat(cleaned)
		} else {
			return
		}
	}

	switch runtime.GOOS {
	case "darwin":
		if info != nil && !info.IsDir() {
			_ = exec.Command("open", "-R", cleaned).Start()
		} else {
			_ = exec.Command("open", cleaned).Start()
		}
	case "linux":
		dir := cleaned
		if info != nil && !info.IsDir() {
			dir = filepath.Dir(cleaned)
		}
		_ = exec.Command("xdg-open", dir).Start()
	case "windows":
		if info != nil && !info.IsDir() {
			_ = exec.Command("explorer.exe", "/select,", cleaned).Start()
		} else {
			_ = exec.Command("explorer.exe", cleaned).Start()
		}
	}
}

func pickFolderDialog(prompt string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "选择保存目录"
	}
	escaped := strings.ReplaceAll(prompt, `"`, `\"`)
	script := fmt.Sprintf(`POSIX path of (choose folder with prompt "%s")`, escaped)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(string(out)), "/")
}

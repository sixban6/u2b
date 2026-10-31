package cmd

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed extension_assets/*
var embeddedExtensionAssets embed.FS

const (
	nativeHostName           = "com.sixban.u2b"
	deterministicExtensionID = "cffnjjfomkglkjmdablieebnpdljcpfd"
)

var extNoOpen bool

var extCmd = &cobra.Command{
	Use:     "ext",
	Aliases: []string{"extension", "plugin"},
	Short:   "Install or manage the u2b Brave/Chrome browser extension",
	Long:    `Install and configure the u2b browser extension and Native Messaging Host for Brave/Chrome.`,
	RunE:    runExtInstall,
}

var extInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the u2b browser extension and register Brave Native Messaging Host",
	RunE:  runExtInstall,
}

func init() {
	extCmd.PersistentFlags().BoolVar(&extNoOpen, "no-open", false, "do not automatically open Brave and Finder after installation")
	extCmd.AddCommand(extInstallCmd)
	rootCmd.AddCommand(extCmd)
}

func runExtInstall(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to resolve home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, "u2b")
	extDir := filepath.Join(baseDir, "extension")
	if err := os.MkdirAll(extDir, 0755); err != nil {
		return fmt.Errorf("failed to create extension directory: %w", err)
	}

	// 1. Write embedded extension files
	entries, err := fs.ReadDir(embeddedExtensionAssets, "extension_assets")
	if err != nil {
		return fmt.Errorf("failed to read embedded extension assets: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := embeddedExtensionAssets.ReadFile(path.Join("extension_assets", entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read asset %s: %w", entry.Name(), err)
		}
		target := filepath.Join(extDir, entry.Name())
		if err := os.WriteFile(target, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", target, err)
		}
	}

	// 2. Generate icons (16, 48, 128)
	for _, size := range []int{16, 48, 128} {
		iconBytes, err := generateExtensionIconPNG(size)
		if err != nil {
			return fmt.Errorf("failed to generate icon%d.png: %w", size, err)
		}
		iconPath := filepath.Join(extDir, fmt.Sprintf("icon%d.png", size))
		if err := os.WriteFile(iconPath, iconBytes, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", iconPath, err)
		}
	}

	// 3. Determine installed u2b binary path and the native host launcher
	u2bBinary := resolveInstalledU2BBinary(homeDir)
	hostLauncherPath := u2bBinary
	if runtime.GOOS == "windows" {
		// Chromium on Windows launches the .exe directly; u2b routes the
		// chrome-extension:// origin argument to the native-host command itself.
		_ = os.Remove(filepath.Join(baseDir, "native-host.sh"))
	} else {
		hostLauncherPath = filepath.Join(baseDir, "native-host.sh")
		scriptContent := fmt.Sprintf(`#!/usr/bin/env bash
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:%s/.local/bin:%s/go/bin:$PATH"
exec %q native-host "$@"
`, homeDir, homeDir, u2bBinary)
		if err := os.WriteFile(hostLauncherPath, []byte(scriptContent), 0755); err != nil {
			return fmt.Errorf("failed to write native host wrapper script: %w", err)
		}
		_ = os.Chmod(hostLauncherPath, 0755)
	}

	// 4. Register Native Messaging Host for Brave (and Chrome/Chromium/Edge)
	origins := []string{
		fmt.Sprintf("chrome-extension://%s/", deterministicExtensionID),
	}
	if runtime.GOOS != "windows" {
		if pathID := computePathExtensionID(extDir); pathID != "" && pathID != deterministicExtensionID {
			origins = append(origins, fmt.Sprintf("chrome-extension://%s/", pathID))
		}
	}

	manifestPayload := map[string]any{
		"name":            nativeHostName,
		"description":     "u2b YouTube Downloader Native Messaging Host",
		"path":            hostLauncherPath,
		"type":            "stdio",
		"allowed_origins": origins,
	}
	manifestBytes, err := json.MarshalIndent(manifestPayload, "", "  ")
	if err != nil {
		return err
	}

	var registeredPaths []string
	if runtime.GOOS == "windows" {
		manifestFile := filepath.Join(baseDir, nativeHostName+".json")
		if err := os.WriteFile(manifestFile, manifestBytes, 0644); err != nil {
			return fmt.Errorf("failed to write native host manifest: %w", err)
		}
		registeredPaths = registerWindowsNativeHost(manifestFile)
	} else {
		for _, dir := range getNativeMessagingHostDirs(homeDir) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				continue
			}
			targetFile := filepath.Join(dir, nativeHostName+".json")
			if err := os.WriteFile(targetFile, manifestBytes, 0644); err == nil {
				registeredPaths = append(registeredPaths, targetFile)
			}
		}
	}

	// 5. Initialize default presets if not present
	if _, err := os.Stat(presetsFilePath()); os.IsNotExist(err) {
		_ = saveExtensionPresets(buildDefaultExtensionPresets())
	}

	fmt.Printf("✅ u2b 浏览器插件文件已生成: %s\n", extDir)
	fmt.Printf("✅ Native Messaging 宿主已注册 (%d 处):\n", len(registeredPaths))
	for _, p := range registeredPaths {
		fmt.Printf("   - %s\n", p)
	}

	fileManager := "Finder"
	switch runtime.GOOS {
	case "darwin":
		copyCmd := exec.Command("pbcopy")
		copyCmd.Stdin = strings.NewReader(extDir)
		_ = copyCmd.Run()
		if !extNoOpen {
			_ = exec.Command("open", "-a", "Brave Browser", "brave://extensions").Start()
			_ = exec.Command("open", "-R", extDir).Start()
		}
	case "windows":
		fileManager = "资源管理器"
		copyCmd := exec.Command("clip")
		copyCmd.Stdin = strings.NewReader(extDir)
		_ = copyCmd.Run()
		if !extNoOpen {
			_ = exec.Command("explorer.exe", extDir).Start()
		}
	}

	fmt.Printf("\n👉 首次在 Brave 中启用插件只需 1 步：\n")
	fmt.Printf("   1. 在 Brave 地址栏打开 `brave://extensions`，右上角开启「开发者模式 (Developer mode)」\n")
	fmt.Printf("   2. 将%s中的 `extension` 文件夹拖入该页面（或点击「加载已解压的扩展程序」选择：%s，路径已复制到剪贴板）\n", fileManager, extDir)

	return nil
}

// registerWindowsNativeHost writes HKCU registry keys pointing at the manifest file
// for Brave, Chrome, Chromium and Edge. Returns the registry keys written.
func registerWindowsNativeHost(manifestFile string) []string {
	keys := []string{
		`HKCU\Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + nativeHostName,
		`HKCU\Software\Google\Chrome\NativeMessagingHosts\` + nativeHostName,
		`HKCU\Software\Chromium\NativeMessagingHosts\` + nativeHostName,
		`HKCU\Software\Microsoft\Edge\NativeMessagingHosts\` + nativeHostName,
	}
	var written []string
	for _, key := range keys {
		cmd := exec.Command("reg", "add", key, "/ve", "/t", "REG_SZ", "/d", manifestFile, "/f")
		if err := cmd.Run(); err == nil {
			written = append(written, key)
		}
	}
	return written
}

func resolveInstalledU2BBinary(homeDir string) string {
	if runtime.GOOS == "windows" {
		winBin := filepath.Join(homeDir, "u2b", "bin", "u2b.exe")
		if _, err := os.Stat(winBin); err == nil {
			return winBin
		}
		if exe, err := os.Executable(); err == nil {
			return exe
		}
		return winBin
	}
	localBin := filepath.Join(homeDir, ".local", "bin", "u2b")
	if _, err := os.Stat(localBin); err == nil {
		return localBin
	}
	if p, err := exec.LookPath("u2b"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	if p, err := exec.LookPath("u2b_download"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return localBin
}

func getNativeMessagingHostDirs(homeDir string) []string {
	switch runtime.GOOS {
	case "darwin":
		appSupport := filepath.Join(homeDir, "Library", "Application Support")
		return []string{
			filepath.Join(appSupport, "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
			filepath.Join(appSupport, "Google", "Chrome", "NativeMessagingHosts"),
			filepath.Join(appSupport, "Chromium", "NativeMessagingHosts"),
		}
	case "linux":
		cfgDir := filepath.Join(homeDir, ".config")
		return []string{
			filepath.Join(cfgDir, "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
			filepath.Join(cfgDir, "google-chrome", "NativeMessagingHosts"),
			filepath.Join(cfgDir, "chromium", "NativeMessagingHosts"),
		}
	default:
		return nil
	}
}

func computePathExtensionID(extDir string) string {
	realPath, err := filepath.EvalSymlinks(extDir)
	if err != nil {
		realPath = extDir
	}
	sum := sha256.Sum256([]byte(realPath))
	var id [32]byte
	for i := 0; i < 16; i++ {
		id[i*2] = 'a' + (sum[i] >> 4)
		id[i*2+1] = 'a' + (sum[i] & 0x0f)
	}
	return string(id[:])
}

func generateExtensionIconPNG(size int) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	fSize := float64(size)
	radius := fSize * 0.22

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if !insideRoundedRect(fx, fy, fSize, fSize, radius) {
				continue
			}

			// Gradient from deep royal blue to vibrant cyan-blue
			t := (fx + fy) / (2 * fSize)
			r := uint8(30 + 25*t)
			g := uint8(75 + 85*t)
			b := uint8(215 + 35*t)
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})

			// Draw white download arrow in center
			nx := (fx / fSize) - 0.5
			ny := (fy / fSize) - 0.5

			// Vertical shaft: x in [-0.09, 0.09], y in [-0.26, 0.05]
			inShaft := math.Abs(nx) <= 0.09 && ny >= -0.26 && ny <= 0.05
			// Arrowhead triangle: y in [0.0, 0.22], |nx| <= (0.22 - ny) * 1.15
			inHead := ny >= -0.02 && ny <= 0.21 && math.Abs(nx) <= (0.21-ny)*1.15
			// Bottom tray bar: x in [-0.24, 0.24], y in [0.24, 0.31]
			inBar := math.Abs(nx) <= 0.24 && ny >= 0.24 && ny <= 0.31

			if inShaft || inHead || inBar {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func insideRoundedRect(x, y, w, h, r float64) bool {
	if x < 0 || y < 0 || x > w || y > h {
		return false
	}
	cx := math.Max(r, math.Min(w-r, x))
	cy := math.Max(r, math.Min(h-r, y))
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= r*r
}

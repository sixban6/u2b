package cmd

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sixban6/u2b/config"
	"github.com/spf13/cobra"
)

const launchAgentLabel = "com.sixban.u2b.serve"

var serviceNoOpen bool

var serveInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start u2b serve as a silent background service (auto-starts on login)",
	RunE:  runServiceInstall,
}

var serveUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove the u2b background service",
	RunE:  runServiceUninstall,
}

var serveStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the u2b background service",
	RunE:  runServiceStatus,
}

var serviceAliasCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the u2b background service (install / uninstall / status)",
}

func init() {
	serveInstallCmd.Flags().BoolVar(&serviceNoOpen, "no-open", false, "do not open browser setup page after installing service")

	serveCmd.AddCommand(serveInstallCmd)
	serveCmd.AddCommand(serveUninstallCmd)
	serveCmd.AddCommand(serveStatusCmd)

	// Also expose top-level `u2b service install|uninstall|status` for convenience
	serviceInstallAlias := *serveInstallCmd
	serviceUninstallAlias := *serveUninstallCmd
	serviceStatusAlias := *serveStatusCmd
	serviceAliasCmd.AddCommand(&serviceInstallAlias, &serviceUninstallAlias, &serviceStatusAlias)
	rootCmd.AddCommand(serviceAliasCmd)
}

func macLaunchAgentPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", launchAgentLabel+".plist")
}

func linuxSystemdUnitPath(homeDir string) string {
	return filepath.Join(homeDir, ".config", "systemd", "user", "u2b.service")
}

func runServiceInstall(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	listenAddr, port, _ := resolveListenAddress(cfg)
	explicitToken := strings.TrimSpace(serveToken)
	token := resolveOrCreateServerToken(explicitToken, cfg.Server.Token, serveGenToken, serveNoAuth)

	u2bBin := resolveInstalledU2BBinary(homeDir)
	baseDir := ensureSecureU2BDir()
	logPath := filepath.Join(baseDir, "serve.log")

	switch runtime.GOOS {
	case "darwin":
		plistPath := macLaunchAgentPath(homeDir)
		if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
			return err
		}

		// Only pass --token on process CLI if user overrode --token explicitly;
		// otherwise `u2b serve` automatically reads ~/u2b/server_token (mode 0600),
		// avoiding exposing the token in `ps aux` process listings.
		cmdArgs := []string{u2bBin, "serve", "--listen", listenAddr}
		if explicitToken != "" {
			cmdArgs = append(cmdArgs, "--token", explicitToken)
		}
		if serveNoAuth {
			cmdArgs = append(cmdArgs, "--no-auth")
		}

		var argsXML strings.Builder
		for _, a := range cmdArgs {
			fmt.Fprintf(&argsXML, "\t\t<string>%s</string>\n", xmlEscapeString(a))
		}

		envPath := fmt.Sprintf("/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:%s/.local/bin:%s/go/bin:/usr/bin:/bin:/usr/sbin:/sbin", homeDir, homeDir)
		plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
%s	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>%s</string>
		<key>HOME</key>
		<string>%s</string>
	</dict>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, launchAgentLabel, argsXML.String(), xmlEscapeString(envPath), xmlEscapeString(homeDir), xmlEscapeString(baseDir), xmlEscapeString(logPath), xmlEscapeString(logPath))

		if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
			return fmt.Errorf("failed to write LaunchAgent plist: %w", err)
		}

		_ = exec.Command("launchctl", "unload", plistPath).Run()
		if out, err := exec.Command("launchctl", "load", "-w", plistPath).CombinedOutput(); err != nil {
			return fmt.Errorf("failed to load LaunchAgent: %w (%s)", err, strings.TrimSpace(string(out)))
		}

		fmt.Printf("✅ u2b 后台常驻服务已安装并启动 (开机自启)\n")
		fmt.Printf("   • 服务配置: %s\n", plistPath)
		fmt.Printf("   • 运行日志: %s\n", logPath)

	case "linux":
		unitPath := linuxSystemdUnitPath(homeDir)
		if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
			return err
		}
		execLine := fmt.Sprintf("%s serve --listen %s", u2bBin, listenAddr)
		if explicitToken != "" {
			execLine += " --token " + explicitToken
		}
		if serveNoAuth {
			execLine += " --no-auth"
		}
		unitContent := fmt.Sprintf(`[Unit]
Description=u2b Cross-Device YouTube Downloader Server
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`, baseDir, execLine)

		if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
			return fmt.Errorf("failed to write systemd user unit: %w", err)
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		if out, err := exec.Command("systemctl", "--user", "enable", "--now", "u2b.service").CombinedOutput(); err != nil {
			return fmt.Errorf("failed to enable systemd service: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		fmt.Printf("✅ u2b systemd 后台服务已安装并启动: %s\n", unitPath)

	default:
		return fmt.Errorf("当前系统 (%s) 暂不支持自动注册系统服务，请直接运行 `u2b serve`", runtime.GOOS)
	}

	addrs := DiscoverServerAddresses(port)
	primary := fmt.Sprintf("http://localhost:%d", port)
	if len(addrs) > 0 {
		primary = addrs[0].URL
	}
	qrTargetURL := primary + "/"
	if token != "" {
		qrTargetURL += "?token=" + url.QueryEscape(token)
	}

	fmt.Printf("\n🌐 iPhone Brave 接入地址：\n")
	for _, a := range addrs {
		fmt.Printf("   • %-24s %s\n", a.Label+":", a.URL)
	}
	if qr, err := RenderQRTerminal(qrTargetURL); err == nil {
		fmt.Printf("\n📱 用 iPhone 相机扫码一键进入下载中心：\n\n%s\n", qr)
	}

	if !serviceNoOpen {
		time.Sleep(400 * time.Millisecond)
		localSetup := "http://localhost:" + strconv.Itoa(port) + "/setup"
		if token != "" {
			localSetup += "?token=" + url.QueryEscape(token)
		}
		_ = openURL(localSetup)
	}
	return nil
}

func runServiceUninstall(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		plistPath := macLaunchAgentPath(homeDir)
		_ = exec.Command("launchctl", "unload", "-w", plistPath).Run()
		_ = os.Remove(plistPath)
		fmt.Println("✅ u2b macOS 后台服务已停止并移除")
	case "linux":
		unitPath := linuxSystemdUnitPath(homeDir)
		_ = exec.Command("systemctl", "--user", "disable", "--now", "u2b.service").Run()
		_ = os.Remove(unitPath)
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		fmt.Println("✅ u2b Linux systemd 服务已停止并移除")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return nil
}

func runServiceStatus(cmd *cobra.Command, args []string) error {
	homeDir, _ := os.UserHomeDir()
	cfg, err := loadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	_, port, _ := resolveListenAddress(cfg)

	switch runtime.GOOS {
	case "darwin":
		plistPath := macLaunchAgentPath(homeDir)
		if _, err := os.Stat(plistPath); os.IsNotExist(err) {
			fmt.Println("⚪ u2b 后台服务尚未安装（运行 `u2b serve install` 即可一键开启）")
			return nil
		}
		out, err := exec.Command("launchctl", "list", launchAgentLabel).CombinedOutput()
		if err != nil {
			fmt.Println("⚠️ u2b 服务已配置但当前未运行")
			return nil
		}
		fmt.Printf("🟢 u2b 后台服务正在运行 (%s)\n", launchAgentLabel)
		_ = out
	case "linux":
		if err := exec.Command("systemctl", "--user", "is-active", "--quiet", "u2b.service").Run(); err == nil {
			fmt.Println("🟢 u2b systemd 后台服务正在运行")
		} else {
			fmt.Println("⚪ u2b systemd 后台服务未运行")
			return nil
		}
	}

	token := resolveOrCreateServerToken(serveToken, cfg.Server.Token, false, serveNoAuth)
	for _, a := range DiscoverServerAddresses(port) {
		fmt.Printf("   • %-24s %s\n", a.Label+":", a.URL)
	}
	if token != "" {
		fmt.Printf("   • 本机安全引导页:         http://localhost:%d/setup?token=%s\n", port, url.QueryEscape(token))
	}
	return nil
}

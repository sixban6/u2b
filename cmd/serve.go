package cmd

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sixban6/u2b/config"
	"github.com/sixban6/u2b/downloader"
	"github.com/spf13/cobra"
)

//go:embed web_assets/*
var embeddedWebAssets embed.FS

var (
	servePort     int
	serveListen   string
	serveToken    string
	serveGenToken bool
	serveOpen     bool
)

var serveCmd = &cobra.Command{
	Use:     "serve",
	Aliases: []string{"server"},
	Short:   "Start the u2b HTTP & Mobile Web server (for iPhone Brave & Shortcuts)",
	Long: `Start a lightweight cross-device u2b server providing:
  - Mobile Web Panel / PWA (real-time SSE progress, category management, fetch to phone)
  - iOS Shortcuts quick endpoint (/api/quick) & Apple-signed .shortcut generator
  - QR Code onboarding guide (/setup)`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 0, "port to listen on (default 8989)")
	serveCmd.Flags().StringVar(&serveListen, "listen", "", "listen address (default 0.0.0.0:8989)")
	serveCmd.Flags().StringVar(&serveToken, "token", "", "optional API access token for remote security")
	serveCmd.Flags().BoolVar(&serveGenToken, "gen-token", false, "generate a random access token for this session")
	serveCmd.Flags().BoolVar(&serveOpen, "open", false, "open the iPhone setup QR guide in local browser on startup")

	rootCmd.AddCommand(serveCmd)
}

// ServerAddress represents a reachable URL for mobile onboarding.
type ServerAddress struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// ServerTask represents a download job managed by `u2b serve`.
type ServerTask struct {
	ID         string  `json:"id"`
	URL        string  `json:"url"`
	Title      string  `json:"title"`
	Format     string  `json:"format"`
	Quality    string  `json:"quality"`
	OutputDir  string  `json:"outputDir"`
	Filename   string  `json:"filename,omitempty"`
	IsPlaylist bool    `json:"isPlaylist,omitempty"`
	Status     string  `json:"status"` // starting, downloading, completed, error
	Percentage float64 `json:"percentage"`
	Speed      string  `json:"speed,omitempty"`
	ETA        string  `json:"eta,omitempty"`
	FilePath   string  `json:"filePath,omitempty"`
	FileSize   int64   `json:"fileSize,omitempty"`
	Error      string  `json:"error,omitempty"`
	UpdatedAt  int64   `json:"updatedAt"`
}

// U2BServer encapsulates HTTP handlers, task state, and SSE subscribers.
type U2BServer struct {
	listenAddr string
	port       int
	token      string

	mu          sync.RWMutex
	tasks       map[string]*ServerTask
	cancelFuncs map[string]context.CancelFunc

	sseMu       sync.Mutex
	subscribers map[chan string]struct{}

	iconCacheMu sync.Mutex
	icon192     []byte
	icon512     []byte
}

func NewU2BServer(listenAddr string, port int, token string) *U2BServer {
	s := &U2BServer{
		listenAddr:  listenAddr,
		port:        port,
		token:       strings.TrimSpace(token),
		tasks:       make(map[string]*ServerTask),
		cancelFuncs: make(map[string]context.CancelFunc),
		subscribers: make(map[chan string]struct{}),
	}
	s.loadPersistedTasks()
	return s
}

func resolveListenAddress(cfg *config.Config) (string, int, error) {
	addr := strings.TrimSpace(serveListen)
	if addr == "" && cfg != nil && strings.TrimSpace(cfg.Server.Listen) != "" {
		addr = strings.TrimSpace(cfg.Server.Listen)
	}
	if addr == "" {
		addr = "0.0.0.0:8989"
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// Allow passing just ":8989" or host without port
		if strings.HasPrefix(addr, ":") {
			host = "0.0.0.0"
			portStr = strings.TrimPrefix(addr, ":")
		} else {
			host = addr
			portStr = "8989"
		}
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 || p > 65535 {
		p = 8989
	}
	if servePort > 0 && servePort <= 65535 {
		p = servePort
	}
	return net.JoinHostPort(host, strconv.Itoa(p)), p, nil
}

func generateRandomToken() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func runServe(cmd *cobra.Command, args []string) error {
	EnsureEnvironmentPath()

	cfg, err := loadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	listenAddr, port, err := resolveListenAddress(cfg)
	if err != nil {
		return err
	}

	token := strings.TrimSpace(serveToken)
	if token == "" {
		token = strings.TrimSpace(cfg.Server.Token)
	}
	if serveGenToken && token == "" {
		token = generateRandomToken()
	}

	srv := NewU2BServer(listenAddr, port, token)
	httpServer := &http.Server{
		Addr:              listenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	addrs := DiscoverServerAddresses(port)
	primaryURL := fmt.Sprintf("http://localhost:%d", port)
	if len(addrs) > 0 {
		primaryURL = addrs[0].URL
	}
	setupURL := primaryURL + "/setup"
	qrTargetURL := primaryURL + "/"
	if token != "" {
		q := "?token=" + url.QueryEscape(token)
		setupURL += q
		qrTargetURL += q
	}

	fmt.Printf("\n🚀 u2b 跨端下载服务已启动 (监听 %s)\n", listenAddr)
	for _, a := range addrs {
		fmt.Printf("   • %-24s %s\n", a.Label+":", a.URL)
	}
	if token != "" {
		fmt.Printf("   • 访问口令 (Token):        %s\n", token)
	}
	fmt.Printf("   • iPhone 接入引导页:       %s\n", setupURL)

	if qrTerm, err := RenderQRTerminal(qrTargetURL); err == nil {
		fmt.Printf("\n📱 使用 iPhone 相机扫描二维码，直接进入下载中心：\n\n%s\n", qrTerm)
	}

	if serveOpen {
		localSetup := fmt.Sprintf("http://localhost:%d/setup", port)
		if token != "" {
			localSetup += "?token=" + url.QueryEscape(token)
		}
		_ = openURL(localSetup)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n正在关闭 u2b 服务...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// DiscoverServerAddresses probes mDNS (.local), Tailscale (100.64/10), and LAN IPv4 addresses.
func DiscoverServerAddresses(port int) []ServerAddress {
	var list []ServerAddress
	seen := make(map[string]bool)
	add := func(label, host string) {
		if host == "" {
			return
		}
		u := fmt.Sprintf("http://%s:%d", host, port)
		if seen[u] {
			return
		}
		seen[u] = true
		list = append(list, ServerAddress{Label: label, URL: u})
	}

	// 1. macOS Bonjour / mDNS hostname (survives DHCP IP changes!)
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("scutil", "--get", "LocalHostName").Output(); err == nil {
			lh := strings.TrimSpace(string(out))
			if lh != "" {
				add("Wi-Fi 固定域名 (推荐)", lh+".local")
			}
		}
	}
	if len(list) == 0 {
		if hn, err := os.Hostname(); err == nil {
			hn = strings.TrimSpace(strings.TrimSuffix(hn, ".local"))
			if hn != "" && !strings.Contains(hn, " ") {
				add("局域网主机名", hn+".local")
			}
		}
	}

	// 2. Probe network interfaces for Tailscale (100.64.0.0/10) and private LAN IPv4
	var tailscaleIPs []string
	var lanIPs []string

	_, cgnatNet, _ := net.ParseCIDR("100.64.0.0/10")
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			if cgnatNet != nil && cgnatNet.Contains(ip4) {
				tailscaleIPs = append(tailscaleIPs, ip4.String())
			} else if ip4.IsPrivate() && !isVirtualOrBridgeInterface(iface.Name) {
				lanIPs = append(lanIPs, ip4.String())
			}
		}
	}

	for _, ts := range tailscaleIPs {
		add("Tailscale 远程 (4G/5G)", ts)
	}
	for _, lan := range lanIPs {
		add("局域网 IP", lan)
	}
	if len(list) == 0 {
		add("本机回环", "127.0.0.1")
	}
	return list
}

// isVirtualOrBridgeInterface filters out host-only VM bridges and container interfaces
// (e.g. macOS Virtualization.framework/Colima bridge100, VMware vmnet, Docker, WSL)
// that are unreachable from external phones on the Wi-Fi LAN.
func isVirtualOrBridgeInterface(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	prefixes := []string{
		"bridge", "vmenet", "vmnet", "vboxnet",
		"docker", "br-", "virbr", "veth", "cni", "flannel", "cali", "podman", "lxc",
		"utun", "tun", "tap",
		"awdl", "llw", "anpi",
		"vethernet",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	substrings := []string{"vmware", "virtualbox", "wsl", "hyper-v", "docker", "colima", "orbstack", "parallels"}
	for _, sub := range substrings {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// Handler returns the root http.Handler for `u2b serve`.
func (s *U2BServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public static / PWA manifest icons
	mux.HandleFunc("/icon-192.png", s.handleIcon(192))
	mux.HandleFunc("/icon-512.png", s.handleIcon(512))
	mux.HandleFunc("/manifest.webmanifest", s.handleManifest)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": Version})
	})

	// UI pages (index.html handles token prompt if token-protected, and sets cookie if ?token= passed)
	mux.HandleFunc("/", s.handleIndexPage)
	mux.HandleFunc("/setup", s.handleIndexPage)

	// Protected API endpoints
	mux.HandleFunc("/api/config", s.withAuth(s.handleAPIConfig))
	mux.HandleFunc("/api/categories", s.withAuth(s.handleAPICategories))
	mux.HandleFunc("/api/cookies", s.withAuth(s.handleAPICookies))
	mux.HandleFunc("/api/tasks", s.withAuth(s.handleAPITasks))
	mux.HandleFunc("/api/tasks/cancel", s.withAuth(s.handleAPITaskCancel))
	mux.HandleFunc("/api/events", s.withAuth(s.handleAPIEvents))
	mux.HandleFunc("/api/files/", s.withAuth(s.handleAPIFileDownload))
	mux.HandleFunc("/api/quick", s.withAuth(s.handleAPIQuick))
	mux.HandleFunc("/api/qr.svg", s.withAuth(s.handleAPIQRCode))
	mux.HandleFunc("/api/shortcut/u2b.shortcut", s.withAuth(s.handleAPIShortcutDownload))
	mux.HandleFunc("/api/shortcut/import-mac", s.withAuth(s.handleAPIShortcutImportMac))

	return mux
}

func (s *U2BServer) isAuthorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	check := func(cand string) bool {
		cand = strings.TrimSpace(cand)
		return len(cand) == len(s.token) && subtle.ConstantTimeCompare([]byte(cand), []byte(s.token)) == 1
	}

	if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		if check(auth[7:]) {
			return true
		}
	}
	if check(r.Header.Get("X-U2B-Token")) {
		return true
	}
	if check(r.URL.Query().Get("token")) {
		return true
	}
	if c, err := r.Cookie("u2b_token"); err == nil && check(c.Value) {
		return true
	}
	return false
}

func (s *U2BServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthorized(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"ok":    false,
				"error": "未授权访问，请提供有效 Token",
			})
			return
		}
		if s.token != "" && r.URL.Query().Get("token") != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "u2b_token",
				Value:    s.token,
				Path:     "/",
				HttpOnly: true,
				MaxAge:   86400 * 90,
				SameSite: http.SameSiteLaxMode,
			})
		}
		next(w, r)
	}
}

func (s *U2BServer) handleIndexPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/setup" {
		http.NotFound(w, r)
		return
	}
	if s.token != "" && s.isAuthorized(r) && r.URL.Query().Get("token") != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     "u2b_token",
			Value:    s.token,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400 * 90,
			SameSite: http.SameSiteLaxMode,
		})
	}
	data, err := embeddedWebAssets.ReadFile("web_assets/index.html")
	if err != nil {
		http.Error(w, "missing web UI", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *U2BServer) handleManifest(w http.ResponseWriter, r *http.Request) {
	manifest := map[string]any{
		"name":             "u2b 跨端下载",
		"short_name":       "u2b",
		"description":      "iPhone Brave & Mac YouTube 视频音频极速下载中心",
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#0b1120",
		"theme_color":      "#0b1120",
		"icons": []map[string]any{
			{"src": "/icon-192.png", "sizes": "192x192", "type": "image/png"},
			{"src": "/icon-512.png", "sizes": "512x512", "type": "image/png"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(manifest)
}

func (s *U2BServer) handleIcon(size int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.iconCacheMu.Lock()
		var data []byte
		if size == 192 {
			if len(s.icon192) == 0 {
				s.icon192, _ = generateExtensionIconPNG(192)
			}
			data = s.icon192
		} else {
			if len(s.icon512) == 0 {
				s.icon512, _ = generateExtensionIconPNG(512)
			}
			data = s.icon512
		}
		s.iconCacheMu.Unlock()

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
	}
}

func (s *U2BServer) buildConfigPayload() map[string]any {
	presets := loadExtensionPresets()
	cfg, err := loadConfig()
	hasCookies := resolveCachedCookiesFile() != ""
	if err == nil && cfg != nil && cfg.Auth.Method != config.AuthMethodNone {
		hasCookies = true
	}

	return map[string]any{
		"ok":              true,
		"version":         Version,
		"platform":        runtime.GOOS,
		"videoRootDir":    presets.VideoRootDir,
		"audioRootDir":    presets.AudioRootDir,
		"defaultVideoDir": presets.DefaultVideoDir,
		"defaultAudioDir": presets.DefaultAudioDir,
		"videoDirs":       presets.VideoDirs,
		"audioDirs":       presets.AudioDirs,
		"addresses":       DiscoverServerAddresses(s.port),
		"hasCookies":      hasCookies,
		"canSignShortcut": CanSignShortcuts(),
	}
}

func (s *U2BServer) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildConfigPayload())
}

func (s *U2BServer) handleAPICategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		MediaType string `json:"mediaType"`
		Name      string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "请求格式错误"})
		return
	}

	catName := sanitizeFolderName(req.Name)
	if catName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "分类目录名称不能为空"})
		return
	}

	presets := loadRawExtensionPresets()
	rootDir := presets.VideoRootDir
	if req.MediaType == "audio" {
		rootDir = presets.AudioRootDir
	}
	if rootDir == "" {
		def := buildDefaultExtensionPresets()
		if req.MediaType == "audio" {
			rootDir = def.AudioRootDir
		} else {
			rootDir = def.VideoRootDir
		}
	}

	newDirPath := filepath.Join(rootDir, catName)
	if err := os.MkdirAll(newDirPath, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	presets.HiddenDirs = removeString(presets.HiddenDirs, newDirPath)
	_ = saveExtensionPresets(presets)

	cfgPayload := s.buildConfigPayload()
	s.broadcastSSE("config_updated", cfgPayload)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"name":        catName,
		"createdPath": newDirPath,
		"config":      cfgPayload,
	})
}

func (s *U2BServer) handleAPICookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Cookies string `json:"cookies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	if !strings.Contains(req.Cookies, "youtube.com") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "未检测到 youtube.com 域名记录，请粘贴有效的 Netscape cookies.txt"})
		return
	}
	saveCachedCookies(req.Cookies)
	cfgPayload := s.buildConfigPayload()
	s.broadcastSSE("config_updated", cfgPayload)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *U2BServer) handleAPITasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"tasks": s.listTasksSorted(),
		})
	case http.MethodPost:
		var req struct {
			URL        string `json:"url"`
			Title      string `json:"title"`
			Format     string `json:"format"`
			Quality    string `json:"quality"`
			OutputDir  string `json:"outputDir"`
			Category   string `json:"category"`
			Filename   string `json:"filename"`
			IsPlaylist bool   `json:"isPlaylist"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid JSON body"})
			return
		}
		cleanURL := sanitizeInputURL(req.URL)
		if cleanURL == "" || (!strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://")) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "请输入有效的视频 URL"})
			return
		}

		outDir, dirLabel := resolveTargetOutputDir(req.Format, req.OutputDir, req.Category)
		task := s.enqueueDownload(cleanURL, req.Title, req.Format, req.Quality, outDir, req.Filename, req.IsPlaylist)
		_ = dirLabel
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":   true,
			"task": task,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
	}
}

func (s *U2BServer) handleAPITaskCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ID == "" {
		req.ID = r.URL.Query().Get("id")
	}

	s.mu.Lock()
	cancel, hasCancel := s.cancelFuncs[req.ID]
	if hasCancel {
		delete(s.cancelFuncs, req.ID)
	}
	if t, ok := s.tasks[req.ID]; ok && (t.Status == "starting" || t.Status == "downloading") {
		t.Status = "error"
		t.Error = "已取消下载"
		t.UpdatedAt = time.Now().UnixMilli()
		cloned := *t
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		s.persistTasks()
		s.broadcastSSE("task_update", cloned)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *U2BServer) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := make(chan string, 32)
	s.sseMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.subscribers, ch)
		s.sseMu.Unlock()
	}()

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			_, _ = w.Write([]byte(msg))
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *U2BServer) handleAPIFileDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/files/")
	id = strings.Trim(id, "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	t, exists := s.tasks[id]
	var filePath string
	if exists && t != nil && t.Status == "completed" && !t.IsPlaylist {
		filePath = t.FilePath
	}
	s.mu.RUnlock()

	if filePath == "" {
		http.Error(w, "文件不存在或尚未完成下载", http.StatusNotFound)
		return
	}
	serveMediaFile(w, r, filePath, r.URL.Query().Get("download") == "1")
}

// handleAPIQuick serves both async fire-and-forget requests (for saving to Mac)
// and synchronous wait=1 file responses (for saving directly to iPhone via Shortcuts).
func (s *U2BServer) handleAPIQuick(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rawURL := q.Get("url")
	format := q.Get("format")
	quality := q.Get("quality")
	category := q.Get("category")
	outputDir := q.Get("outputDir")
	filename := q.Get("filename")
	waitMode := q.Get("wait") == "1" || strings.EqualFold(q.Get("wait"), "true")
	plainMode := q.Get("plain") == "1" || strings.EqualFold(q.Get("plain"), "true")

	if r.Method == http.MethodPost && rawURL == "" {
		var body struct {
			URL       string `json:"url"`
			Format    string `json:"format"`
			Quality   string `json:"quality"`
			Category  string `json:"category"`
			OutputDir string `json:"outputDir"`
			Filename  string `json:"filename"`
			Wait      bool   `json:"wait"`
			Plain     bool   `json:"plain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			rawURL = body.URL
			if format == "" {
				format = body.Format
			}
			if quality == "" {
				quality = body.Quality
			}
			if category == "" {
				category = body.Category
			}
			if outputDir == "" {
				outputDir = body.OutputDir
			}
			if filename == "" {
				filename = body.Filename
			}
			if body.Wait {
				waitMode = true
			}
			if body.Plain {
				plainMode = true
			}
		}
	}

	cleanURL := sanitizeInputURL(rawURL)
	if cleanURL == "" || (!strings.HasPrefix(cleanURL, "http://") && !strings.HasPrefix(cleanURL, "https://")) {
		if plainMode {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("⚠️ 未检测到有效的视频链接"))
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "未检测到有效的视频链接"})
		return
	}

	if strings.EqualFold(format, "mp3") {
		format = "mp3"
	} else {
		format = "mp4"
	}
	if quality == "" {
		quality = "hd1080"
	}

	resolvedDir, dirLabel := resolveTargetOutputDir(format, outputDir, category)

	if !waitMode {
		isPlaylist := strings.Contains(cleanURL, "youtube.com/playlist")
		task := s.enqueueDownload(cleanURL, "", format, quality, resolvedDir, filename, isPlaylist)
		modeDesc := "1080p 视频"
		if format == "mp3" {
			modeDesc = "高音质 MP3"
		}
		msg := fmt.Sprintf("✅ 已开始下载 %s → %s", modeDesc, dirLabel)
		if plainMode {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(msg))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"id":      task.ID,
			"message": msg,
			"task":    task,
		})
		return
	}

	// Synchronous wait=1 mode: download and stream binary file straight back to iPhone Shortcut!
	task, doneCh := s.enqueueDownloadWithWait(r.Context(), cleanURL, "", format, quality, resolvedDir, filename, false)
	<-doneCh

	s.mu.RLock()
	finished := s.tasks[task.ID]
	var finalPath, finalErr string
	if finished != nil {
		finalPath = finished.FilePath
		finalErr = finished.Error
	}
	s.mu.RUnlock()

	if finalErr != "" || finalPath == "" {
		if finalErr == "" {
			finalErr = "下载失败"
		}
		http.Error(w, finalErr, http.StatusBadGateway)
		return
	}

	serveMediaFile(w, r, finalPath, true)
}

func (s *U2BServer) handleAPIQRCode(w http.ResponseWriter, r *http.Request) {
	text := strings.TrimSpace(r.URL.Query().Get("text"))
	if text == "" {
		addrs := DiscoverServerAddresses(s.port)
		text = addrs[0].URL + "/"
		if s.token != "" {
			text += "?token=" + url.QueryEscape(s.token)
		}
	}
	svg, err := RenderQRSVG(text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(svg))
}

func (s *U2BServer) resolveRequestedHost(r *http.Request, rawHostParam string) string {
	hostParam := strings.TrimSpace(rawHostParam)
	if hostParam != "" && (strings.HasPrefix(hostParam, "http://") || strings.HasPrefix(hostParam, "https://")) {
		return strings.TrimRight(hostParam, "/")
	}
	addrs := DiscoverServerAddresses(s.port)
	if r.Host != "" && !strings.HasPrefix(r.Host, "localhost") && !strings.HasPrefix(r.Host, "127.0.0.1") {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return fmt.Sprintf("%s://%s", scheme, r.Host)
	}
	return addrs[0].URL
}

func (s *U2BServer) handleAPIShortcutDownload(w http.ResponseWriter, r *http.Request) {
	targetHost := s.resolveRequestedHost(r, r.URL.Query().Get("host"))
	shortcutBytes, _, err := BuildSignedIOSShortcut(targetHost, s.token)
	if err != nil && len(shortcutBytes) == 0 {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fileName := "u2b 下载.shortcut"
	w.Header().Set("Content-Type", "application/x-apple-shortcut")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="u2b.shortcut"; filename*=UTF-8''%s`, url.PathEscape(fileName)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(shortcutBytes)
}

func (s *U2BServer) handleAPIShortcutImportMac(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	var req struct {
		Host string `json:"host"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	targetHost := s.resolveRequestedHost(r, req.Host)

	if err := ImportShortcutOnMac(targetHost, s.token); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func resolveTargetOutputDir(format, explicitDir, categoryName string) (string, string) {
	presets := loadExtensionPresets()
	dirs := presets.VideoDirs
	defaultDir := presets.DefaultVideoDir
	rootDir := presets.VideoRootDir
	defaultLabel := "视频默认"

	if strings.EqualFold(format, config.OutputFormatMP3) {
		dirs = presets.AudioDirs
		defaultDir = presets.DefaultAudioDir
		rootDir = presets.AudioRootDir
		defaultLabel = "音乐默认"
	}

	if trimmedDir := strings.TrimSpace(explicitDir); trimmedDir != "" {
		for _, d := range dirs {
			if d.Path == trimmedDir {
				return d.Path, d.Label
			}
		}
		if abs, err := filepath.Abs(trimmedDir); err == nil {
			return abs, filepath.Base(abs)
		}
	}

	if cat := sanitizeFolderName(categoryName); cat != "" {
		for _, d := range dirs {
			if strings.EqualFold(d.Label, cat) {
				return d.Path, d.Label
			}
		}
		if rootDir != "" {
			joined := filepath.Join(rootDir, cat)
			return joined, cat
		}
	}

	return defaultDir, defaultLabel
}

func (s *U2BServer) enqueueDownload(urlStr, title, format, quality, outputDir, filename string, isPlaylist bool) *ServerTask {
	task, _ := s.enqueueDownloadWithWait(context.Background(), urlStr, title, format, quality, outputDir, filename, isPlaylist)
	return task
}

func (s *U2BServer) enqueueDownloadWithWait(parentCtx context.Context, urlStr, title, format, quality, outputDir, filename string, isPlaylist bool) (*ServerTask, <-chan struct{}) {
	if strings.EqualFold(format, "mp3") {
		format = "mp3"
	} else {
		format = "mp4"
	}
	if quality == "" {
		quality = "hd1080"
	}
	if title == "" {
		title = urlStr
	}

	id := fmt.Sprintf("task-%d-%s", time.Now().UnixMilli(), generateRandomToken()[:6])
	task := &ServerTask{
		ID:         id,
		URL:        urlStr,
		Title:      title,
		Format:     format,
		Quality:    quality,
		OutputDir:  outputDir,
		Filename:   strings.TrimSpace(filename),
		IsPlaylist: isPlaylist,
		Status:     "starting",
		Percentage: 0,
		UpdatedAt:  time.Now().UnixMilli(),
	}

	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Hour)
	doneCh := make(chan struct{})

	s.mu.Lock()
	s.tasks[id] = task
	s.cancelFuncs[id] = cancel
	cloned := *task
	s.mu.Unlock()

	s.persistTasks()
	s.broadcastSSE("task_update", cloned)

	go func() {
		defer close(doneCh)
		defer func() {
			s.mu.Lock()
			delete(s.cancelFuncs, id)
			s.mu.Unlock()
			cancel()
		}()
		s.executeServerTask(ctx, id)
	}()

	return &cloned, doneCh
}

func (s *U2BServer) executeServerTask(ctx context.Context, id string) {
	s.mu.RLock()
	orig, ok := s.tasks[id]
	if !ok {
		s.mu.RUnlock()
		return
	}
	t := *orig
	s.mu.RUnlock()

	cfg, err := loadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	_ = os.MkdirAll(t.OutputDir, 0755)
	cfg.Download.OutputDir = t.OutputDir

	// Automatically attach synced desktop Brave cookies if auth is currently unconfigured
	if cfg.Auth.Method == config.AuthMethodNone {
		if cachedCookie := resolveCachedCookiesFile(); cachedCookie != "" {
			cfg.Auth.Method = config.AuthMethodCookiesFile
			cfg.Auth.CookiesFile = cachedCookie
		}
	}

	d, err := downloader.NewYouTubeDownloader(cfg)
	if err != nil {
		s.failTask(id, fmt.Sprintf("初始化下载器失败: %v", err))
		return
	}
	defer d.Close()

	var lastBroadcast time.Time
	d.SetProgressCallback(func(p *downloader.DownloadProgress) {
		if p == nil {
			return
		}
		if time.Since(lastBroadcast) < 220*time.Millisecond && p.Percentage < 99.9 {
			return
		}
		lastBroadcast = time.Now()
		s.updateTask(id, func(st *ServerTask) {
			st.Status = "downloading"
			st.Percentage = p.Percentage
			st.Speed = p.Speed
			st.ETA = p.ETA
		}, false)
	})

	selectedFormat := ""
	if t.Format == "mp3" {
		selectedFormat = config.OutputFormatMP3
	}

	if t.IsPlaylist {
		if info, infoErr := d.GetPlaylistInfo(ctx, t.URL); infoErr == nil && info.PlaylistTitle != "" {
			subFolder := sanitizeFolderName(info.PlaylistTitle)
			if subFolder != "" {
				t.OutputDir = filepath.Join(t.OutputDir, subFolder)
				_ = os.MkdirAll(t.OutputDir, 0755)
			}
			s.updateTask(id, func(st *ServerTask) {
				st.Title = info.PlaylistTitle
				st.OutputDir = t.OutputDir
				st.Status = "downloading"
			}, true)
		}
		opts := &config.DownloadOptions{
			URL:            t.URL,
			Quality:        t.Quality,
			Format:         selectedFormat,
			OutputDir:      t.OutputDir,
			OutputTemplate: "%(playlist_index)02d - %(title)s [%(id)s].%(ext)s",
			ExtraArgs:      cfg.Advanced.ExtraArgs,
		}
		if err := downloadPlaylistDirect(ctx, cfg, t.URL, t.OutputDir, opts); err != nil {
			s.failTask(id, err.Error())
			return
		}
		s.updateTask(id, func(st *ServerTask) {
			st.Status = "completed"
			st.Percentage = 100
			st.FilePath = t.OutputDir
		}, true)
		return
	}

	outputTemplate := cfg.Download.OutputTemplate
	if t.Filename != "" {
		if customTpl, tplErr := BuildOutputTemplateForFilename(t.Filename); tplErr == nil {
			outputTemplate = customTpl
		}
	}

	opts := &config.DownloadOptions{
		URL:            t.URL,
		Quality:        t.Quality,
		Format:         selectedFormat,
		OutputDir:      t.OutputDir,
		OutputTemplate: outputTemplate,
		ExtraArgs:      cfg.Advanced.ExtraArgs,
	}

	res, err := d.Download(ctx, t.URL, opts)
	if err != nil {
		s.failTask(id, err.Error())
		return
	}

	s.updateTask(id, func(st *ServerTask) {
		st.Status = "completed"
		st.Percentage = 100
		st.FilePath = res.FilePath
		st.FileSize = res.FileSize
		if res.VideoInfo != nil && res.VideoInfo.Title != "" {
			st.Title = res.VideoInfo.Title
		} else if res.FilePath != "" {
			st.Title = strings.TrimSuffix(filepath.Base(res.FilePath), filepath.Ext(res.FilePath))
		}
	}, true)
}

func (s *U2BServer) updateTask(id string, mutator func(*ServerTask), persist bool) {
	s.mu.Lock()
	st, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	// Do not overwrite user-cancelled state
	if st.Status == "error" && st.Error == "已取消下载" {
		s.mu.Unlock()
		return
	}
	mutator(st)
	st.UpdatedAt = time.Now().UnixMilli()
	cloned := *st
	s.mu.Unlock()

	if persist {
		s.persistTasks()
	}
	s.broadcastSSE("task_update", cloned)
}

func (s *U2BServer) failTask(id, errMsg string) {
	s.updateTask(id, func(st *ServerTask) {
		st.Status = "error"
		st.Error = errMsg
	}, true)
}

func (s *U2BServer) listTasksSorted() []ServerTask {
	s.mu.RLock()
	list := make([]ServerTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		list = append(list, *t)
	}
	s.mu.RUnlock()

	// Sort newest first
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].UpdatedAt > list[i].UpdatedAt {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	if len(list) > 40 {
		list = list[:40]
	}
	return list
}

func serverTasksHistoryPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "u2b", "server_tasks.json")
}

func (s *U2BServer) persistTasks() {
	list := s.listTasksSorted()
	path := serverTasksHistoryPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if data, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0600)
	}
}

func (s *U2BServer) loadPersistedTasks() {
	data, err := os.ReadFile(serverTasksHistoryPath())
	if err != nil {
		return
	}
	var list []ServerTask
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range list {
		t := list[i]
		if t.Status == "starting" || t.Status == "downloading" {
			t.Status = "error"
			t.Error = "服务重启导致中断"
		}
		s.tasks[t.ID] = &t
	}
}

func (s *U2BServer) broadcastSSE(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(data))

	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- frame:
		default:
		}
	}
}

func serveMediaFile(w http.ResponseWriter, r *http.Request, filePath string, forceDownload bool) {
	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "无法打开媒体文件", http.StatusNotFound)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		http.Error(w, "无效的媒体文件", http.StatusNotFound)
		return
	}

	baseName := filepath.Base(filePath)
	ext := strings.ToLower(filepath.Ext(baseName))
	contentType := mime.TypeByExtension(ext)
	switch ext {
	case ".mp4":
		contentType = "video/mp4"
	case ".mp3":
		contentType = "audio/mpeg"
	case ".m4a":
		contentType = "audio/mp4"
	case ".webm":
		contentType = "video/webm"
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	disp := "inline"
	if forceDownload {
		disp = "attachment"
	}
	asciiFallback := "u2b_media" + ext
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disp, asciiFallback, url.PathEscape(baseName)))

	http.ServeContent(w, r, baseName, stat.ModTime(), f)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

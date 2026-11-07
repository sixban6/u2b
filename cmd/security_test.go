package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sixban6/u2b/config"
	"github.com/sixban6/u2b/downloader"
)

func TestMediaDomainAllowlistAndSSRFProtection(t *testing.T) {
	allowedInputs := map[string]string{
		"https://www.youtube.com/watch?v=jNQXAC9IVRw":                     "https://www.youtube.com/watch?v=jNQXAC9IVRw",
		"https://youtu.be/jNQXAC9IVRw":                                    "https://youtu.be/jNQXAC9IVRw",
		"https://www.bilibili.com/video/BV1xx411c7mD":                     "https://www.bilibili.com/video/BV1xx411c7mD",
		"【影视飓风】测试 https://b23.tv/abcDEF123，快来看！":                         "https://b23.tv/abcDEF123",
		"6.89 复制打开抖音，看看作品 https://v.douyin.com/iY12345/ ":                 "https://v.douyin.com/iY12345/",
		"小红书分享 http://xhslink.com/a/xyz789。":                              "http://xhslink.com/a/xyz789",
		"https://www.xiaohongshu.com/explore/66112233":                    "https://www.xiaohongshu.com/explore/66112233",
		"https://v.kuaishou.com/abc123":                                   "https://v.kuaishou.com/abc123",
		"https://x.com/elonmusk/status/1234567890":                        "https://x.com/elonmusk/status/1234567890",
		"https://twitter.com/user/status/1234567890":                      "https://twitter.com/user/status/1234567890",
		"https://www.tiktok.com/@creator/video/1234567890":                "https://www.tiktok.com/@creator/video/1234567890",
		"https://vm.tiktok.com/ZM123456/":                                 "https://vm.tiktok.com/ZM123456/",
	}

	for raw, want := range allowedInputs {
		got, err := config.ValidateAllowedMediaURL(raw)
		if err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("ValidateAllowedMediaURL(%q) = %q, want %q", raw, got, want)
		}
	}

	blockedInputs := []string{
		"",
		"http://127.0.0.1:8080/internal",
		"http://192.168.1.1/admin",
		"http://localhost:8989/api/config",
		"http://nas.local/secret.mp4",
		"https://evil.example.com/payload.exe",
		"https://youtube.com.evil.com/watch?v=123",
		"https://notyoutube.com/watch?v=123",
		"file:///etc/passwd",
		"https://admin:secret@www.youtube.com/watch?v=123",
	}

	for _, bad := range blockedInputs {
		if got, err := config.ValidateAllowedMediaURL(bad); err == nil {
			t.Errorf("expected %q to be blocked, but was allowed as %q", bad, got)
		}
	}
}

func TestServerCSRFAndOutputDirSandbox(t *testing.T) {
	srv := NewU2BServer("127.0.0.1:18995", 18995, "tok-sec-99")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Cross-origin POST with valid token must be blocked (403)
	csrfReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks", bytes.NewBufferString(`{"url":"https://www.youtube.com/watch?v=jNQXAC9IVRw"}`))
	csrfReq.Header.Set("Authorization", "Bearer tok-sec-99")
	csrfReq.Header.Set("Origin", "https://evil.attacker.com")
	csrfResp, err := http.DefaultClient.Do(csrfReq)
	if err != nil {
		t.Fatal(err)
	}
	csrfResp.Body.Close()
	if csrfResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-origin Origin header, got %d", csrfResp.StatusCode)
	}

	// 2. Top-level cross-site GET navigation (Sec-Fetch-Site: cross-site) must be blocked (403)
	navReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/quick?url=https://www.youtube.com/watch?v=jNQXAC9IVRw", nil)
	navReq.AddCookie(&http.Cookie{Name: "u2b_token", Value: "tok-sec-99"})
	navReq.Header.Set("Sec-Fetch-Site", "cross-site")
	navResp, err := http.DefaultClient.Do(navReq)
	if err != nil {
		t.Fatal(err)
	}
	navResp.Body.Close()
	if navResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Sec-Fetch-Site: cross-site, got %d", navResp.StatusCode)
	}

	// 3. Disallowed domain must be rejected (400)
	badDomReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks", bytes.NewBufferString(`{"url":"http://127.0.0.1:9999/secret"}`))
	badDomReq.Header.Set("Authorization", "Bearer tok-sec-99")
	badDomReq.Header.Set("Origin", ts.URL)
	badDomResp, err := http.DefaultClient.Do(badDomReq)
	if err != nil {
		t.Fatal(err)
	}
	badDomResp.Body.Close()
	if badDomResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for SSRF IP URL, got %d", badDomResp.StatusCode)
	}

	// 4. Unauthorized outputDir outside media roots must be rejected (403)
	unauthDir := t.TempDir()
	badDirReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/tasks", bytes.NewBufferString(`{"url":"https://www.bilibili.com/video/BV1xx411c7mD","outputDir":"`+strings.ReplaceAll(unauthDir, `\`, `\\`)+`"}`))
	badDirReq.Header.Set("Authorization", "Bearer tok-sec-99")
	badDirReq.Header.Set("Origin", ts.URL)
	badDirResp, err := http.DefaultClient.Do(badDirReq)
	if err != nil {
		t.Fatal(err)
	}
	badDirResp.Body.Close()
	if badDirResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unauthorized outputDir, got %d", badDirResp.StatusCode)
	}
}

func TestMacSeatbeltSandboxIsolation(t *testing.T) {
	if runtime.GOOS != "darwin" || !downloader.CanApplyMacSeatbelt() {
		t.Skip("macOS Seatbelt sandbox not applicable in this test environment")
	}

	allowedDir := t.TempDir()
	// Create a sibling directory outside allowedDir in UserHomeDir or /var/tmp?
	// Note: t.TempDir() is under /var/folders (which is a temp dir), so test writing to a temp path in ~/u2b_sb_test_forbidden if needed,
	// OR test reading blocked sensitive paths and writing outside temp!
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no homeDir")
	}

	// 1. Writing inside allowedDir must succeed
	okFile := filepath.Join(allowedDir, "allowed.mp4")
	cmdAllow := downloader.WrapCommandInSandbox(context.Background(), "/bin/sh", []string{"-c", "echo ok > " + okFile}, allowedDir, nil, false)
	if out, err := cmdAllow.CombinedOutput(); err != nil {
		t.Fatalf("expected write inside allowedDir to succeed, got err: %v (%s)", err, string(out))
	}

	// 2. Writing to ~/Library/LaunchAgents/com.evil.plist must be blocked by kernel Seatbelt!
	evilPlist := filepath.Join(homeDir, "Library", "LaunchAgents", "com.u2b.sandbox_test_should_fail.plist")
	defer os.Remove(evilPlist)
	cmdDenyWrite := downloader.WrapCommandInSandbox(context.Background(), "/bin/sh", []string{"-c", "echo pwn > " + evilPlist}, allowedDir, nil, false)
	if err := cmdDenyWrite.Run(); err == nil {
		os.Remove(evilPlist)
		t.Fatal("SECURITY FAILURE: sandboxed process was able to write to ~/Library/LaunchAgents!")
	}

	// 3. Reading ~/.ssh must be blocked by kernel Seatbelt!
	sshDir := filepath.Join(homeDir, ".ssh")
	if _, statErr := os.Stat(sshDir); statErr == nil {
		cmdDenyRead := downloader.WrapCommandInSandbox(context.Background(), "/bin/ls", []string{sshDir}, allowedDir, nil, false)
		if err := cmdDenyRead.Run(); err == nil {
			t.Fatal("SECURITY FAILURE: sandboxed process was able to list ~/.ssh!")
		}
	}

	// 4. Explicit --cookies file passed via extraReadFiles must be both readable and writable
	// (because yt-dlp's YoutubeDL.close() calls save_cookies() on exit).
	u2bDir := filepath.Join(homeDir, "u2b")
	_ = os.MkdirAll(u2bDir, 0700)
	testCookieFile := filepath.Join(u2bDir, ".sandbox_test_cookies.txt")
	_ = os.WriteFile(testCookieFile, []byte("# Netscape HTTP Cookie File\n"), 0600)
	defer os.Remove(testCookieFile)

	cmdCookieSave := downloader.WrapCommandInSandbox(
		context.Background(),
		"/bin/sh",
		[]string{"-c", "cat " + testCookieFile + " >/dev/null && echo '# updated' >> " + testCookieFile},
		allowedDir,
		[]string{testCookieFile},
		false,
	)
	if out, err := cmdCookieSave.CombinedOutput(); err != nil {
		t.Fatalf("expected explicit cookie file read/write to succeed in sandbox, got err: %v (%s)", err, string(out))
	}
}

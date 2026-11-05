package cmd

import (
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestQRGeneratorAndAppleVisionScan(t *testing.T) {
	targetURL := "http://sweetcsMBP.local:8989/setup?token=ab12cd34"

	svg, err := RenderQRSVG(targetURL)
	if err != nil {
		t.Fatalf("RenderQRSVG failed: %v", err)
	}
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "<path") {
		t.Fatalf("unexpected SVG output: %s", svg)
	}

	termQR, err := RenderQRTerminal(targetURL)
	if err != nil || len(termQR) == 0 {
		t.Fatalf("RenderQRTerminal failed: %v", err)
	}

	img, err := RenderQRPNGImage(targetURL, 8)
	if err != nil {
		t.Fatalf("RenderQRPNGImage failed: %v", err)
	}

	tmpPNG := filepath.Join(t.TempDir(), "qr.png")
	f, err := os.Create(tmpPNG)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// On macOS, verify with Vision VNDetectBarcodesRequest in full test mode
	if runtime.GOOS == "darwin" && !testing.Short() {
		if swiftPath, err := exec.LookPath("swift"); err == nil {
			script := `
import Foundation
import Vision
import AppKit

let url = URL(fileURLWithPath: CommandLine.arguments[1])
guard let img = NSImage(contentsOf: url),
      let cgImg = img.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    fputs("load_failed\n", stderr)
    exit(2)
}
let req = VNDetectBarcodesRequest()
req.symbologies = [.qr]
let handler = VNImageRequestHandler(cgImage: cgImg, options: [:])
try handler.perform([req])
if let first = req.results?.first, let payload = first.payloadStringValue {
    print(payload)
} else {
    fputs("no_qr_detected\n", stderr)
    exit(3)
}
`
			out, err := exec.Command(swiftPath, "-e", script, tmpPNG).CombinedOutput()
			if err != nil {
				t.Fatalf("Apple Vision QR decoder failed to scan generated QR code: %v (%s)", err, string(out))
			}
			decoded := strings.TrimSpace(string(out))
			if decoded != targetURL {
				t.Fatalf("decoded QR mismatch: got %q, want %q", decoded, targetURL)
			}
		}
	}
}

func TestGenerateAndSignIOSShortcut(t *testing.T) {
	xmlStr := GenerateIOSShortcutXML("http://sweetcsMBP.local:8989", "sec123")
	if !strings.Contains(xmlStr, "is.workflow.actions.choosefrommenu") {
		t.Fatal("missing choosefrommenu action in shortcut XML")
	}
	if !strings.Contains(xmlStr, "http://sweetcsMBP.local:8989/api/quick?format=mp4&amp;plain=1&amp;token=sec123&amp;url=") {
		t.Fatal("missing encoded quick MP4 endpoint in shortcut XML")
	}

	if runtime.GOOS == "darwin" {
		if plutilPath, err := exec.LookPath("plutil"); err == nil {
			tmpPlist := filepath.Join(t.TempDir(), "test.plist")
			if err := os.WriteFile(tmpPlist, []byte(xmlStr), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(plutilPath, "-lint", tmpPlist).CombinedOutput(); err != nil {
				t.Fatalf("plutil -lint rejected generated shortcut XML: %v (%s)", err, string(out))
			}
		}

		if CanSignShortcuts() && !testing.Short() {
			signedBytes, isSigned, err := BuildSignedIOSShortcut("http://sweetcsMBP.local:8989", "sec123")
			if err != nil {
				t.Fatalf("BuildSignedIOSShortcut failed on macOS: %v", err)
			}
			if !isSigned || len(signedBytes) < 1024 {
				t.Fatalf("expected Apple-signed shortcut binary (>1KB), got isSigned=%v len=%d", isSigned, len(signedBytes))
			}
		}
	}
}

func TestServerEndpointsAuthAndFileStreaming(t *testing.T) {
	srv := NewU2BServer("127.0.0.1:18989", 18989, "my-secret")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 1. Public healthz & PWA UI
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", resp.StatusCode)
	}

	uiResp, err := http.Get(ts.URL + "/setup")
	if err != nil {
		t.Fatal(err)
	}
	uiBody, _ := io.ReadAll(uiResp.Body)
	uiResp.Body.Close()
	if uiResp.StatusCode != http.StatusOK || !strings.Contains(string(uiBody), "iPhone 接入引导") {
		t.Fatalf("unexpected /setup response: status=%d", uiResp.StatusCode)
	}

	// 2. Unauthorized without token
	unauthResp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", unauthResp.StatusCode)
	}

	// 3. Authorized via query token & header
	cfgResp, err := http.Get(ts.URL + "/api/config?token=my-secret")
	if err != nil {
		t.Fatal(err)
	}
	var cfgPayload map[string]any
	if err := json.NewDecoder(cfgResp.Body).Decode(&cfgPayload); err != nil {
		cfgResp.Body.Close()
		t.Fatal(err)
	}
	cfgResp.Body.Close()
	if cfgPayload["ok"] != true {
		t.Fatalf("expected ok=true from /api/config, got %+v", cfgPayload)
	}

	// 4. Test completed file download & HTTP Range support (for iOS Brave playback/save)
	sampleFile := filepath.Join(t.TempDir(), "测试视频.mp4")
	sampleBytes := []byte("0123456789ABCDEF")
	if err := os.WriteFile(sampleFile, sampleBytes, 0644); err != nil {
		t.Fatal(err)
	}

	srv.mu.Lock()
	srv.tasks["task-demo-1"] = &ServerTask{
		ID:       "task-demo-1",
		Title:    "测试视频",
		Format:   "mp4",
		Status:   "completed",
		FilePath: sampleFile,
		FileSize: int64(len(sampleBytes)),
	}
	srv.mu.Unlock()

	dlReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/files/task-demo-1?download=1", nil)
	dlReq.Header.Set("Authorization", "Bearer my-secret")
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		t.Fatal(err)
	}
	gotAll, _ := io.ReadAll(dlResp.Body)
	dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK || !bytes.Equal(gotAll, sampleBytes) {
		t.Fatalf("unexpected file download response: status=%d body=%q", dlResp.StatusCode, string(gotAll))
	}
	if !strings.Contains(dlResp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("expected attachment Content-Disposition, got %q", dlResp.Header.Get("Content-Disposition"))
	}

	// Range request
	rangeReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/files/task-demo-1", nil)
	rangeReq.Header.Set("X-U2B-Token", "my-secret")
	rangeReq.Header.Set("Range", "bytes=0-3")
	rangeResp, err := http.DefaultClient.Do(rangeReq)
	if err != nil {
		t.Fatal(err)
	}
	gotRange, _ := io.ReadAll(rangeResp.Body)
	rangeResp.Body.Close()
	if rangeResp.StatusCode != http.StatusPartialContent || string(gotRange) != "0123" {
		t.Fatalf("expected 206 Partial Content '0123', got status=%d body=%q", rangeResp.StatusCode, string(gotRange))
	}
}

func TestVirtualInterfaceFilteringAndSetupUI(t *testing.T) {
	virtualIfaces := []string{"bridge0", "bridge100", "vmenet0", "vmnet8", "docker0", "br-abc123", "veth9f12", "utun4", "vEthernet (WSL)"}
	for _, name := range virtualIfaces {
		if !isVirtualOrBridgeInterface(name) {
			t.Errorf("expected virtual interface %q to be filtered out", name)
		}
	}

	realIfaces := []string{"en0", "en1", "eth0", "wlan0", "Wi-Fi", "Ethernet"}
	for _, name := range realIfaces {
		if isVirtualOrBridgeInterface(name) {
			t.Errorf("expected physical LAN interface %q NOT to be filtered out", name)
		}
	}

	srv := NewU2BServer("127.0.0.1:18990", 18990, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/setup")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	html := string(body)

	for _, want := range []string{"方式一：手机扫码", "方式二：安装快捷指令"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected /setup HTML to contain %q", want)
		}
	}
	if strings.Contains(html, "方案 B：主屏幕 Web App") {
		t.Error("expected /setup HTML NOT to contain deleted '方案 B：主屏幕 Web App'")
	}
}


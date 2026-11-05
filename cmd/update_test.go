package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildTestTarGz(t *testing.T, entries map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func buildTestZip(t *testing.T, entries map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func TestCompareSemverAndValidation(t *testing.T) {
	if compareSemver("1.0.2", "v1.0.2") != 0 {
		t.Fatal("expected 1.0.2 == v1.0.2")
	}
	if compareSemver("v1.0.2", "v1.0.10") >= 0 {
		t.Fatal("expected v1.0.2 < v1.0.10")
	}
	if compareSemver("v1.1.0", "v1.0.99") <= 0 {
		t.Fatal("expected v1.1.0 > v1.0.99")
	}
	if compareSemver("v1.0.0-rc1", "v1.0.0") >= 0 {
		t.Fatal("expected prerelease < release")
	}

	if err := validateReleaseTag("v1.2.3"); err != nil {
		t.Fatalf("unexpected error for valid tag: %v", err)
	}
	for _, bad := range []string{"", "latest", "v1.2", "../v1.0.0", "v1.0.0/../../evil"} {
		if err := validateReleaseTag(bad); err == nil {
			t.Fatalf("expected error for invalid tag %q", bad)
		}
	}
}

func TestVerifiedSelfUpdateTarGzAndZip(t *testing.T) {
	tarBytes, tarSHA := buildTestTarGz(t, map[string][]byte{
		"u2b":       []byte("new-linux-binary-v1.9.0"),
		"README.md": []byte("# u2b"),
	})
	zipBytes, zipSHA := buildTestZip(t, map[string][]byte{
		"u2b.exe":   []byte("new-windows-binary-v1.9.0"),
		"README.md": []byte("# u2b"),
	})
	checksumsTxt := fmt.Sprintf("%s  u2b-linux-amd64.tar.gz\n%s  u2b-windows-amd64.zip\n", tarSHA, zipSHA)

	var ts *httptest.Server
	ts = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/sixban6/u2b/releases/latest":
			_ = json.NewEncoder(w).Encode(githubReleaseResponse{
				TagName: "v1.9.0",
				Assets: []githubReleaseAsset{
					{Name: "u2b-linux-amd64.tar.gz", Digest: "sha256:" + tarSHA},
					{Name: "u2b-windows-amd64.zip", Digest: "sha256:" + zipSHA},
				},
			})
		case "/sixban6/u2b/releases/download/v1.9.0/u2b-linux-amd64.tar.gz":
			_, _ = w.Write(tarBytes)
		case "/sixban6/u2b/releases/download/v1.9.0/u2b-windows-amd64.zip":
			_, _ = w.Write(zipBytes)
		case "/sixban6/u2b/releases/download/v1.9.0/checksums.txt":
			_, _ = w.Write([]byte(checksumsTxt))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// 1. Test Linux .tar.gz update
	tmpDir := t.TempDir()
	linuxTarget := filepath.Join(tmpDir, "u2b")
	if err := os.WriteFile(linuxTarget, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	srcLinux := releaseSourceConfig{
		Repo:       "sixban6/u2b",
		APIBase:    ts.URL,
		WebBase:    ts.URL,
		Client:     ts.Client(),
		TargetOS:   "linux",
		TargetArch: "amd64",
	}

	meta, err := performVerifiedUpdate(context.Background(), srcLinux, linuxTarget, "latest", nil, false, false, false)
	if err != nil {
		t.Fatalf("performVerifiedUpdate (tar.gz) failed: %v", err)
	}
	if meta.Tag != "v1.9.0" {
		t.Fatalf("expected v1.9.0, got %s", meta.Tag)
	}
	gotLinux, _ := os.ReadFile(linuxTarget)
	if string(gotLinux) != "new-linux-binary-v1.9.0" {
		t.Fatalf("unexpected updated binary content: %q", string(gotLinux))
	}

	// 2. Test Windows .zip update
	winTarget := filepath.Join(tmpDir, "u2b.exe")
	if err := os.WriteFile(winTarget, []byte("old-exe"), 0755); err != nil {
		t.Fatal(err)
	}
	srcWin := srcLinux
	srcWin.TargetOS = "windows"
	if _, err := performVerifiedUpdate(context.Background(), srcWin, winTarget, "latest", nil, false, false, false); err != nil {
		t.Fatalf("performVerifiedUpdate (zip) failed: %v", err)
	}
	gotWin, _ := os.ReadFile(winTarget)
	if string(gotWin) != "new-windows-binary-v1.9.0" {
		t.Fatalf("unexpected updated Windows binary content: %q", string(gotWin))
	}
}

func TestSelfUpdateRejectsTamperedArchiveAndLeavesOriginalIntact(t *testing.T) {
	tarBytes, _ := buildTestTarGz(t, map[string][]byte{
		"u2b": []byte("tampered-malicious-binary"),
	})
	wrongSHA := strings.Repeat("a", 64)

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/sixban6/u2b/releases/latest":
			_ = json.NewEncoder(w).Encode(githubReleaseResponse{
				TagName: "v1.9.1",
				Assets: []githubReleaseAsset{
					{Name: "u2b-linux-amd64.tar.gz", Digest: "sha256:" + wrongSHA},
				},
			})
		case "/sixban6/u2b/releases/download/v1.9.1/u2b-linux-amd64.tar.gz":
			_, _ = w.Write(tarBytes)
		case "/sixban6/u2b/releases/download/v1.9.1/checksums.txt":
			_, _ = w.Write([]byte(wrongSHA + "  u2b-linux-amd64.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	target := filepath.Join(t.TempDir(), "u2b")
	if err := os.WriteFile(target, []byte("safe-original-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	src := releaseSourceConfig{
		Repo:       "sixban6/u2b",
		APIBase:    ts.URL,
		WebBase:    ts.URL,
		Client:     ts.Client(),
		TargetOS:   "linux",
		TargetArch: "amd64",
	}

	_, err := performVerifiedUpdate(context.Background(), src, target, "latest", nil, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "SECURITY ALERT") {
		t.Fatalf("expected SECURITY ALERT checksum failure, got: %v", err)
	}

	got, _ := os.ReadFile(target)
	if string(got) != "safe-original-binary" {
		t.Fatalf("existing binary was modified despite failed verification! got %q", string(got))
	}
}

func TestArchivePathTraversalBlocked(t *testing.T) {
	tmpDir := t.TempDir()

	evilTarBytes, _ := buildTestTarGz(t, map[string][]byte{
		"../evil": []byte("pwn"),
	})
	evilTarPath := filepath.Join(tmpDir, "evil.tar.gz")
	_ = os.WriteFile(evilTarPath, evilTarBytes, 0600)
	if err := extractBinaryFromTarGz(evilTarPath, "u2b", filepath.Join(tmpDir, "out1")); err == nil || !strings.Contains(err.Error(), "SECURITY ALERT") {
		t.Fatalf("expected Tar-Slip SECURITY ALERT, got: %v", err)
	}

	evilZipBytes, _ := buildTestZip(t, map[string][]byte{
		"../../evil.exe": []byte("pwn"),
	})
	evilZipPath := filepath.Join(tmpDir, "evil.zip")
	_ = os.WriteFile(evilZipPath, evilZipBytes, 0600)
	if err := extractBinaryFromZip(evilZipPath, "u2b.exe", filepath.Join(tmpDir, "out2")); err == nil || !strings.Contains(err.Error(), "SECURITY ALERT") {
		t.Fatalf("expected Zip-Slip SECURITY ALERT, got: %v", err)
	}
}

func TestAPIRateLimitFallbackToRedirect(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/sixban6/u2b/releases/latest":
			w.WriteHeader(http.StatusForbidden) // simulate anonymous rate-limit
		case "/sixban6/u2b/releases/latest":
			http.Redirect(w, r, ts.URL+"/sixban6/u2b/releases/tag/v1.8.5", http.StatusFound)
		case "/sixban6/u2b/releases/tag/v1.8.5":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	src := releaseSourceConfig{
		Repo:       "sixban6/u2b",
		APIBase:    ts.URL,
		WebBase:    ts.URL,
		Client:     ts.Client(),
		TargetOS:   "darwin",
		TargetArch: "arm64",
	}

	meta, err := resolveReleaseMetadata(context.Background(), src, "latest")
	if err != nil {
		t.Fatalf("resolveReleaseMetadata fallback failed: %v", err)
	}
	if meta.Tag != "v1.8.5" {
		t.Fatalf("expected fallback tag v1.8.5, got %s", meta.Tag)
	}
}

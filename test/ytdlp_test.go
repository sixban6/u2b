package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sr.ht/~sixban/u2b_download/config"
	"git.sr.ht/~sixban/u2b_download/downloader"
)

func TestBuildDownloadArgsMP3(t *testing.T) {
	y := &downloader.YtdlpExecutor{Config: config.DefaultConfig()}

	args := y.BuildDownloadArgs(&downloader.YtdlpOptions{
		URL:            "https://example.com/watch?v=abc123",
		Quality:        "1080",
		Format:         config.OutputFormatMP3,
		AudioQuality:   "192K",
		OutputDir:      "/tmp",
		OutputTemplate: "%(title)s.%(ext)s",
	})

	if !hasArgPair(args, "--format", "bestaudio/best") {
		t.Fatalf("expected mp3 mode to use bestaudio selector, args: %v", args)
	}
	if !hasArg(args, "--extract-audio") {
		t.Fatalf("expected mp3 mode to add --extract-audio, args: %v", args)
	}
	if !hasArgPair(args, "--audio-format", config.OutputFormatMP3) {
		t.Fatalf("expected mp3 mode to set --audio-format mp3, args: %v", args)
	}
	if !hasArgPair(args, "--audio-quality", "192K") {
		t.Fatalf("expected mp3 mode to include --audio-quality, args: %v", args)
	}
	for _, arg := range args {
		if strings.Contains(arg, "height<=") {
			t.Fatalf("did not expect video quality selector in mp3 mode, args: %v", args)
		}
	}
}

func TestBuildDownloadArgsVideoMode(t *testing.T) {
	y := &downloader.YtdlpExecutor{Config: config.DefaultConfig()}

	args := y.BuildDownloadArgs(&downloader.YtdlpOptions{
		URL:            "https://example.com/watch?v=abc123",
		Quality:        config.Quality720p,
		OutputDir:      "/tmp",
		OutputTemplate: "%(title)s.%(ext)s",
	})

	if !hasArgPair(args, "--format", "bestvideo[height<=720]+bestaudio/best[height<=720]/best") {
		t.Fatalf("expected video mode to use DASH video+audio quality selector, args: %v", args)
	}
	if !hasArgPair(args, "--merge-output-format", "mp4") {
		t.Fatalf("expected video mode to set --merge-output-format mp4, args: %v", args)
	}
	if hasArg(args, "--extract-audio") {
		t.Fatalf("did not expect --extract-audio in video mode, args: %v", args)
	}
	if hasArg(args, "--audio-format") {
		t.Fatalf("did not expect --audio-format in video mode, args: %v", args)
	}
}

func TestBuildDownloadArgsMP3MapsNamedAudioQuality(t *testing.T) {
	y := &downloader.YtdlpExecutor{Config: config.DefaultConfig()}

	args := y.BuildDownloadArgs(&downloader.YtdlpOptions{
		URL:            "https://example.com/watch?v=abc123",
		Format:         config.OutputFormatMP3,
		AudioQuality:   "best",
		OutputDir:      "/tmp",
		OutputTemplate: "%(title)s.%(ext)s",
	})

	if !hasArgPair(args, "--audio-quality", "0") {
		t.Fatalf("expected named quality 'best' to map to '0', args: %v", args)
	}
}

func TestParseDownloadResultUsesDetectedExtension(t *testing.T) {
	y := &downloader.YtdlpExecutor{Config: config.DefaultConfig()}
	result := &downloader.DownloadResult{}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample_track.mp3")
	if err := os.WriteFile(filePath, []byte("audio"), 0644); err != nil {
		t.Fatalf("failed to prepare test file: %v", err)
	}

	err := y.ParseDownloadResult(result, []string{
		"[download] Destination: " + filePath,
	}, &downloader.YtdlpOptions{
		Format: config.OutputFormatMP3,
	})
	if err != nil {
		t.Fatalf("ParseDownloadResult returned error: %v", err)
	}

	if result.FilePath != filePath {
		t.Fatalf("expected file path %q, got %q", filePath, result.FilePath)
	}
	if result.Format != config.OutputFormatMP3 {
		t.Fatalf("expected format %q, got %q", config.OutputFormatMP3, result.Format)
	}
	if result.FileSize == 0 {
		t.Fatalf("expected non-zero file size, got %d", result.FileSize)
	}
}

func TestParseDownloadResultPrefersMergedFile(t *testing.T) {
	y := &downloader.YtdlpExecutor{Config: config.DefaultConfig()}
	result := &downloader.DownloadResult{}

	tmpDir := t.TempDir()
	mergedPath := filepath.Join(tmpDir, "video.mp4")
	if err := os.WriteFile(mergedPath, []byte("merged-video-content"), 0644); err != nil {
		t.Fatalf("failed to prepare merged file: %v", err)
	}

	err := y.ParseDownloadResult(result, []string{
		"[download] Destination: " + filepath.Join(tmpDir, "video.f399.mp4"),
		"[download] Destination: " + filepath.Join(tmpDir, "video.f251.webm"),
		`[Merger] Merging formats into "` + mergedPath + `"`,
	}, &downloader.YtdlpOptions{
		Quality: "1080",
	})
	if err != nil {
		t.Fatalf("ParseDownloadResult returned error: %v", err)
	}

	if result.FilePath != mergedPath {
		t.Fatalf("expected merged file path %q, got %q", mergedPath, result.FilePath)
	}
	if result.Format != "mp4" {
		t.Fatalf("expected format mp4, got %q", result.Format)
	}
}

func TestCompareYtdlpVersions(t *testing.T) {
	if downloader.CompareYtdlpVersions("2026.08.19", "2026.10.26") >= 0 {
		t.Fatalf("expected 2026.08.19 < 2026.10.26")
	}
	if downloader.CompareYtdlpVersions("2026.10.26", "2026.10.26") != 0 {
		t.Fatalf("expected 2026.10.26 == 2026.10.26")
	}
	if downloader.CompareYtdlpVersions("2026.11.01", "2026.10.26") <= 0 {
		t.Fatalf("expected 2026.11.01 > 2026.10.26")
	}
}

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func hasArgPair(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

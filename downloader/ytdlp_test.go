package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sr.ht/~sixban/u2b_download/config"
)

func TestBuildDownloadArgsMP3(t *testing.T) {
	y := &YtdlpExecutor{config: config.DefaultConfig()}

	args := y.buildDownloadArgs(&YtdlpOptions{
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
	y := &YtdlpExecutor{config: config.DefaultConfig()}

	args := y.buildDownloadArgs(&YtdlpOptions{
		URL:            "https://example.com/watch?v=abc123",
		Quality:        config.Quality720p,
		OutputDir:      "/tmp",
		OutputTemplate: "%(title)s.%(ext)s",
	})

	if !hasArgPair(args, "--format", "best[height<=720]/best") {
		t.Fatalf("expected video mode to use quality selector, args: %v", args)
	}
	if hasArg(args, "--extract-audio") {
		t.Fatalf("did not expect --extract-audio in video mode, args: %v", args)
	}
	if hasArg(args, "--audio-format") {
		t.Fatalf("did not expect --audio-format in video mode, args: %v", args)
	}
}

func TestBuildDownloadArgsMP3MapsNamedAudioQuality(t *testing.T) {
	y := &YtdlpExecutor{config: config.DefaultConfig()}

	args := y.buildDownloadArgs(&YtdlpOptions{
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
	y := &YtdlpExecutor{config: config.DefaultConfig()}
	result := &DownloadResult{}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample_track.mp3")
	if err := os.WriteFile(filePath, []byte("audio"), 0644); err != nil {
		t.Fatalf("failed to prepare test file: %v", err)
	}

	err := y.parseDownloadResult(result, []string{
		"[download] Destination: " + filePath,
	}, &YtdlpOptions{
		Format: config.OutputFormatMP3,
	})
	if err != nil {
		t.Fatalf("parseDownloadResult returned error: %v", err)
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

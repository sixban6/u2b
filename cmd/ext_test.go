package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateExtensionIconPNG(t *testing.T) {
	for _, size := range []int{16, 48, 128} {
		data, err := generateExtensionIconPNG(size)
		if err != nil {
			t.Fatalf("failed to generate icon for size %d: %v", size, err)
		}
		if len(data) == 0 {
			t.Fatalf("generated icon for size %d is empty", size)
		}
		// PNG header: \x89PNG\r\n\x1a\n
		if len(data) < 8 || string(data[1:4]) != "PNG" {
			t.Fatalf("data is not a valid PNG header for size %d", size)
		}
	}
}

func TestComputePathExtensionID(t *testing.T) {
	tmpDir := t.TempDir()
	id1 := computePathExtensionID(tmpDir)
	if len(id1) != 32 {
		t.Fatalf("expected 32-char ID, got %d chars: %s", len(id1), id1)
	}
	for _, c := range id1 {
		if c < 'a' || c > 'p' {
			t.Fatalf("invalid character %c in extension ID %s", c, id1)
		}
	}

	id2 := computePathExtensionID(tmpDir)
	if id1 != id2 {
		t.Fatalf("extension ID is not deterministic: %s vs %s", id1, id2)
	}
}

func TestDetectInstalledBrowsers(t *testing.T) {
	browsers := detectInstalledBrowsers()
	// Should not panic, and each browser returned should have valid fields
	for _, b := range browsers {
		if b.ID == "" || b.Name == "" || b.Scheme == "" || b.ExecPath == "" {
			t.Errorf("browser info contains empty fields: %+v", b)
		}
		if fi, err := os.Stat(b.ExecPath); err != nil {
			t.Errorf("detected browser executable path not found: %s (%v)", b.ExecPath, err)
		} else if fi.IsDir() && filepath.Ext(b.ExecPath) != ".app" {
			t.Errorf("detected browser executable is unexpected directory: %s", b.ExecPath)
		}
	}
}

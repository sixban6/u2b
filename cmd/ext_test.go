package cmd

import (
	"os"
	"path/filepath"
	"strings"
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

func TestGenerateNavHTML(t *testing.T) {
	htmlContent := generateNavHTML("Brave Browser", "brave://extensions", "/home/test/u2b/extension", "/home/test/u2b")
	if len(htmlContent) == 0 {
		t.Fatal("nav.html content is empty")
	}
	expectedStrings := []string{
		"Brave Browser",
		"brave://extensions",
		"/home/test/u2b/extension",
		"开发者模式",
		"copyText",
	}
	for _, s := range expectedStrings {
		if !strings.Contains(htmlContent, s) {
			t.Errorf("expected nav.html to contain %q", s)
		}
	}
}

func TestEnableDeveloperModeInFile(t *testing.T) {
	tmpDir := t.TempDir()
	prefFile := filepath.Join(tmpDir, "Preferences")
	initialJSON := `{"extensions": {"other": 1}}`
	if err := os.WriteFile(prefFile, []byte(initialJSON), 0644); err != nil {
		t.Fatal(err)
	}

	updated := enableDeveloperModeInFile(prefFile)
	if !updated {
		t.Fatal("expected enableDeveloperModeInFile to return true")
	}

	data, err := os.ReadFile(prefFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"developer_mode": true`) {
		t.Fatalf("expected developer_mode: true in file, got: %s", string(data))
	}

	// Second invocation should return false (already enabled)
	updatedAgain := enableDeveloperModeInFile(prefFile)
	if updatedAgain {
		t.Fatal("expected second enableDeveloperModeInFile to return false")
	}
}

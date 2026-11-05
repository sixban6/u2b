package downloader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	seatbeltCheckOnce sync.Once
	seatbeltAvailable bool
)

// CanApplyMacSeatbelt probes once whether macOS `/usr/bin/sandbox-exec` is available
// and can apply a sandbox profile (i.e. current process is not already nested inside a strict sandbox).
func CanApplyMacSeatbelt() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	seatbeltCheckOnce.Do(func() {
		sbPath, err := exec.LookPath("sandbox-exec")
		if err != nil || sbPath == "" {
			seatbeltAvailable = false
			return
		}
		err = exec.Command(sbPath, "-p", "(version 1)(allow default)", "/usr/bin/true").Run()
		seatbeltAvailable = (err == nil)
	})
	return seatbeltAvailable
}

func resolveRealPathForSandbox(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		abs = trimmed
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	// If target directory/file doesn't exist yet, resolve symlinks on its parent directory
	parent := filepath.Dir(abs)
	if realParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(realParent, filepath.Base(abs))
	}
	return abs
}

func escapeSBPLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// ExtractCookieFilesFromArgs inspects yt-dlp auth arguments and returns any explicit `--cookies <file>` paths.
func ExtractCookieFilesFromArgs(args []string) []string {
	var files []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--cookies" && strings.TrimSpace(args[i+1]) != "" {
			files = append(files, args[i+1])
		}
	}
	return files
}

// BuildMacSeatbeltProfile generates a per-task macOS Seatbelt (SBPL) profile that:
//  1. Blocks reading sensitive credentials/keys (~/.ssh, ~/.aws, ~/.gnupg, ~/.kube, ~/Desktop, ~/Documents, server_token, shell rc files)
//  2. Denies ALL disk writes by default (`(deny file-write*)`), allowing writes ONLY to `outputDir`, temp dirs, and yt-dlp's cache dir.
func BuildMacSeatbeltProfile(outputDir string, extraReadFiles []string) string {
	homeDir, _ := os.UserHomeDir()
	realHome := resolveRealPathForSandbox(homeDir)
	realOut := resolveRealPathForSandbox(outputDir)
	realTmp := resolveRealPathForSandbox(os.TempDir())

	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")

	if realHome != "" {
		deniedSubpaths := []string{
			filepath.Join(realHome, ".ssh"),
			filepath.Join(realHome, ".gnupg"),
			filepath.Join(realHome, ".aws"),
			filepath.Join(realHome, ".kube"),
			filepath.Join(realHome, ".docker"),
			filepath.Join(realHome, "Desktop"),
			filepath.Join(realHome, "Documents"),
		}
		deniedLiterals := []string{
			filepath.Join(realHome, "u2b", "server_token"),
			filepath.Join(realHome, ".zshrc"),
			filepath.Join(realHome, ".zshenv"),
			filepath.Join(realHome, ".bashrc"),
			filepath.Join(realHome, ".bash_profile"),
			filepath.Join(realHome, ".gitconfig"),
			filepath.Join(realHome, ".netrc"),
			filepath.Join(realHome, ".npmrc"),
			filepath.Join(realHome, ".pypirc"),
		}

		b.WriteString("(deny file-read*\n")
		for _, sp := range deniedSubpaths {
			fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(sp))
		}
		for _, lit := range deniedLiterals {
			fmt.Fprintf(&b, "  (literal \"%s\")\n", escapeSBPLString(lit))
		}
		b.WriteString(")\n")
	}

	// Allow stat() metadata traversal so getcwd()/realpath() never fail on parent path segments
	b.WriteString("(allow file-read-metadata)\n")

	// Re-allow explicit outputDir and cookie files even if user placed outputDir under ~/Documents or ~/Desktop
	if realOut != "" || len(extraReadFiles) > 0 {
		b.WriteString("(allow file-read*\n")
		if realOut != "" {
			fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(realOut))
		}
		for _, rf := range extraReadFiles {
			if realRF := resolveRealPathForSandbox(rf); realRF != "" {
				fmt.Fprintf(&b, "  (literal \"%s\")\n", escapeSBPLString(realRF))
			}
		}
		b.WriteString(")\n")
	}

	// Default-deny ALL file writes across the entire filesystem, allowing only target outputDir & temp/cache dirs
	b.WriteString("(deny file-write*)\n")
	b.WriteString("(allow file-write*\n")
	if realOut != "" {
		fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(realOut))
	}
	if realTmp != "" {
		fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(realTmp))
	}
	b.WriteString("  (subpath \"/private/tmp\")\n")
	b.WriteString("  (subpath \"/private/var/folders\")\n")
	if realHome != "" {
		fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(filepath.Join(realHome, ".cache", "yt-dlp")))
		fmt.Fprintf(&b, "  (subpath \"%s\")\n", escapeSBPLString(filepath.Join(realHome, "Library", "Caches", "yt-dlp")))
	}
	b.WriteString("  (literal \"/dev/null\")\n")
	b.WriteString("  (literal \"/dev/tty\")\n")
	b.WriteString("  (literal \"/dev/dtracehelper\")\n")
	b.WriteString(")\n")

	return b.String()
}

// WrapCommandInSandbox wraps an external media tool invocation (yt-dlp / ffmpeg) inside
// macOS Seatbelt (`sandbox-exec`) when running on macOS and sandboxing is enabled.
func WrapCommandInSandbox(ctx context.Context, binPath string, args []string, outputDir string, extraReadFiles []string, disableSandbox bool) *exec.Cmd {
	if disableSandbox || os.Getenv("U2B_NO_SANDBOX") == "1" || !CanApplyMacSeatbelt() {
		return exec.CommandContext(ctx, binPath, args...)
	}

	sbPath, err := exec.LookPath("sandbox-exec")
	if err != nil || sbPath == "" {
		return exec.CommandContext(ctx, binPath, args...)
	}

	profile := BuildMacSeatbeltProfile(outputDir, extraReadFiles)
	sbArgs := make([]string, 0, len(args)+3)
	sbArgs = append(sbArgs, "-p", profile, binPath)
	sbArgs = append(sbArgs, args...)

	return exec.CommandContext(ctx, sbPath, sbArgs...)
}

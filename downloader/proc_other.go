//go:build !windows

package downloader

import "os/exec"

// HideConsole is a no-op on non-Windows platforms.
func HideConsole(cmd *exec.Cmd) {}

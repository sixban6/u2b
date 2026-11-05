package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sixban6/u2b/config"
	"github.com/spf13/cobra"
)

const (
	defaultGitHubRepo        = "sixban6/u2b"
	defaultGitHubAPIBase     = "https://api.github.com"
	defaultGitHubWebBase     = "https://github.com"
	maxReleaseArchiveBytes   = 150 * 1024 * 1024 // 150 MB upper bound against decompression bombs
	maxExtractedBinaryBytes  = 150 * 1024 * 1024 // 150 MB upper bound for extracted u2b binary
)

var (
	semverTagPattern = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)(?:[.-]([0-9A-Za-z.-]+))?$`)
	hexSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

	updateCheckOnly          bool
	updateTargetVersion      string
	updateForce              bool
	updateSkipVerify         bool
	updateRequireAttestation bool
	updateDeps               bool
)

var updateCmd = &cobra.Command{
	Use:     "update",
	Aliases: []string{"upgrade", "self-update"},
	Short:   "Update u2b to the latest GitHub Release with security verification",
	Long: `Automatically download, verify (HTTPS TLS 1.2+, SHA-256 checksums, GitHub API digest,
optional SLSA provenance attestation, and code signature), and atomically replace the
installed u2b binary without needing curl or PowerShell scripts.

Examples:
  u2b update                        # Update to the latest GitHub Release
  u2b update --check                # Only check if a newer version is available
  u2b update --version v1.0.2       # Install a specific release version
  u2b update --deps                 # Update u2b and also upgrade yt-dlp
  u2b update --require-attestation  # Enforce GitHub SLSA attestation via 'gh'`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

func init() {
	if runtime.GOOS == "windows" {
		cleanupWindowsOldExecutable()
	}

	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "check for updates without installing")
	updateCmd.Flags().StringVar(&updateTargetVersion, "version", "latest", "target release version tag (default: latest)")
	updateCmd.Flags().BoolVarP(&updateForce, "force", "f", false, "reinstall even if already on the target version")
	updateCmd.Flags().BoolVar(&updateSkipVerify, "skip-verify", false, "skip SHA-256 and signature checks (NOT recommended)")
	updateCmd.Flags().BoolVar(&updateRequireAttestation, "require-attestation", false, "require GitHub SLSA provenance verification via 'gh'")
	updateCmd.Flags().BoolVar(&updateDeps, "deps", false, "also upgrade yt-dlp if installed")
	updateCmd.Flags().StringVar(&proxy, "proxy", "", "HTTP/SOCKS5 proxy URL for downloading from GitHub")

	rootCmd.AddCommand(updateCmd)
}

func cleanupWindowsOldExecutable() {
	if exe, err := os.Executable(); err == nil && exe != "" {
		_ = os.Remove(exe + ".old")
	}
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type githubReleaseResponse struct {
	TagName string               `json:"tag_name"`
	HTMLURL string               `json:"html_url"`
	Assets  []githubReleaseAsset `json:"assets"`
}

type releaseSourceConfig struct {
	Repo      string
	APIBase   string
	WebBase   string
	Client    *http.Client
	TargetOS  string
	TargetArch string
}

type resolvedReleaseMeta struct {
	Tag        string
	AssetName  string
	ArchiveURL string
	SumsURL    string
	APISHA256  string
}

func defaultReleaseSourceConfig() releaseSourceConfig {
	return releaseSourceConfig{
		Repo:       defaultGitHubRepo,
		APIBase:    defaultGitHubAPIBase,
		WebBase:    defaultGitHubWebBase,
		Client:     newUpdateHTTPClient(5 * time.Minute),
		TargetOS:   runtime.GOOS,
		TargetArch: detectEffectiveArch(),
	}
}

func detectEffectiveArch() string {
	// If running an amd64 binary under Rosetta 2 on Apple Silicon, prefer native arm64 upgrade!
	if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		if out, err := exec.Command("sysctl", "-n", "hw.optional.arm64").Output(); err == nil {
			if strings.TrimSpace(string(out)) == "1" {
				return "arm64"
			}
		}
	}
	return runtime.GOARCH
}

func newUpdateHTTPClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	proxyStr := strings.TrimSpace(proxy)
	if proxyStr == "" {
		cfgPath := getEffectiveConfigPath()
		if cfgPath != "" {
			if _, err := os.Stat(cfgPath); err == nil {
				if cfg, err := config.LoadConfig(cfgPath); err == nil && cfg != nil && cfg.Proxy.Enabled {
					proxyStr = strings.TrimSpace(cfg.Proxy.URL)
				}
			}
		}
	}
	if proxyStr != "" {
		if parsed, err := url.Parse(proxyStr); err == nil {
			tr.Proxy = http.ProxyURL(parsed)
		}
	}

	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}
}

func normalizeSemverTag(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "v") {
		trimmed = "v" + trimmed
	}
	return trimmed
}

func validateReleaseTag(tag string) error {
	if !semverTagPattern.MatchString(tag) {
		return fmt.Errorf("invalid release version tag %q (expected vMAJOR.MINOR.PATCH)", tag)
	}
	return nil
}

// compareSemver returns -1 if a < b, 0 if a == b, +1 if a > b.
// Falls back to string inequality check if non-standard tags are passed.
func compareSemver(a, b string) int {
	na := normalizeSemverTag(a)
	nb := normalizeSemverTag(b)
	if na == nb {
		return 0
	}
	ma := semverTagPattern.FindStringSubmatch(na)
	mb := semverTagPattern.FindStringSubmatch(nb)
	if ma == nil || mb == nil {
		if na < nb {
			return -1
		}
		return 1
	}
	for i := 1; i <= 3; i++ {
		va, _ := strconv.Atoi(ma[i])
		vb, _ := strconv.Atoi(mb[i])
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	// Release without prerelease suffix > prerelease suffix (e.g. v1.0.0 > v1.0.0-rc1)
	if ma[4] == "" && mb[4] != "" {
		return 1
	}
	if ma[4] != "" && mb[4] == "" {
		return -1
	}
	if ma[4] < mb[4] {
		return -1
	}
	return 1
}

func releaseArchiveName(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "darwin/arm64", "darwin/amd64", "linux/amd64", "linux/arm64":
		return fmt.Sprintf("u2b-%s-%s.tar.gz", goos, goarch), nil
	case "windows/amd64", "windows/arm64":
		return fmt.Sprintf("u2b-%s-%s.zip", goos, goarch), nil
	default:
		return "", fmt.Errorf("no prebuilt binary available for %s/%s", goos, goarch)
	}
}

func addGitHubAuthHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "u2b-updater/"+normalizeSemverTag(Version))
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
}

func resolveReleaseMetadata(ctx context.Context, src releaseSourceConfig, requestedVersion string) (*resolvedReleaseMeta, error) {
	assetName, err := releaseArchiveName(src.TargetOS, src.TargetArch)
	if err != nil {
		return nil, err
	}

	reqVer := strings.TrimSpace(requestedVersion)
	if reqVer == "" {
		reqVer = "latest"
	}

	var apiURL string
	if reqVer == "latest" {
		apiURL = fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(src.APIBase, "/"), src.Repo)
	} else {
		reqVer = normalizeSemverTag(reqVer)
		if err := validateReleaseTag(reqVer); err != nil {
			return nil, err
		}
		apiURL = fmt.Sprintf("%s/repos/%s/releases/tags/%s", strings.TrimRight(src.APIBase, "/"), src.Repo, reqVer)
	}

	var resolvedTag, apiSHA string
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err == nil {
		addGitHubAuthHeaders(req)
		req.Header.Set("Accept", "application/vnd.github+json")
		if resp, err := src.Client.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var rel githubReleaseResponse
				if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&rel); err == nil {
					resolvedTag = strings.TrimSpace(rel.TagName)
					for _, a := range rel.Assets {
						if a.Name == assetName {
							d := strings.ToLower(strings.TrimSpace(a.Digest))
							if strings.HasPrefix(d, "sha256:") {
								cand := strings.TrimPrefix(d, "sha256:")
								if hexSHA256Pattern.MatchString(cand) {
									apiSHA = cand
								}
							}
							break
						}
					}
				}
			}
		}
	}

	// Fallback if GitHub REST API rate-limit (60/hr unauthenticated) was hit
	if resolvedTag == "" {
		if reqVer == "latest" {
			latestPage := fmt.Sprintf("%s/%s/releases/latest", strings.TrimRight(src.WebBase, "/"), src.Repo)
			headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, latestPage, nil)
			if err == nil {
				addGitHubAuthHeaders(headReq)
				if resp, err := src.Client.Do(headReq); err == nil {
					finalURL := resp.Request.URL.String()
					resp.Body.Close()
					if idx := strings.Index(finalURL, "/releases/tag/"); idx != -1 {
						tagPart := finalURL[idx+len("/releases/tag/"):]
						if q := strings.IndexAny(tagPart, "?#/"); q != -1 {
							tagPart = tagPart[:q]
						}
						resolvedTag = strings.TrimSpace(tagPart)
					}
				}
			}
		} else {
			resolvedTag = reqVer
		}
	}

	if resolvedTag == "" {
		return nil, fmt.Errorf("failed to query latest release from GitHub (%s); check network or configure --proxy", src.Repo)
	}
	if err := validateReleaseTag(resolvedTag); err != nil {
		return nil, fmt.Errorf("security check failed on resolved tag: %w", err)
	}

	webBase := strings.TrimRight(src.WebBase, "/")
	if !strings.HasPrefix(webBase, "https://") {
		return nil, fmt.Errorf("security check failed: refusing non-HTTPS download origin %q", webBase)
	}

	dlPrefix := fmt.Sprintf("%s/%s/releases/download/%s/", webBase, src.Repo, resolvedTag)
	return &resolvedReleaseMeta{
		Tag:        resolvedTag,
		AssetName:  assetName,
		ArchiveURL: dlPrefix + assetName,
		SumsURL:    dlPrefix + "checksums.txt",
		APISHA256:  apiSHA,
	}, nil
}

func downloadToFileWithSHA256(ctx context.Context, client *http.Client, expectedPrefix, rawURL, destPath string, maxBytes int64) (string, error) {
	if !strings.HasPrefix(rawURL, expectedPrefix) || !strings.HasPrefix(rawURL, "https://") {
		return "", fmt.Errorf("SECURITY ALERT: refusing download from untrusted URL %q", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	addGitHubAuthHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d while downloading %s", resp.StatusCode, rawURL)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer out.Close()

	hasher := sha256.New()
	limited := io.LimitReader(resp.Body, maxBytes+1)
	written, err := io.Copy(io.MultiWriter(out, hasher), limited)
	if err != nil {
		return "", err
	}
	if written == 0 {
		return "", fmt.Errorf("downloaded empty file from %s", rawURL)
	}
	if written > maxBytes {
		return "", fmt.Errorf("downloaded file exceeded maximum safe size (%d bytes)", maxBytes)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func parseExpectedSHA256FromChecksums(checksumsContent, assetName string) (string, error) {
	lines := strings.Split(checksumsContent, "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(fields[0])
		name := strings.TrimPrefix(fields[1], "*")
		if name == assetName {
			if !hexSHA256Pattern.MatchString(hash) {
				return "", fmt.Errorf("malformed SHA-256 digest %q in checksums.txt for %s", fields[0], assetName)
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("entry for %q not found in checksums.txt", assetName)
}

func isSafeArchiveEntryPath(entryName string) bool {
	if entryName == "" {
		return false
	}
	if strings.HasPrefix(entryName, "/") || strings.HasPrefix(entryName, `\`) {
		return false
	}
	if filepath.VolumeName(entryName) != "" {
		return false
	}
	normalized := strings.ReplaceAll(entryName, `\`, "/")
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func extractBinaryFromTarGz(archivePath, expectedBinName, destBinPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	found := false
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("corrupted tar archive: %w", err)
		}
		if !isSafeArchiveEntryPath(hdr.Name) {
			return fmt.Errorf("SECURITY ALERT: unsafe path traversal entry in tar archive: %q", hdr.Name)
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			return fmt.Errorf("SECURITY ALERT: unexpected link entry in release archive: %q", hdr.Name)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == expectedBinName {
			out, err := os.OpenFile(destBinPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, maxExtractedBinaryBytes+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if n == 0 || n > maxExtractedBinaryBytes {
				return fmt.Errorf("extracted binary has invalid size (%d bytes)", n)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("binary %q not found inside archive", expectedBinName)
	}
	return nil
}

func extractBinaryFromZip(archivePath, expectedBinName, destBinPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("invalid zip archive: %w", err)
	}
	defer zr.Close()

	found := false
	for _, entry := range zr.File {
		if !isSafeArchiveEntryPath(entry.Name) {
			return fmt.Errorf("SECURITY ALERT: unsafe path traversal entry in zip archive: %q", entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("SECURITY ALERT: unexpected symlink entry in zip archive: %q", entry.Name)
		}
		if !entry.FileInfo().IsDir() && filepath.Base(strings.ReplaceAll(entry.Name, `\`, "/")) == expectedBinName {
			rc, err := entry.Open()
			if err != nil {
				return err
			}
			out, err := os.OpenFile(destBinPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				rc.Close()
				return err
			}
			n, copyErr := io.Copy(out, io.LimitReader(rc, maxExtractedBinaryBytes+1))
			rc.Close()
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if n == 0 || n > maxExtractedBinaryBytes {
				return fmt.Errorf("extracted binary has invalid size (%d bytes)", n)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("binary %q not found inside zip archive", expectedBinName)
	}
	return nil
}

func verifyGitHubAttestationIfAvailable(archivePath, repo string, requireAttestation bool) error {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		if requireAttestation {
			return fmt.Errorf("--require-attestation set, but GitHub CLI ('gh') is not installed")
		}
		return nil
	}

	if err := exec.Command(ghPath, "attestation", "--help").Run(); err != nil {
		if requireAttestation {
			return fmt.Errorf("--require-attestation set, but installed 'gh' CLI does not support 'gh attestation'")
		}
		return nil
	}

	if err := exec.Command(ghPath, "auth", "status").Run(); err != nil {
		if requireAttestation {
			return fmt.Errorf("--require-attestation set, but 'gh' CLI is not logged in (run 'gh auth login')")
		}
		return nil
	}

	out, err := exec.Command(ghPath, "attestation", "verify", archivePath, "--repo", repo).CombinedOutput()
	if err == nil {
		fmt.Printf("✓ SLSA 构建溯源证明已通过 GitHub Attestations 验证 (%s)\n", repo)
		return nil
	}

	msg := strings.TrimSpace(string(out))
	lower := strings.ToLower(msg)
	if !requireAttestation && (strings.Contains(lower, "no attestations") || strings.Contains(lower, "not found") || strings.Contains(lower, "404")) {
		fmt.Printf("⚠️  该历史版本未包含 SLSA 证明包（已完成 SHA-256 校验）\n")
		return nil
	}
	return fmt.Errorf("SECURITY ALERT: GitHub SLSA attestation verification failed: %s", msg)
}

func replaceExecutableAtomically(extractedBinPath, targetExePath string) error {
	mode := os.FileMode(0755)
	if fi, err := os.Stat(targetExePath); err == nil {
		mode = fi.Mode().Perm() | 0111
	}

	targetDir := filepath.Dir(targetExePath)
	stagedPath := filepath.Join(targetDir, fmt.Sprintf(".%s.update.%d", filepath.Base(targetExePath), os.Getpid()))
	defer func() { _ = os.Remove(stagedPath) }()

	srcFile, err := os.Open(extractedBinPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		if errors.Is(err, os.ErrPermission) && runtime.GOOS != "windows" {
			return fmt.Errorf("没有写入目录 %s 的权限，请尝试使用 `sudo u2b update`", targetDir)
		}
		return err
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		return err
	}
	if err := dstFile.Sync(); err != nil {
		dstFile.Close()
		return err
	}
	if err := dstFile.Close(); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		backupPath := targetExePath + ".old"
		_ = os.Remove(backupPath)
		if _, err := os.Stat(targetExePath); err == nil {
			if err := os.Rename(targetExePath, backupPath); err != nil {
				return fmt.Errorf("failed to move running Windows binary aside: %w", err)
			}
		}
		if err := os.Rename(stagedPath, targetExePath); err != nil {
			// Roll back if rename failed
			_ = os.Rename(backupPath, targetExePath)
			return fmt.Errorf("failed to install updated binary: %w", err)
		}
		_ = os.Remove(backupPath)
		return nil
	}

	if err := os.Rename(stagedPath, targetExePath); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("没有替换 %s 的权限，请尝试使用 `sudo u2b update`", targetExePath)
		}
		return err
	}
	return nil
}

func performVerifiedUpdate(ctx context.Context, src releaseSourceConfig, targetExePath, requestedVersion string, preResolved *resolvedReleaseMeta, skipVerify, requireAttestation, runSmokeCheck bool) (*resolvedReleaseMeta, error) {
	meta := preResolved
	if meta == nil {
		var err error
		meta, err = resolveReleaseMetadata(ctx, src, requestedVersion)
		if err != nil {
			return nil, err
		}
	}

	tmpDir, err := os.MkdirTemp("", "u2b_update_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	expectedPrefix := fmt.Sprintf("%s/%s/releases/download/%s/", strings.TrimRight(src.WebBase, "/"), src.Repo, meta.Tag)
	archivePath := filepath.Join(tmpDir, meta.AssetName)

	fmt.Printf("==> 正在下载最新版本 %s (%s/%s)...\n", meta.Tag, src.TargetOS, src.TargetArch)
	actualSHA, err := downloadToFileWithSHA256(ctx, src.Client, expectedPrefix, meta.ArchiveURL, archivePath, maxReleaseArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("下载更新包失败: %w", err)
	}

	if skipVerify {
		fmt.Fprintln(os.Stderr, "Warning: 已跳过安全校验 (--skip-verify)！")
	} else {
		fmt.Println("==> 正在执行多层安全校验...")
		sumsPath := filepath.Join(tmpDir, "checksums.txt")
		if _, err := downloadToFileWithSHA256(ctx, src.Client, expectedPrefix, meta.SumsURL, sumsPath, 1024*1024); err != nil {
			return nil, fmt.Errorf("安全校验失败: 无法下载官方 checksums.txt: %w", err)
		}
		sumsBytes, err := os.ReadFile(sumsPath)
		if err != nil {
			return nil, err
		}
		expectedSHA, err := parseExpectedSHA256FromChecksums(string(sumsBytes), meta.AssetName)
		if err != nil {
			return nil, fmt.Errorf("安全校验失败: %w", err)
		}

		if meta.APISHA256 != "" && subtle.ConstantTimeCompare([]byte(meta.APISHA256), []byte(expectedSHA)) != 1 {
			return nil, fmt.Errorf("SECURITY ALERT: GitHub API 摘要 (%s) 与 checksums.txt (%s) 不一致，已中止更新", meta.APISHA256, expectedSHA)
		}

		if subtle.ConstantTimeCompare([]byte(actualSHA), []byte(expectedSHA)) != 1 {
			return nil, fmt.Errorf("SECURITY ALERT: SHA-256 哈希校验失败！期望 %s，实际 %s，已中止更新", expectedSHA, actualSHA)
		}
		fmt.Printf("✓ SHA-256 完整性校验通过: %s\n", actualSHA)

		if src.WebBase == defaultGitHubWebBase {
			if err := verifyGitHubAttestationIfAvailable(archivePath, src.Repo, requireAttestation); err != nil {
				return nil, err
			}
		}
	}

	binName := "u2b"
	if src.TargetOS == "windows" {
		binName = "u2b.exe"
	}
	extractedBin := filepath.Join(tmpDir, binName)

	if strings.HasSuffix(meta.AssetName, ".zip") {
		if err := extractBinaryFromZip(archivePath, binName, extractedBin); err != nil {
			return nil, err
		}
	} else {
		if err := extractBinaryFromTarGz(archivePath, binName, extractedBin); err != nil {
			return nil, err
		}
	}

	if src.TargetOS == "darwin" && runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-d", "com.apple.quarantine", extractedBin).Run()
		if !skipVerify {
			if codesignPath, err := exec.LookPath("codesign"); err == nil {
				if out, err := exec.Command(codesignPath, "--verify", extractedBin).CombinedOutput(); err != nil {
					return nil, fmt.Errorf("SECURITY ALERT: macOS Mach-O 代码签名校验失败: %w (%s)", err, strings.TrimSpace(string(out)))
				}
				fmt.Println("✓ macOS Mach-O 代码签名校验通过")
			}
		}
	}

	if runSmokeCheck && src.TargetOS == runtime.GOOS {
		smokeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		out, err := exec.CommandContext(smokeCtx, extractedBin, "version").CombinedOutput()
		cancel()
		if err != nil || !bytes.Contains(out, []byte("u2b version")) {
			return nil, fmt.Errorf("新版本二进制运行自检失败 (%v): %s", err, strings.TrimSpace(string(out)))
		}
	}

	if err := replaceExecutableAtomically(extractedBin, targetExePath); err != nil {
		return nil, err
	}

	return meta, nil
}

func postUpdateSyncServices(newExePath string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// 1. Quietly sync browser extension assets if previously installed
	extManifest := filepath.Join(homeDir, "u2b", "extension", "manifest.json")
	if _, err := os.Stat(extManifest); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := exec.CommandContext(ctx, newExePath, "ext", "install", "--no-open").Run()
		cancel()
		if err == nil {
			fmt.Println("✓ 已同步刷新本地浏览器扩展资源")
		}
	}

	// 2. Smoothly restart background `u2b serve` daemon if currently active
	switch runtime.GOOS {
	case "darwin":
		plistPath := macLaunchAgentPath(homeDir)
		if _, err := os.Stat(plistPath); err == nil {
			if exec.Command("launchctl", "list", launchAgentLabel).Run() == nil {
				_ = exec.Command("launchctl", "unload", plistPath).Run()
				if exec.Command("launchctl", "load", "-w", plistPath).Run() == nil {
					fmt.Println("✓ 已自动平滑重启 macOS 后台 u2b serve 服务")
				}
			}
		}
	case "linux":
		if exec.Command("systemctl", "--user", "is-active", "--quiet", "u2b.service").Run() == nil {
			if exec.Command("systemctl", "--user", "restart", "u2b.service").Run() == nil {
				fmt.Println("✓ 已自动平滑重启 Linux 后台 u2b serve 服务")
			}
		}
	}
}

func upgradeExternalDependencies() {
	fmt.Println("==> 正在检查并更新 yt-dlp...")
	switch runtime.GOOS {
	case "darwin":
		if brewPath, err := exec.LookPath("brew"); err == nil {
			cmd := exec.Command(brewPath, "upgrade", "yt-dlp")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			return
		}
	case "windows":
		if wingetPath, err := exec.LookPath("winget"); err == nil {
			cmd := exec.Command(wingetPath, "upgrade", "--id", "yt-dlp.yt-dlp", "--exact", "--silent", "--accept-package-agreements", "--accept-source-agreements")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
			return
		}
	}

	if ytdlpPath, err := exec.LookPath("yt-dlp"); err == nil {
		cmd := exec.Command(ytdlpPath, "-U")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	src := defaultReleaseSourceConfig()
	currentTag := normalizeSemverTag(Version)
	fmt.Printf("==> 当前版本: %s (commit: %s, %s/%s)\n", currentTag, GitCommit, runtime.GOOS, runtime.GOARCH)

	meta, err := resolveReleaseMetadata(ctx, src, updateTargetVersion)
	if err != nil {
		return err
	}

	isDevBuild := GitCommit == "dev" || GitCommit == "none" || GitCommit == ""
	alreadySame := !isDevBuild && currentTag == meta.Tag && src.TargetArch == runtime.GOARCH

	if updateCheckOnly {
		if alreadySame || (!isDevBuild && updateTargetVersion == "latest" && compareSemver(currentTag, meta.Tag) >= 0) {
			fmt.Printf("✓ 当前已是最新版本 (%s)\n", currentTag)
		} else {
			fmt.Printf("🎉 发现新版本: %s -> %s（运行 `u2b update` 即可一键升级）\n", currentTag, meta.Tag)
		}
		return nil
	}

	if alreadySame && !updateForce {
		fmt.Printf("✓ 当前已是目标版本 %s，无需重复下载（如需强制重装可添加 --force）\n", meta.Tag)
		if updateDeps {
			upgradeExternalDependencies()
		}
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位当前 u2b 可执行文件路径: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}

	updatedMeta, err := performVerifiedUpdate(ctx, src, exePath, meta.Tag, meta, updateSkipVerify, updateRequireAttestation, true)
	if err != nil {
		return err
	}

	postUpdateSyncServices(exePath)

	if updateDeps {
		upgradeExternalDependencies()
	}

	fmt.Printf("\n🚀 更新完成！已升级至 %s (%s)\n", updatedMeta.Tag, exePath)
	return nil
}

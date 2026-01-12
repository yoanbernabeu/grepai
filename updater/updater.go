package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubAPI      = "https://api.github.com/repos/yoanbernabeu/grepai/releases/latest"
	defaultTimeout = 60 * time.Second
)

// ReleaseInfo contains GitHub release metadata
type ReleaseInfo struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

// Asset represents a release asset
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Updater handles version checking and binary updates
type Updater struct {
	client         *http.Client
	currentVersion string
	apiURL         string
}

// NewUpdater creates a new updater instance
func NewUpdater(currentVersion string) *Updater {
	return &Updater{
		client: &http.Client{
			Timeout: defaultTimeout,
		},
		currentVersion: currentVersion,
		apiURL:         githubAPI,
	}
}

// CheckResult contains the result of a version check
type CheckResult struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	ReleaseURL      string
	PublishedAt     string
}

// CheckForUpdate fetches latest release info and compares versions
func (u *Updater) CheckForUpdate(ctx context.Context) (*CheckResult, error) {
	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(u.currentVersion, "v")

	return &CheckResult{
		CurrentVersion:  u.currentVersion,
		LatestVersion:   release.TagName,
		UpdateAvailable: latestVersion != currentVersion && currentVersion != "dev",
		ReleaseURL:      fmt.Sprintf("https://github.com/yoanbernabeu/grepai/releases/tag/%s", release.TagName),
		PublishedAt:     release.PublishedAt,
	}, nil
}

// Update downloads and installs the latest version
func (u *Updater) Update(ctx context.Context, progressFn func(downloaded, total int64)) error {
	// 1. Fetch release info
	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch release info: %w", err)
	}

	// 2. Find matching asset for current platform
	asset, checksumAsset := u.findAssets(release)
	if asset == nil {
		return fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// 3. Download to temp file
	tempDir, err := os.MkdirTemp("", "grepai-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, asset.Name)
	if err := u.downloadFile(ctx, asset.BrowserDownloadURL, archivePath, asset.Size, progressFn); err != nil {
		return fmt.Errorf("failed to download release: %w", err)
	}

	// 4. Verify checksum if available
	if checksumAsset != nil {
		checksumPath := filepath.Join(tempDir, checksumAsset.Name)
		if err := u.downloadFile(ctx, checksumAsset.BrowserDownloadURL, checksumPath, checksumAsset.Size, nil); err != nil {
			return fmt.Errorf("failed to download checksums: %w", err)
		}
		if err := u.verifyChecksum(archivePath, checksumPath, asset.Name); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// 5. Extract binary
	binaryPath, err := u.extractBinary(archivePath, tempDir)
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	// 6. Replace current binary
	if err := u.replaceBinary(binaryPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}

func (u *Updater) fetchLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "grepai-updater")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub API rate limit exceeded, try again later")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found for this repository")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release info: %w", err)
	}

	return &release, nil
}

func (u *Updater) findAssets(release *ReleaseInfo) (*Asset, *Asset) {
	// Build expected archive name based on .goreleaser.yml pattern:
	// grepai_{VERSION}_{OS}_{ARCH}.tar.gz (or .zip for Windows)
	version := strings.TrimPrefix(release.TagName, "v")
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	expectedName := fmt.Sprintf("grepai_%s_%s_%s%s", version, runtime.GOOS, runtime.GOARCH, ext)

	var asset, checksumAsset *Asset
	for i := range release.Assets {
		a := &release.Assets[i]
		if a.Name == expectedName {
			asset = a
		}
		if a.Name == "checksums.txt" {
			checksumAsset = a
		}
	}

	return asset, checksumAsset
}

func (u *Updater) downloadFile(ctx context.Context, url, destPath string, totalSize int64, progressFn func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if progressFn != nil {
		// Wrap reader with progress tracking
		reader := &progressReader{reader: resp.Body, total: totalSize, progressFn: progressFn}
		_, err = io.Copy(out, reader)
	} else {
		_, err = io.Copy(out, resp.Body)
	}

	return err
}

type progressReader struct {
	reader     io.Reader
	downloaded int64
	total      int64
	progressFn func(downloaded, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.progressFn != nil {
		pr.progressFn(pr.downloaded, pr.total)
	}
	return n, err
}

func (u *Updater) verifyChecksum(archivePath, checksumPath, assetName string) error {
	// Read checksums file
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}

	// Parse checksums.txt (format: "checksum  filename")
	var expectedChecksum string
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			expectedChecksum = parts[0]
			break
		}
	}

	if expectedChecksum == "" {
		return fmt.Errorf("checksum for %s not found in checksums.txt", assetName)
	}

	// Calculate actual checksum
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualChecksum := hex.EncodeToString(h.Sum(nil))

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

func (u *Updater) extractBinary(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return u.extractZip(archivePath, destDir)
	}
	return u.extractTarGz(archivePath, destDir)
}

func (u *Updater) extractTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	binaryName := "grepai"

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if hdr.Name == binaryName || filepath.Base(hdr.Name) == binaryName {
			destPath := filepath.Join(destDir, binaryName)
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0755) // #nosec G302 - executable needs 0755
			if err != nil {
				return "", err
			}
			// #nosec G110 - trusted source (GitHub releases)
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			return destPath, nil
		}
	}

	return "", fmt.Errorf("binary %s not found in archive", binaryName)
}

func (u *Updater) extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	binaryName := "grepai.exe"

	for _, f := range r.File {
		if f.Name == binaryName || filepath.Base(f.Name) == binaryName {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}

			destPath := filepath.Join(destDir, binaryName)
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0755) // #nosec G302 - executable needs 0755
			if err != nil {
				rc.Close()
				return "", err
			}

			// #nosec G110 - trusted source (GitHub releases)
			_, err = io.Copy(out, rc)
			rc.Close()
			out.Close()
			if err != nil {
				return "", err
			}
			return destPath, nil
		}
	}

	return "", fmt.Errorf("binary %s not found in archive", binaryName)
}

func (u *Updater) replaceBinary(newBinaryPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Check write permissions
	if err := checkWritePermission(execPath); err != nil {
		return err
	}

	// Platform-specific replacement
	if runtime.GOOS == "windows" {
		return u.replaceWindowsBinary(execPath, newBinaryPath)
	}
	return u.replaceUnixBinary(execPath, newBinaryPath)
}

func (u *Updater) replaceUnixBinary(execPath, newBinaryPath string) error {
	// On Unix, we can rename over the running binary
	backupPath := execPath + ".old"

	// Remove old backup if exists
	os.Remove(backupPath)

	// Rename current to backup
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary to target location
	if err := os.Rename(newBinaryPath, execPath); err != nil {
		// Try to restore backup (best effort, ignore error)
		_ = os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Set executable permissions
	if err := os.Chmod(execPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	return nil
}

func (u *Updater) replaceWindowsBinary(execPath, newBinaryPath string) error {
	// On Windows, we cannot replace a running binary directly
	// We rename current to .old, copy new one, and old will be deleted on next run
	backupPath := execPath + ".old"

	// Remove old backup if exists
	os.Remove(backupPath)

	// Rename current to backup
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Copy new binary to target location (move might fail across drives)
	src, err := os.Open(newBinaryPath)
	if err != nil {
		_ = os.Rename(backupPath, execPath) // Best effort restore
		return fmt.Errorf("failed to open new binary: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(execPath, os.O_CREATE|os.O_WRONLY, 0755) // #nosec G302 - executable needs 0755
	if err != nil {
		_ = os.Rename(backupPath, execPath) // Best effort restore
		return fmt.Errorf("failed to create new binary: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(execPath)
		_ = os.Rename(backupPath, execPath) // Best effort restore
		return fmt.Errorf("failed to copy new binary: %w", err)
	}

	return nil
}

func checkWritePermission(path string) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("cannot access directory %s: %w", dir, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	// Try to create a temp file to verify write permissions
	testFile := filepath.Join(dir, ".grepai-update-test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("no write permission in %s: %w\nTry running with elevated privileges (sudo)", dir, err)
	}
	f.Close()
	os.Remove(testFile)

	return nil
}

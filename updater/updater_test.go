package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewUpdater(t *testing.T) {
	u := NewUpdater("1.0.0")
	if u.currentVersion != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", u.currentVersion)
	}
	if u.client == nil {
		t.Error("expected non-nil HTTP client")
	}
}

func TestCheckForUpdate_NewVersionAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName:     "v2.0.0",
			PublishedAt: "2026-01-01T00:00:00Z",
		}
		if err := json.NewEncoder(w).Encode(release); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	u := &Updater{
		client:         server.Client(),
		currentVersion: "1.0.0",
		apiURL:         server.URL,
	}

	ctx := context.Background()
	result, err := u.CheckForUpdate(ctx)
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}

	if !result.UpdateAvailable {
		t.Error("expected update to be available")
	}
	if result.CurrentVersion != "1.0.0" {
		t.Errorf("expected current version 1.0.0, got %s", result.CurrentVersion)
	}
	if result.LatestVersion != "v2.0.0" {
		t.Errorf("expected latest version v2.0.0, got %s", result.LatestVersion)
	}
}

func TestCheckForUpdate_AlreadyLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName:     "v1.0.0",
			PublishedAt: "2026-01-01T00:00:00Z",
		}
		if err := json.NewEncoder(w).Encode(release); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	u := &Updater{
		client:         server.Client(),
		currentVersion: "1.0.0",
		apiURL:         server.URL,
	}

	ctx := context.Background()
	result, err := u.CheckForUpdate(ctx)
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}

	if result.UpdateAvailable {
		t.Error("should not show update available when versions match")
	}
}

func TestCheckForUpdate_DevVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		release := ReleaseInfo{
			TagName:     "v1.0.0",
			PublishedAt: "2026-01-01T00:00:00Z",
		}
		if err := json.NewEncoder(w).Encode(release); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	u := &Updater{
		client:         server.Client(),
		currentVersion: "dev",
		apiURL:         server.URL,
	}

	ctx := context.Background()
	result, err := u.CheckForUpdate(ctx)
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}

	if result.UpdateAvailable {
		t.Error("dev version should not show update available")
	}
}

func TestCheckForUpdate_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	u := &Updater{
		client:         server.Client(),
		currentVersion: "1.0.0",
		apiURL:         server.URL,
	}

	ctx := context.Background()
	_, err := u.CheckForUpdate(ctx)
	if err == nil {
		t.Error("expected error for rate limit")
	}
	if err.Error() != "GitHub API rate limit exceeded, try again later" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCheckForUpdate_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	u := &Updater{
		client:         server.Client(),
		currentVersion: "1.0.0",
		apiURL:         server.URL,
	}

	ctx := context.Background()
	_, err := u.CheckForUpdate(ctx)
	if err == nil {
		t.Error("expected error for not found")
	}
	if err.Error() != "no releases found for this repository" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFindAssets(t *testing.T) {
	u := NewUpdater("1.0.0")

	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	expectedName := "grepai_1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + ext

	release := &ReleaseInfo{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: expectedName, BrowserDownloadURL: "https://example.com/asset"},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums"},
		},
	}

	asset, checksumAsset := u.findAssets(release)

	if asset == nil {
		t.Fatal("expected to find asset")
	}
	if asset.Name != expectedName {
		t.Errorf("expected asset name %s, got %s", expectedName, asset.Name)
	}
	if checksumAsset == nil {
		t.Fatal("expected to find checksum asset")
	}
	if checksumAsset.Name != "checksums.txt" {
		t.Errorf("expected checksum asset name checksums.txt, got %s", checksumAsset.Name)
	}
}

func TestFindAssets_NoMatchingAsset(t *testing.T) {
	u := NewUpdater("1.0.0")

	release := &ReleaseInfo{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "grepai_1.0.0_other_arch.tar.gz", BrowserDownloadURL: "https://example.com/asset"},
		},
	}

	asset, _ := u.findAssets(release)

	if asset != nil {
		t.Error("expected no matching asset")
	}
}

func TestVerifyChecksum(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tempDir, "test.tar.gz")
	testContent := []byte("test content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatal(err)
	}

	// SHA256 of "test content" = 6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72
	checksumFile := filepath.Join(tempDir, "checksums.txt")
	checksumContent := "6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72  test.tar.gz\n"
	if err := os.WriteFile(checksumFile, []byte(checksumContent), 0644); err != nil {
		t.Fatal(err)
	}

	u := NewUpdater("1.0.0")
	err := u.verifyChecksum(testFile, checksumFile, "test.tar.gz")
	if err != nil {
		t.Errorf("checksum verification should pass: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.tar.gz")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	checksumFile := filepath.Join(tempDir, "checksums.txt")
	checksumContent := "0000000000000000000000000000000000000000000000000000000000000000  test.tar.gz\n"
	if err := os.WriteFile(checksumFile, []byte(checksumContent), 0644); err != nil {
		t.Fatal(err)
	}

	u := NewUpdater("1.0.0")
	err := u.verifyChecksum(testFile, checksumFile, "test.tar.gz")
	if err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestVerifyChecksum_MissingEntry(t *testing.T) {
	tempDir := t.TempDir()

	testFile := filepath.Join(tempDir, "test.tar.gz")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	checksumFile := filepath.Join(tempDir, "checksums.txt")
	checksumContent := "abc123  other_file.tar.gz\n"
	if err := os.WriteFile(checksumFile, []byte(checksumContent), 0644); err != nil {
		t.Fatal(err)
	}

	u := NewUpdater("1.0.0")
	err := u.verifyChecksum(testFile, checksumFile, "test.tar.gz")
	if err == nil {
		t.Error("expected missing checksum entry error")
	}
}

func TestCheckWritePermission(t *testing.T) {
	tempDir := t.TempDir()

	err := checkWritePermission(filepath.Join(tempDir, "grepai"))
	if err != nil {
		t.Errorf("should have write permission in temp dir: %v", err)
	}
}

func TestCheckWritePermission_NonExistentDir(t *testing.T) {
	err := checkWritePermission("/nonexistent/path/grepai")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestProgressReader(t *testing.T) {
	content := []byte("test content for progress tracking")
	var lastDownloaded, lastTotal int64

	pr := &progressReader{
		reader: &mockReader{data: content},
		total:  int64(len(content)),
		progressFn: func(downloaded, total int64) {
			lastDownloaded = downloaded
			lastTotal = total
		},
	}

	buf := make([]byte, 10)
	for {
		n, err := pr.Read(buf)
		if n == 0 || err != nil {
			break
		}
	}

	if lastTotal != int64(len(content)) {
		t.Errorf("expected total %d, got %d", len(content), lastTotal)
	}
	if lastDownloaded != int64(len(content)) {
		t.Errorf("expected downloaded %d, got %d", len(content), lastDownloaded)
	}
}

type mockReader struct {
	data   []byte
	offset int
}

func (m *mockReader) Read(p []byte) (int, error) {
	if m.offset >= len(m.data) {
		return 0, nil
	}
	n := copy(p, m.data[m.offset:])
	m.offset += n
	return n, nil
}

func TestDownloadFile(t *testing.T) {
	content := []byte("downloaded content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	u := &Updater{
		client: server.Client(),
	}

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "downloaded.txt")

	ctx := context.Background()
	err := u.downloadFile(ctx, server.URL, destPath, int64(len(content)), nil)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(data) != string(content) {
		t.Errorf("expected content %q, got %q", string(content), string(data))
	}
}

func TestDownloadFile_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	u := &Updater{
		client: server.Client(),
	}

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "downloaded.txt")

	ctx := context.Background()
	err := u.downloadFile(ctx, server.URL, destPath, 0, nil)
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

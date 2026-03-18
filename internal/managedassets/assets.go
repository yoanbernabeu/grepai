package managedassets

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
	"slices"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/config"
)

const (
	DefaultModelID          = "bge-small-en-v1.5-q8_0"
	DefaultRuntimeVersion   = "b3426"
	DefaultSidecarPort      = 12434
	runtimeStateFileName    = "llamacpp-runtime.json"
	modelManifestFileName   = "models.json"
	runtimeDownloadTimeout  = 10 * time.Minute
	modelDownloadTimeout    = 30 * time.Minute
	defaultEmbeddingDimSize = 768
)

type ModelDefinition struct {
	ID          string `json:"id"`
	Display     string `json:"display"`
	SizeBytes   int64  `json:"size_bytes"`
	FileName    string `json:"file_name"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256,omitempty"`
	Dimensions  int    `json:"dimensions"`
	QueryPrefix string `json:"query_prefix,omitempty"`
	DocPrefix   string `json:"doc_prefix,omitempty"`
}

type RuntimeDefinition struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256,omitempty"`
	Archive  string `json:"archive"`
	Binary   string `json:"binary"`
}

type InstalledModel struct {
	ID         string    `json:"id"`
	FileName   string    `json:"file_name"`
	Path       string    `json:"path"`
	SourceURL  string    `json:"source_url"`
	Installed  time.Time `json:"installed_at"`
	SizeBytes  int64     `json:"size_bytes"`
	Dimensions int       `json:"dimensions"`
}

type RuntimeState struct {
	Version  string    `json:"version"`
	Platform string    `json:"platform"`
	Arch     string    `json:"arch"`
	Binary   string    `json:"binary"`
	Endpoint string    `json:"endpoint"`
	PID      int       `json:"pid"`
	Started  time.Time `json:"started_at"`
}

var defaultModels = map[string]ModelDefinition{
	DefaultModelID: {
		ID:         DefaultModelID,
		Display:    "BGE Small English v1.5 Q8_0",
		SizeBytes:  36685152,
		FileName:   "bge-small-en-v1.5-q8_0.gguf",
		URL:        "https://huggingface.co/ggml-org/bge-small-en-v1.5-Q8_0-GGUF/resolve/main/bge-small-en-v1.5-q8_0.gguf?download=1",
		Dimensions: 384,
	},
	"nomic-embed-text-v1.5-q8_0": {
		ID:          "nomic-embed-text-v1.5-q8_0",
		Display:     "Nomic Embed Text v1.5 Q8_0",
		SizeBytes:   153092096,
		FileName:    "nomic-embed-text-v1.5.Q8_0.gguf",
		URL:         "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q8_0.gguf?download=1",
		Dimensions:  768,
		QueryPrefix: "search_query: ",
		DocPrefix:   "search_document: ",
	},
	"nomic-embed-text-v1.5-q4_k_m": {
		ID:          "nomic-embed-text-v1.5-q4_k_m",
		Display:     "Nomic Embed Text v1.5 Q4_K_M",
		SizeBytes:   88185242,
		FileName:    "nomic-embed-text-v1.5.Q4_K_M.gguf",
		URL:         "https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/nomic-embed-text-v1.5.Q4_K_M.gguf?download=1",
		Dimensions:  768,
		QueryPrefix: "search_query: ",
		DocPrefix:   "search_document: ",
	},
}

var runtimeDefinitions = map[string]RuntimeDefinition{
	"darwin/arm64": {
		Version:  DefaultRuntimeVersion,
		Platform: "darwin",
		Arch:     "arm64",
		URL:      "https://github.com/ggml-org/llama.cpp/releases/download/b3426/llama-b3426-bin-macos-arm64.zip",
		Archive:  "zip",
		Binary:   "llama-server",
	},
	"darwin/amd64": {
		Version:  DefaultRuntimeVersion,
		Platform: "darwin",
		Arch:     "amd64",
		URL:      "https://github.com/ggml-org/llama.cpp/releases/download/b3426/llama-b3426-bin-macos-x64.zip",
		Archive:  "zip",
		Binary:   "llama-server",
	},
	"linux/amd64": {
		Version:  DefaultRuntimeVersion,
		Platform: "linux",
		Arch:     "amd64",
		URL:      "https://github.com/ggml-org/llama.cpp/releases/download/b3426/llama-b3426-bin-ubuntu-x64.zip",
		Archive:  "zip",
		Binary:   "llama-server",
	},
	"windows/amd64": {
		Version:  DefaultRuntimeVersion,
		Platform: "windows",
		Arch:     "amd64",
		URL:      "https://github.com/ggml-org/llama.cpp/releases/download/b3426/llama-b3426-bin-win-avx2-x64.zip",
		Archive:  "zip",
		Binary:   "llama-server.exe",
	},
}

func GetManagedBinDir() (string, error) {
	root, err := config.GetGlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "bin"), nil
}

func GetManagedModelsDir() (string, error) {
	root, err := config.GetGlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "models"), nil
}

func GetManagedStateDir() (string, error) {
	root, err := config.GetGlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state"), nil
}

func GetManagedRuntimeStatePath() (string, error) {
	dir, err := GetManagedStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, runtimeStateFileName), nil
}

func GetManagedModelManifestPath() (string, error) {
	dir, err := GetManagedModelsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, modelManifestFileName), nil
}

func DefaultSidecarEndpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d", DefaultSidecarPort)
}

func LookupModel(id string) (ModelDefinition, error) {
	if id == "" {
		id = DefaultModelID
	}
	def, ok := defaultModels[id]
	if !ok {
		return ModelDefinition{}, fmt.Errorf("unknown managed model: %s", id)
	}
	return def, nil
}

func ListAvailableModels() []ModelDefinition {
	models := make([]ModelDefinition, 0, len(defaultModels))
	for _, model := range defaultModels {
		models = append(models, model)
	}
	slices.SortFunc(models, func(a, b ModelDefinition) int {
		return strings.Compare(a.ID, b.ID)
	})
	return models
}

func LookupRuntime(goos, goarch string) (RuntimeDefinition, error) {
	key := goos + "/" + goarch
	def, ok := runtimeDefinitions[key]
	if !ok {
		return RuntimeDefinition{}, fmt.Errorf("managed llama.cpp runtime is not available for %s/%s", goos, goarch)
	}
	return def, nil
}

func LookupCurrentRuntime() (RuntimeDefinition, error) {
	return LookupRuntime(runtime.GOOS, runtime.GOARCH)
}

func EnsureManagedDirs() error {
	for _, fn := range []func() (string, error){GetManagedBinDir, GetManagedModelsDir, GetManagedStateDir} {
		dir, err := fn()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create managed assets directory %s: %w", dir, err)
		}
	}
	return nil
}

func ManagedModelPath(def ModelDefinition) (string, error) {
	dir, err := GetManagedModelsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, def.FileName), nil
}

func ManagedRuntimeBinaryPath(def RuntimeDefinition) (string, error) {
	dir, err := GetManagedBinDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, def.Binary), nil
}

func LoadInstalledModels() ([]InstalledModel, error) {
	manifestPath, err := GetManagedModelManifestPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read model manifest: %w", err)
	}
	var models []InstalledModel
	if err := json.Unmarshal(data, &models); err != nil {
		return nil, fmt.Errorf("failed to parse model manifest: %w", err)
	}
	for i := range models {
		if models[i].SizeBytes <= 0 {
			if st, err := os.Stat(models[i].Path); err == nil {
				models[i].SizeBytes = st.Size()
			} else if def, ok := defaultModels[models[i].ID]; ok && def.SizeBytes > 0 {
				models[i].SizeBytes = def.SizeBytes
			}
		}
		if models[i].Dimensions <= 0 {
			if def, ok := defaultModels[models[i].ID]; ok && def.Dimensions > 0 {
				models[i].Dimensions = def.Dimensions
			}
		}
	}
	return models, nil
}

func SaveInstalledModels(models []InstalledModel) error {
	if err := EnsureManagedDirs(); err != nil {
		return err
	}
	manifestPath, err := GetManagedModelManifestPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal model manifest: %w", err)
	}
	return os.WriteFile(manifestPath, data, 0o600)
}

func FindInstalledModel(id string) (*InstalledModel, error) {
	models, err := LoadInstalledModels()
	if err != nil {
		return nil, err
	}
	for i := range models {
		if models[i].ID == id {
			return &models[i], nil
		}
	}
	return nil, nil
}

func InstallModel(ctx context.Context, id string, progress func(downloaded, total int64)) (*InstalledModel, error) {
	def, err := LookupModel(id)
	if err != nil {
		return nil, err
	}
	if err := EnsureManagedDirs(); err != nil {
		return nil, err
	}
	modelPath, err := ManagedModelPath(def)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, modelDownloadTimeout)
	defer cancel()
	if err := downloadFile(ctx, def.URL, modelPath, def.SHA256, progress); err != nil {
		return nil, err
	}
	model := InstalledModel{
		ID:         def.ID,
		FileName:   def.FileName,
		Path:       modelPath,
		SourceURL:  def.URL,
		Installed:  time.Now().UTC(),
		SizeBytes:  def.SizeBytes,
		Dimensions: def.Dimensions,
	}
	models, err := LoadInstalledModels()
	if err != nil {
		return nil, err
	}
	replaced := false
	for i := range models {
		if models[i].ID == model.ID {
			models[i] = model
			replaced = true
			break
		}
	}
	if !replaced {
		models = append(models, model)
	}
	if err := SaveInstalledModels(models); err != nil {
		return nil, err
	}
	return &model, nil
}

func RemoveInstalledModel(id string) error {
	models, err := LoadInstalledModels()
	if err != nil {
		return err
	}
	filtered := models[:0]
	removed := false
	for _, m := range models {
		if m.ID == id {
			removed = true
			if m.Path != "" {
				_ = os.Remove(m.Path)
			}
			continue
		}
		filtered = append(filtered, m)
	}
	if !removed {
		return fmt.Errorf("managed model %q is not installed", id)
	}
	return SaveInstalledModels(filtered)
}

func EnsureRuntime(ctx context.Context, progress func(downloaded, total int64)) (string, RuntimeDefinition, error) {
	def, err := LookupCurrentRuntime()
	if err != nil {
		return "", RuntimeDefinition{}, err
	}
	if err := EnsureManagedDirs(); err != nil {
		return "", RuntimeDefinition{}, err
	}
	binPath, err := ManagedRuntimeBinaryPath(def)
	if err != nil {
		return "", RuntimeDefinition{}, err
	}
	if st, err := os.Stat(binPath); err == nil && st.Mode().IsRegular() {
		return binPath, def, nil
	}
	ctx, cancel := context.WithTimeout(ctx, runtimeDownloadTimeout)
	defer cancel()
	tmpDir, err := os.MkdirTemp("", "grepai-llamacpp-runtime-*")
	if err != nil {
		return "", RuntimeDefinition{}, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	archivePath := filepath.Join(tmpDir, filepath.Base(def.URL))
	if err := downloadFile(ctx, def.URL, archivePath, def.SHA256, progress); err != nil {
		return "", RuntimeDefinition{}, err
	}
	if err := extractArchive(archivePath, tmpDir, def.Archive); err != nil {
		return "", RuntimeDefinition{}, err
	}
	extracted, err := findFile(tmpDir, def.Binary)
	if err != nil {
		return "", RuntimeDefinition{}, err
	}
	if err := copyExecutable(extracted, binPath); err != nil {
		return "", RuntimeDefinition{}, err
	}
	return binPath, def, nil
}

func ResolveModelPath(id, override string) (string, int, error) {
	if strings.TrimSpace(override) != "" {
		return override, defaultEmbeddingDimSize, nil
	}
	if id == "" {
		id = DefaultModelID
	}
	installed, err := FindInstalledModel(id)
	if err != nil {
		return "", 0, err
	}
	if installed == nil {
		return "", 0, fmt.Errorf("managed model %q is not installed; run 'grepai model install %s'", id, id)
	}
	return installed.Path, installed.Dimensions, nil
}

func LoadRuntimeState() (*RuntimeState, error) {
	path, err := GetManagedRuntimeStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read runtime state: %w", err)
	}
	var state RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse runtime state: %w", err)
	}
	return &state, nil
}

func SaveRuntimeState(state RuntimeState) error {
	if err := EnsureManagedDirs(); err != nil {
		return err
	}
	path, err := GetManagedRuntimeStatePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal runtime state: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func ClearRuntimeState() error {
	path, err := GetManagedRuntimeStatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove runtime state: %w", err)
	}
	return nil
}

func downloadFile(ctx context.Context, url, dest, checksum string, progress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed for %s: status %d", url, resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer f.Close()

	var r io.Reader = resp.Body
	var written int64
	buf := make([]byte, 32*1024)
	hash := sha256.New()
	total := resp.ContentLength

	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if _, err := f.Write(chunk); err != nil {
				return fmt.Errorf("failed to write download: %w", err)
			}
			if _, err := hash.Write(chunk); err != nil {
				return fmt.Errorf("failed to hash download: %w", err)
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("failed while downloading %s: %w", url, readErr)
		}
	}

	if checksum != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if !strings.EqualFold(got, checksum) {
			return fmt.Errorf("checksum mismatch for %s", filepath.Base(dest))
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to finalize download: %w", err)
	}
	return os.Rename(tmp, dest)
}

func extractArchive(archivePath, destDir, kind string) error {
	switch kind {
	case "zip":
		return extractZip(archivePath, destDir)
	case "tar.gz":
		return extractTarGz(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive type: %s", kind)
	}
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open tar.gz archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()
	return untar(gz, destDir)
}

func untar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

func findFile(root, fileName string) (string, error) {
	var found string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) == fileName {
			found = path
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("required file %s not found in extracted archive", fileName)
	}
	return found, nil
}

func copyExecutable(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read executable: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return fmt.Errorf("failed to install executable: %w", err)
	}
	return nil
}

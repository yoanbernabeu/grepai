package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// IndexMetaFileName stores which embedder built a project's index.
const IndexMetaFileName = "index-meta.json"

// IndexMeta identifies the embedder whose vectors are stored in an index.
// Vectors from different models (or sizes) are not comparable, so an index
// built with one embedder must not be searched or extended with another.
type IndexMeta struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

// EmbedderIndexMeta returns the identity of the embedder described by cfg.
// The endpoint is left out on purpose: the same model served from another
// host produces the same vectors.
func EmbedderIndexMeta(cfg EmbedderConfig) IndexMeta {
	return IndexMeta{
		Provider:   cfg.Provider,
		Model:      cfg.Model,
		Dimensions: cfg.GetDimensions(),
	}
}

func (m IndexMeta) String() string {
	return fmt.Sprintf("%s/%s (%d dimensions)", m.Provider, m.Model, m.Dimensions)
}

// GetIndexMetaPath returns the index identity file of a project index.
func GetIndexMetaPath(projectRoot string) string {
	return filepath.Join(GetConfigDir(projectRoot), IndexMetaFileName)
}

// GetWorkspaceIndexMetaPath returns the index identity file of one project
// inside a workspace store.
func GetWorkspaceIndexMetaPath(workspaceName, projectName string) (string, error) {
	globalDir, err := GetGlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(globalDir, "index-meta", workspaceName, projectName+".json"), nil
}

// LoadIndexMeta reads an index identity file. It returns nil, nil when the
// file does not exist (indexes built before this file was introduced).
func LoadIndexMeta(path string) (*IndexMeta, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read index metadata: %w", err)
	}
	var meta IndexMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse index metadata %s: %w", path, err)
	}
	return &meta, nil
}

// SaveIndexMeta writes an index identity file atomically.
func SaveIndexMeta(path string, meta IndexMeta) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create index metadata directory: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("failed to write index metadata: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to write index metadata: %w", err)
	}
	return nil
}

// ErrEmbedderChanged reports that an index was built with another embedder.
type ErrEmbedderChanged struct {
	Index   IndexMeta
	Current IndexMeta
}

func (e *ErrEmbedderChanged) Error() string {
	return fmt.Sprintf("the index was built with %s but the config now uses %s; results would be wrong. Rebuild the index with `grepai index` (or restart `grepai watch`)", e.Index, e.Current)
}

// CheckIndexMeta returns an *ErrEmbedderChanged when the index at path was
// built with a different embedder than cfg. A missing file is not an error.
func CheckIndexMeta(path string, cfg EmbedderConfig) error {
	meta, err := LoadIndexMeta(path)
	if err != nil || meta == nil {
		return err
	}
	if current := EmbedderIndexMeta(cfg); *meta != current {
		return &ErrEmbedderChanged{Index: *meta, Current: current}
	}
	return nil
}

// CheckWorkspaceIndexMeta runs CheckIndexMeta for the selected projects of a
// workspace (all projects when none is selected), against the workspace embedder.
func CheckWorkspaceIndexMeta(ws *Workspace, selectedProjects []string) error {
	if ws == nil {
		return nil
	}
	selected := make(map[string]bool, len(selectedProjects))
	for _, name := range selectedProjects {
		selected[name] = true
	}
	for _, p := range ws.Projects {
		if len(selected) > 0 && !selected[p.Name] {
			continue
		}
		path, err := GetWorkspaceIndexMetaPath(ws.Name, p.Name)
		if err != nil {
			return err
		}
		if err := CheckIndexMeta(path, ws.Embedder); err != nil {
			return fmt.Errorf("project %s: %w", p.Name, err)
		}
	}
	return nil
}

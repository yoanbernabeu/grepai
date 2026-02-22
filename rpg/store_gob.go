package rpg

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/yoanbernabeu/grepai/internal/fileutil"
)

// GOBRPGStore implements RPGStore using GOB encoding.
type GOBRPGStore struct {
	indexPath string
	lockPath  string
	graph     *Graph
	mu        sync.RWMutex
}

type gobRPGData struct {
	Version int
	Nodes   map[string]*Node
	Edges   []*Edge
}

// NewGOBRPGStore creates a new GOB-based RPG store.
func NewGOBRPGStore(indexPath string) *GOBRPGStore {
	return &GOBRPGStore{
		indexPath: indexPath,
		lockPath:  indexPath + ".lock",
		graph:     NewGraph(),
	}
}

// Load reads the graph from persistent storage.
func (s *GOBRPGStore) Load(ctx context.Context) error {
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Printf("rpg: flock open failed, proceeding without lock: %v", err)
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.loadUnlocked()
	}
	defer lockFile.Close()
	if err := fileutil.FlockShared(lockFile, true); err != nil {
		log.Printf("rpg: flock shared acquire failed, proceeding without lock: %v", err)
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.loadUnlocked()
	}
	defer func() {
		_ = fileutil.Funlock(lockFile)
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

func (s *GOBRPGStore) loadUnlocked() error {
	file, err := os.Open(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No existing index, start fresh
		}
		return fmt.Errorf("failed to open rpg index: %w", err)
	}
	defer file.Close()

	var data gobRPGData
	if err := gob.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode rpg index: %w", err)
	}

	if data.Version != CurrentRPGIndexVersion {
		hasData := len(data.Nodes) > 0 || len(data.Edges) > 0
		s.graph.Nodes = make(map[string]*Node)
		s.graph.Edges = make([]*Edge, 0)
		s.graph.RebuildIndexes()
		if hasData {
			return ErrRPGIndexOutdated
		}
		return nil
	}

	s.graph.Nodes = data.Nodes
	s.graph.Edges = data.Edges

	if s.graph.Nodes == nil {
		s.graph.Nodes = make(map[string]*Node)
	}
	if s.graph.Edges == nil {
		s.graph.Edges = make([]*Edge, 0)
	}

	s.graph.RebuildIndexes()

	return nil
}

// Persist writes the graph to persistent storage.
func (s *GOBRPGStore) Persist(ctx context.Context) error {
	if err := fileutil.EnsureParentDir(s.indexPath); err != nil {
		return fmt.Errorf("failed to prepare rpg index directory: %w", err)
	}

	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		log.Printf("rpg: flock open failed for persist, proceeding without lock: %v", err)
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.persistUnlocked()
	}
	defer lockFile.Close()
	if err := fileutil.FlockExclusive(lockFile, true); err != nil {
		log.Printf("rpg: flock exclusive acquire failed, proceeding without lock: %v", err)
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.persistUnlocked()
	}
	defer func() {
		_ = fileutil.Funlock(lockFile)
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistUnlocked()
}

func (s *GOBRPGStore) persistUnlocked() error {
	data := gobRPGData{
		Version: CurrentRPGIndexVersion,
		Nodes:   s.graph.Nodes,
		Edges:   s.graph.Edges,
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(s.indexPath), filepath.Base(s.indexPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create rpg index temp file: %w", err)
	}

	tmpPath := tmpFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := gob.NewEncoder(tmpFile).Encode(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to encode rpg index: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync rpg index temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close rpg index temp file: %w", err)
	}
	if err := fileutil.ReplaceFileAtomically(tmpPath, s.indexPath); err != nil {
		return fmt.Errorf("failed to replace rpg index file: %w", err)
	}
	cleanupTemp = false

	return nil
}

// Close cleanly shuts down the store by persisting data.
func (s *GOBRPGStore) Close() error {
	return s.Persist(context.Background())
}

// GetGraph returns the in-memory graph.
func (s *GOBRPGStore) GetGraph() *Graph {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.graph
}

// GetStats returns graph statistics.
func (s *GOBRPGStore) GetStats(ctx context.Context) (*GraphStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.graph.Stats()
	return &stats, nil
}

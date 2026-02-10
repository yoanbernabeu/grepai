package rpg

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// GOBRPGStore implements RPGStore using GOB encoding.
type GOBRPGStore struct {
	indexPath string
	graph     *Graph
	mu        sync.RWMutex
}

type gobRPGData struct {
	Nodes map[string]*Node
	Edges []*Edge
}

// NewGOBRPGStore creates a new GOB-based RPG store.
func NewGOBRPGStore(indexPath string) *GOBRPGStore {
	return &GOBRPGStore{
		indexPath: indexPath,
		graph:     NewGraph(),
	}
}

// Load reads the graph from persistent storage.
func (s *GOBRPGStore) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ensureParentDir(s.indexPath); err != nil {
		return fmt.Errorf("failed to prepare rpg index directory: %w", err)
	}

	file, err := os.Create(s.indexPath)
	if err != nil {
		return fmt.Errorf("failed to create rpg index file: %w", err)
	}
	defer file.Close()

	data := gobRPGData{
		Nodes: s.graph.Nodes,
		Edges: s.graph.Edges,
	}

	if err := gob.NewEncoder(file).Encode(data); err != nil {
		return fmt.Errorf("failed to encode rpg index: %w", err)
	}

	return nil
}

// ensureParentDir creates parent directories if missing.
// Duplicated in store/ and trace/ to avoid cross-package dependency for a trivial helper.
func ensureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0755)
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

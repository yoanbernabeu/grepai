package store

import (
	"context"
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type GOBStore struct {
	indexPath string
	lockPath  string
	chunks    map[string]Chunk    // id -> chunk
	documents map[string]Document // path -> document
	mu        sync.RWMutex
}

type gobData struct {
	Chunks    map[string]Chunk
	Documents map[string]Document
}

func NewGOBStore(indexPath string) *GOBStore {
	return &GOBStore{
		indexPath: indexPath,
		lockPath:  indexPath + ".lock",
		chunks:    make(map[string]Chunk),
		documents: make(map[string]Document),
	}
}

func (s *GOBStore) SaveChunks(ctx context.Context, chunks []Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, chunk := range chunks {
		s.chunks[chunk.ID] = chunk
	}

	return nil
}

func (s *GOBStore) DeleteByFile(ctx context.Context, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.documents[filePath]
	if !ok {
		return nil
	}

	for _, chunkID := range doc.ChunkIDs {
		delete(s.chunks, chunkID)
	}

	return nil
}

func (s *GOBStore) Search(ctx context.Context, queryVector []float32, limit int, opts SearchOptions) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]SearchResult, 0, len(s.chunks))

	for _, chunk := range s.chunks {
		// Filter by path prefix if provided
		if opts.PathPrefix != "" && !strings.HasPrefix(chunk.FilePath, opts.PathPrefix) {
			continue
		}
		score := cosineSimilarity(queryVector, chunk.Vector)
		results = append(results, SearchResult{
			Chunk: chunk,
			Score: score,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (s *GOBStore) GetDocument(ctx context.Context, filePath string) (*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.documents[filePath]
	if !ok {
		return nil, nil
	}

	return &doc, nil
}

func (s *GOBStore) SaveDocument(ctx context.Context, doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.documents[doc.Path] = doc
	return nil
}

func (s *GOBStore) DeleteDocument(ctx context.Context, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.documents, filePath)
	return nil
}

func (s *GOBStore) ListDocuments(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := make([]string, 0, len(s.documents))
	for path := range s.documents {
		paths = append(paths, path)
	}

	return paths, nil
}

func (s *GOBStore) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Acquire shared (read) file lock for cross-process safety
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		// If we can't create lock file, proceed without locking (backward compat)
		return s.loadUnlocked()
	}
	defer lockFile.Close()

	if err := flockShared(lockFile); err != nil {
		// If locking fails, proceed without locking (backward compat)
		return s.loadUnlocked()
	}
	defer func() {
		_ = funlock(lockFile)
	}()

	return s.loadUnlocked()
}

// loadUnlocked performs the actual load without any locking.
func (s *GOBStore) loadUnlocked() error {
	file, err := os.Open(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer file.Close()

	var data gobData
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode index: %w", err)
	}

	s.chunks = data.Chunks
	s.documents = data.Documents

	if s.chunks == nil {
		s.chunks = make(map[string]Chunk)
	}
	if s.documents == nil {
		s.documents = make(map[string]Document)
	}

	return nil
}

func (s *GOBStore) Persist(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ensureParentDir(s.indexPath); err != nil {
		return fmt.Errorf("failed to prepare index directory: %w", err)
	}

	// Acquire exclusive (write) file lock for cross-process safety
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		// If we can't create lock file, proceed without locking (backward compat)
		return s.persistUnlocked()
	}
	defer lockFile.Close()

	if err := flockExclusive(lockFile); err != nil {
		// If locking fails, proceed without locking (backward compat)
		return s.persistUnlocked()
	}
	defer func() {
		_ = funlock(lockFile)
	}()

	return s.persistUnlocked()
}

// persistUnlocked performs the actual persist without any locking.
func (s *GOBStore) persistUnlocked() error {
	file, err := os.Create(s.indexPath)
	if err != nil {
		return fmt.Errorf("failed to create index file: %w", err)
	}
	defer file.Close()

	data := gobData{
		Chunks:    s.chunks,
		Documents: s.documents,
	}

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode index: %w", err)
	}

	return nil
}

func ensureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0755)
}

func (s *GOBStore) Close() error {
	return s.Persist(context.Background())
}

func (s *GOBStore) Stats() (numDocs int, numChunks int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.documents), len(s.chunks)
}

func (s *GOBStore) GetStats(ctx context.Context) (*IndexStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastUpdated time.Time
	for _, chunk := range s.chunks {
		if chunk.UpdatedAt.After(lastUpdated) {
			lastUpdated = chunk.UpdatedAt
		}
	}

	// Get file size
	var size int64
	if info, err := os.Stat(s.indexPath); err == nil {
		size = info.Size()
	}

	return &IndexStats{
		TotalFiles:  len(s.documents),
		TotalChunks: len(s.chunks),
		IndexSize:   size,
		LastUpdated: lastUpdated,
	}, nil
}

func (s *GOBStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make([]FileStats, 0, len(s.documents))
	for _, doc := range s.documents {
		stats = append(stats, FileStats{
			Path:       doc.Path,
			ChunkCount: len(doc.ChunkIDs),
			ModTime:    doc.ModTime,
		})
	}
	return stats, nil
}

func (s *GOBStore) GetChunksForFile(ctx context.Context, filePath string) ([]Chunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.documents[filePath]
	if !ok {
		return nil, nil
	}

	chunks := make([]Chunk, 0, len(doc.ChunkIDs))
	for _, id := range doc.ChunkIDs {
		if chunk, ok := s.chunks[id]; ok {
			chunks = append(chunks, chunk)
		}
	}
	return chunks, nil
}

func (s *GOBStore) GetAllChunks(ctx context.Context) ([]Chunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chunks := make([]Chunk, 0, len(s.chunks))
	for _, chunk := range s.chunks {
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// LookupByContentHash searches in-memory chunks for a matching content hash.
func (s *GOBStore) LookupByContentHash(ctx context.Context, contentHash string) ([]float32, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, chunk := range s.chunks {
		if chunk.ContentHash == contentHash && len(chunk.Vector) > 0 {
			vec := make([]float32, len(chunk.Vector))
			copy(vec, chunk.Vector)
			return vec, true, nil
		}
	}

	return nil, false, nil
}

// cosineSimilarity calculates the cosine similarity between two vectors
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

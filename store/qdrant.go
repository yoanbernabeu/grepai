package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

type QdrantStore struct {
	client         *qdrant.Client
	collectionName string
	dimensions     int
	apiKey         string
}

func parseHost(endpoint string) string {
	host := strings.TrimPrefix(endpoint, "http://")
	host = strings.TrimPrefix(host, "https://")

	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}

	return host
}

func NewQdrantStore(ctx context.Context, endpoint string, port int, useTLS bool, collection, apiKey string, dimensions int) (*QdrantStore, error) {
	host := parseHost(endpoint)

	if port <= 0 {
		port = 6334
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   host,
		Port:   port,
		UseTLS: useTLS,
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	store := &QdrantStore{
		client:         client,
		collectionName: collection,
		dimensions:     dimensions,
		apiKey:         apiKey,
	}

	if err := store.ensureCollection(ctx); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *QdrantStore) ensureCollection(ctx context.Context) error {
	exists, err := s.client.CollectionExists(ctx, s.collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if !exists {
		if s.dimensions <= 0 {
			return fmt.Errorf("dimensions must be positive, got: %d", s.dimensions)
		}
		err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: s.collectionName,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     uint64(s.dimensions),
				Distance: qdrant.Distance_Cosine,
			}),
		})
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
	}

	return nil
}

func sanitizeCollectionName(path string) string {
	return strings.ReplaceAll(path, "/", "_")
}

func SanitizeCollectionName(path string) string {
	return sanitizeCollectionName(path)
}

func (s *QdrantStore) getUUIDForChunk(chunkID string) uuid.UUID {
	namespace := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	return uuid.NewSHA1(namespace, []byte(chunkID))
}

func (s *QdrantStore) SaveChunks(ctx context.Context, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	points := make([]*qdrant.PointStruct, 0, len(chunks))
	for _, chunk := range chunks {
		payload, err := s.buildChunkPayload(chunk)
		if err != nil {
			return fmt.Errorf("failed to build payload: %w", err)
		}

		pointID := s.getUUIDForChunk(chunk.ID)

		points = append(points, &qdrant.PointStruct{
			Id:      qdrant.NewID(pointID.String()),
			Vectors: qdrant.NewVectors(chunk.Vector...),
			Payload: payload,
		})
	}

	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collectionName,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert points: %w", err)
	}

	return nil
}

func (s *QdrantStore) buildChunkPayload(chunk Chunk) (map[string]*qdrant.Value, error) {
	payload := make(map[string]*qdrant.Value)

	filePathVal, err := qdrant.NewValue(chunk.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file_path value: %w", err)
	}

	startLineVal, err := qdrant.NewValue(int64(chunk.StartLine))
	if err != nil {
		return nil, fmt.Errorf("failed to create start_line value: %w", err)
	}

	endLineVal, err := qdrant.NewValue(int64(chunk.EndLine))
	if err != nil {
		return nil, fmt.Errorf("failed to create end_line value: %w", err)
	}

	contentVal, err := qdrant.NewValue(chunk.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to create content value: %w", err)
	}

	hashVal, err := qdrant.NewValue(chunk.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to create hash value: %w", err)
	}

	updatedAtVal, err := qdrant.NewValue(chunk.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to create updated_at value: %w", err)
	}

	payload["file_path"] = filePathVal
	payload["start_line"] = startLineVal
	payload["end_line"] = endLineVal
	payload["content"] = contentVal
	payload["hash"] = hashVal
	payload["updated_at"] = updatedAtVal

	return payload, nil
}

func (s *QdrantStore) DeleteByFile(ctx context.Context, filePath string) error {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("file_path", filePath),
		},
	}

	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collectionName,
		Points:         qdrant.NewPointsSelectorFilter(filter),
	})
	if err != nil {
		return fmt.Errorf("failed to delete points: %w", err)
	}

	return nil
}

func (s *QdrantStore) Search(ctx context.Context, queryVector []float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got: %d", limit)
	}
	searchResult, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResult))
	for _, point := range searchResult {
		chunk := s.parseChunkPayload(point.Payload)
		results = append(results, SearchResult{
			Chunk: *chunk,
			Score: point.Score,
		})
	}

	return results, nil
}

func (s *QdrantStore) parseChunkPayload(payload map[string]*qdrant.Value) *Chunk {
	chunk := &Chunk{}
	if val, ok := payload["file_path"]; ok {
		chunk.FilePath = val.GetStringValue()
	}
	if val, ok := payload["start_line"]; ok {
		chunk.StartLine = int(val.GetIntegerValue())
	}
	if val, ok := payload["end_line"]; ok {
		chunk.EndLine = int(val.GetIntegerValue())
	}
	if val, ok := payload["content"]; ok {
		chunk.Content = val.GetStringValue()
	}
	if val, ok := payload["hash"]; ok {
		chunk.Hash = val.GetStringValue()
	}
	if val, ok := payload["updated_at"]; ok {
		t, err := time.Parse(time.RFC3339, val.GetStringValue())
		if err == nil {
			chunk.UpdatedAt = t
		}
	}

	return chunk
}

func (s *QdrantStore) GetDocument(ctx context.Context, filePath string) (*Document, error) {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("file_path", filePath),
		},
	}

	scrollResult, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter:         filter,
		Limit:          qdrant.PtrOf(uint32(1)),
		WithPayload:    qdrant.NewWithPayloadInclude("chunk_ids"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	if len(scrollResult) == 0 {
		return nil, nil
	}

	doc := &Document{
		Path:     filePath,
		ChunkIDs: []string{},
	}

	return doc, nil
}

func (s *QdrantStore) SaveDocument(ctx context.Context, doc Document) error {
	return nil
}

func (s *QdrantStore) DeleteDocument(ctx context.Context, filePath string) error {
	return nil
}

func (s *QdrantStore) ListDocuments(ctx context.Context) ([]string, error) {
	scrollResult, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Limit:          qdrant.PtrOf(uint32(1000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	pathsMap := make(map[string]bool)
	for _, point := range scrollResult {
		if val, ok := point.Payload["file_path"]; ok {
			pathsMap[val.GetStringValue()] = true
		}
	}

	paths := make([]string, 0, len(pathsMap))
	for path := range pathsMap {
		paths = append(paths, path)
	}

	return paths, nil
}

func (s *QdrantStore) Load(ctx context.Context) error {
	return nil
}

func (s *QdrantStore) Persist(ctx context.Context) error {
	return nil
}

func (s *QdrantStore) Close() error {
	return nil
}

func (s *QdrantStore) GetStats(ctx context.Context) (*IndexStats, error) {
	collectionInfo, err := s.client.GetCollectionInfo(ctx, s.collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection info: %w", err)
	}

	pointsCount := *collectionInfo.PointsCount
	if pointsCount > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("points count %d exceeds maximum int value", pointsCount)
	}

	stats := &IndexStats{
		TotalFiles:  0,
		TotalChunks: int(pointsCount),
		IndexSize:   0,
		LastUpdated: time.Now(),
	}

	return stats, nil
}

func (s *QdrantStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	scrollResult, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Limit:          qdrant.PtrOf(uint32(10000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	fileStats := make(map[string]*FileStats)
	for _, point := range scrollResult {
		filePath := ""
		if val, ok := point.Payload["file_path"]; ok {
			filePath = val.GetStringValue()
		}

		if filePath == "" {
			continue
		}

		if _, exists := fileStats[filePath]; !exists {
			fileStats[filePath] = &FileStats{
				Path:       filePath,
				ChunkCount: 0,
				ModTime:    time.Now(),
			}
		}
		fileStats[filePath].ChunkCount++
	}

	result := make([]FileStats, 0, len(fileStats))
	for _, stat := range fileStats {
		result = append(result, *stat)
	}

	return result, nil
}

func (s *QdrantStore) GetChunksForFile(ctx context.Context, filePath string) ([]Chunk, error) {
	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("file_path", filePath),
		},
	}

	scrollResult, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter:         filter,
		Limit:          qdrant.PtrOf(uint32(10000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
		WithVectors:    qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}

	chunks := make([]Chunk, 0, len(scrollResult))
	for _, point := range scrollResult {
		chunk := s.parseChunkPayload(point.Payload)
		if point.Vectors != nil && point.Vectors.GetVector() != nil {
			if dense := point.Vectors.GetVector().GetDense(); dense != nil {
				chunk.Vector = dense.GetData()
			}
		}
		chunks = append(chunks, *chunk)
	}

	return chunks, nil
}

func (s *QdrantStore) GetAllChunks(ctx context.Context) ([]Chunk, error) {
	scrollResult, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Limit:          qdrant.PtrOf(uint32(100000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
		WithVectors:    qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get all chunks: %w", err)
	}

	chunks := make([]Chunk, 0, len(scrollResult))
	for _, point := range scrollResult {
		chunk := s.parseChunkPayload(point.Payload)
		if point.Vectors != nil && point.Vectors.GetVector() != nil {
			if dense := point.Vectors.GetVector().GetDense(); dense != nil {
				chunk.Vector = dense.GetData()
			}
		}
		chunks = append(chunks, *chunk)
	}

	return chunks, nil
}

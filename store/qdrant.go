package store

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

// sanitizeUTF8 ensures the string contains only valid UTF-8 characters.
// Invalid sequences are replaced with the Unicode replacement character.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

type QdrantStore struct {
	client         qdrantClient
	collectionName string
	dimensions     int
	apiKey         string
}

type qdrantClient interface {
	CollectionExists(ctx context.Context, collectionName string) (bool, error)
	CreateCollection(ctx context.Context, request *qdrant.CreateCollection) error
	CreateFieldIndex(ctx context.Context, request *qdrant.CreateFieldIndexCollection) (*qdrant.UpdateResult, error)
	Upsert(ctx context.Context, request *qdrant.UpsertPoints) (*qdrant.UpdateResult, error)
	SetPayload(ctx context.Context, request *qdrant.SetPayloadPoints) (*qdrant.UpdateResult, error)
	Delete(ctx context.Context, request *qdrant.DeletePoints) (*qdrant.UpdateResult, error)
	Query(ctx context.Context, request *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error)
	Scroll(ctx context.Context, request *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, error)
	ScrollAndOffset(ctx context.Context, request *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error)
	GetCollectionInfo(ctx context.Context, collectionName string) (*qdrant.CollectionInfo, error)
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

	// Create field indexes for efficient filtered lookups.
	// Errors are intentionally ignored because the indexes may already exist.
	_, _ = s.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: s.collectionName,
		FieldName:      "content_hash",
		FieldType:      qdrant.PtrOf(qdrant.FieldType_FieldTypeKeyword),
	})
	_, _ = s.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: s.collectionName,
		FieldName:      "file_path",
		FieldType:      qdrant.PtrOf(qdrant.FieldType_FieldTypeKeyword),
	})

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

	contentVal, err := qdrant.NewValue(sanitizeUTF8(chunk.Content))
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

	if chunk.ContentHash != "" {
		contentHashVal, err := qdrant.NewValue(chunk.ContentHash)
		if err != nil {
			return nil, fmt.Errorf("failed to create content_hash value: %w", err)
		}
		payload["content_hash"] = contentHashVal
	}

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

func (s *QdrantStore) Search(ctx context.Context, queryVector []float32, limit int, opts SearchOptions) ([]SearchResult, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got: %d", limit)
	}

	// Fetch more results to account for filtering by path prefix
	fetchLimit := limit
	if opts.PathPrefix != "" {
		// Fetch 2x the limit to allow for filtering
		maxInt := int(^uint(0) >> 1)
		if limit > maxInt/2 {
			fetchLimit = maxInt
		} else {
			fetchLimit = limit * 2
		}
	}
	if fetchLimit < 0 {
		return nil, fmt.Errorf("invalid fetch limit: %d", fetchLimit)
	}
	fetchLimitU64 := uint64(fetchLimit)

	searchResult, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collectionName,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          qdrant.PtrOf(fetchLimitU64),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResult))
	for _, point := range searchResult {
		chunk := s.parseChunkPayload(point.Payload)

		// Filter by path prefix if provided
		if opts.PathPrefix != "" && !strings.HasPrefix(chunk.FilePath, opts.PathPrefix) {
			continue
		}

		results = append(results, SearchResult{
			Chunk: *chunk,
			Score: point.Score,
		})

		// Stop once we have enough results
		if len(results) >= limit {
			break
		}
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
	if val, ok := payload["content_hash"]; ok {
		chunk.ContentHash = val.GetStringValue()
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
		WithPayload:    qdrant.NewWithPayloadInclude("doc_hash"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	if len(scrollResult) == 0 {
		return nil, nil
	}

	doc := &Document{
		Path: filePath,
	}

	if val, ok := scrollResult[0].Payload["doc_hash"]; ok && val.GetStringValue() != "" {
		doc.Hash = val.GetStringValue()
		// Sentinel value: the indexer only checks len(ChunkIDs) > 0.
		// Fetching all chunk IDs would require scrolling the entire file,
		// so we use a single placeholder to signal that chunks exist.
		doc.ChunkIDs = []string{"_"}
	}
	// Legacy chunks without doc_hash return an empty Hash and nil ChunkIDs,
	// which causes the indexer to re-index the file and write doc_hash.

	return doc, nil
}

func (s *QdrantStore) SaveDocument(ctx context.Context, doc Document) error {
	if doc.Hash == "" {
		return nil
	}

	docHashVal, err := qdrant.NewValue(doc.Hash)
	if err != nil {
		return fmt.Errorf("failed to create doc_hash value: %w", err)
	}

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("file_path", doc.Path),
		},
	}

	_, err = s.client.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: s.collectionName,
		Payload: map[string]*qdrant.Value{
			"doc_hash": docHashVal,
		},
		PointsSelector: qdrant.NewPointsSelectorFilter(filter),
	})
	if err != nil {
		return fmt.Errorf("failed to set doc_hash payload: %w", err)
	}

	return nil
}

// DeleteDocument is a no-op for Qdrant because document metadata (doc_hash)
// is stored as payload on chunk points. RemoveFile always calls DeleteByFile
// first, which deletes all chunk points, so there is nothing left to clean up.
func (s *QdrantStore) DeleteDocument(ctx context.Context, filePath string) error {
	return nil
}

func (s *QdrantStore) ListDocuments(ctx context.Context) ([]string, error) {
	points, err := s.scrollAll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Limit:          qdrant.PtrOf(uint32(1000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	pathsMap := make(map[string]bool)
	for _, point := range points {
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

	pointsCount := uint64(0)
	if collectionInfo.PointsCount != nil {
		pointsCount = *collectionInfo.PointsCount
	}
	if pointsCount > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("points count %d exceeds maximum int value", pointsCount)
	}

	scrollResult, err := s.scrollAll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Limit:          qdrant.PtrOf(uint32(10000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list files for stats: %w", err)
	}

	stats := &IndexStats{
		TotalFiles:  countFilesFromRetrievedPoints(scrollResult),
		TotalChunks: int(pointsCount),
		IndexSize:   0,
		LastUpdated: time.Now(),
	}

	return stats, nil
}

func (s *QdrantStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	scrollResult, err := s.scrollAll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Limit:          qdrant.PtrOf(uint32(10000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return fileStatsFromRetrievedPoints(scrollResult), nil
}

func (s *QdrantStore) scrollAll(ctx context.Context, req *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, error) {
	var (
		allPoints []*qdrant.RetrievedPoint
		offset    *qdrant.PointId
	)

	for {
		reqCopy := cloneScrollPointsRequest(req)
		reqCopy.Offset = offset

		points, nextOffset, err := s.client.ScrollAndOffset(ctx, reqCopy)
		if err != nil {
			return nil, err
		}
		allPoints = append(allPoints, points...)
		if nextOffset == nil || len(points) == 0 {
			return allPoints, nil
		}
		offset = nextOffset
	}
}

func cloneScrollPointsRequest(req *qdrant.ScrollPoints) *qdrant.ScrollPoints {
	if req == nil {
		return nil
	}

	return &qdrant.ScrollPoints{
		CollectionName:   req.CollectionName,
		Filter:           req.Filter,
		Offset:           req.Offset,
		Limit:            req.Limit,
		WithPayload:      req.WithPayload,
		WithVectors:      req.WithVectors,
		ReadConsistency:  req.ReadConsistency,
		ShardKeySelector: req.ShardKeySelector,
		OrderBy:          req.OrderBy,
		Timeout:          req.Timeout,
	}
}

func countFilesFromRetrievedPoints(points []*qdrant.RetrievedPoint) int {
	return len(fileStatsFromRetrievedPoints(points))
}

func fileStatsFromRetrievedPoints(points []*qdrant.RetrievedPoint) []FileStats {
	fileStats := make(map[string]*FileStats)
	for _, point := range points {
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
	return result
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
	points, err := s.scrollAll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Limit:          qdrant.PtrOf(uint32(1000)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
		WithVectors:    qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get all chunks: %w", err)
	}

	chunks := make([]Chunk, 0, len(points))
	for _, point := range points {
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

// LookupByContentHash searches Qdrant for a point matching the content hash.
func (s *QdrantStore) LookupByContentHash(ctx context.Context, contentHash string) ([]float32, bool, error) {
	if contentHash == "" {
		return nil, false, nil
	}

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("content_hash", contentHash),
		},
	}

	scrollResult, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter:         filter,
		Limit:          qdrant.PtrOf(uint32(1)),
		WithVectors:    qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to lookup by content hash: %w", err)
	}

	if len(scrollResult) == 0 {
		return nil, false, nil
	}

	point := scrollResult[0]
	if point.Vectors != nil && point.Vectors.GetVector() != nil {
		if dense := point.Vectors.GetVector().GetDense(); dense != nil {
			return dense.GetData(), true, nil
		}
	}

	return nil, false, nil
}

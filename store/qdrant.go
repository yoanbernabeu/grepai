package store

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	client         *qdrant.Client
	collectionName string
	dimensions     int
	apiKey         string
	address        string // host:port, for error messages
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
		GrpcOptions: []grpc.DialOption{
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64 * 1024 * 1024)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	store := &QdrantStore{
		client:         client,
		collectionName: collection,
		dimensions:     dimensions,
		apiKey:         apiKey,
		address:        fmt.Sprintf("%s:%d", host, port),
	}

	if err := store.ensureCollection(ctx); err != nil {
		return nil, err
	}

	return store, nil
}

// connectionError explains the most common cause of a failed first call:
// Qdrant not running, or grepai pointed at the REST port instead of gRPC (#177).
func (s *QdrantStore) connectionError(err error) error {
	hint := ""
	switch {
	case strings.Contains(err.Error(), "HTTP/1.1"):
		hint = "\nThe server answered with HTTP/1.1: grepai talks to Qdrant over gRPC, so use the gRPC port (6334 by default), not the REST port (6333)."
	case status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded:
		hint = "\nIs Qdrant running? Start it with: docker run -p 6334:6334 qdrant/qdrant\nOtherwise check store.qdrant.endpoint and store.qdrant.port in .grepai/config.yaml."
	}
	return fmt.Errorf("cannot reach Qdrant at %s: %w%s", s.address, err, hint)
}

func (s *QdrantStore) ensureCollection(ctx context.Context) error {
	exists, err := s.client.CollectionExists(ctx, s.collectionName)
	if err != nil {
		return s.connectionError(err)
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

	// Create field index for content_hash to enable efficient lookups.
	// Error is intentionally ignored because the index may already exist.
	_, _ = s.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: s.collectionName,
		FieldName:      "content_hash",
		FieldType:      qdrant.PtrOf(qdrant.FieldType_FieldTypeKeyword),
	})

	return nil
}

func sanitizeCollectionName(path string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\\', ':', '/':
			return '_'
		}
		return r
	}, path)
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

	if chunk.FileHash != "" {
		fileHashVal, err := qdrant.NewValue(chunk.FileHash)
		if err != nil {
			return nil, fmt.Errorf("failed to create file_hash value: %w", err)
		}
		payload["file_hash"] = fileHashVal
	}

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

// qdrantPathSearchMaxScanned bounds how many ranked points a path-filtered
// search examines before giving up on filling the limit.
const qdrantPathSearchMaxScanned = 20000

func (s *QdrantStore) Search(ctx context.Context, queryVector []float32, limit int, opts SearchOptions) ([]SearchResult, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got: %d", limit)
	}

	// The path prefix is filtered here, not in Qdrant, so a path-filtered
	// search walks the ranking page by page until it has enough matches.
	// Fetching a single page of 2x the limit returned too few results (often
	// none) whenever the best matches were outside the path.
	pageSize := limit
	if opts.PathPrefix != "" {
		pageSize = limit * 4
		if pageSize < 64 {
			pageSize = 64
		}
	}

	results := make([]SearchResult, 0, limit)
	for offset := 0; offset < qdrantPathSearchMaxScanned; offset += pageSize {
		page, err := s.client.Query(ctx, &qdrant.QueryPoints{
			CollectionName: s.collectionName,
			Query:          qdrant.NewQuery(queryVector...),
			Limit:          qdrant.PtrOf(uint64(pageSize)),
			Offset:         qdrant.PtrOf(uint64(offset)),
			WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to search: %w", err)
		}

		for _, point := range page {
			chunk := s.parseChunkPayload(point.Payload)
			if opts.PathPrefix != "" && !strings.HasPrefix(chunk.FilePath, opts.PathPrefix) {
				continue
			}
			results = append(results, SearchResult{Chunk: *chunk, Score: point.Score})
			if len(results) >= limit {
				return results, nil
			}
		}

		if opts.PathPrefix == "" || len(page) < pageSize {
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
	if val, ok := payload["file_hash"]; ok {
		chunk.FileHash = val.GetStringValue()
	}
	if val, ok := payload["content_hash"]; ok {
		chunk.ContentHash = val.GetStringValue()
	}

	return chunk
}

// qdrantScrollPageSize is the number of points read per scroll request.
const qdrantScrollPageSize = 1000

// qdrantLookupLimit bounds the points (and vectors) read by one
// LookupByContentHash call.
const qdrantLookupLimit = 64

// scrollEach reads every point matching req, page by page. A single Scroll
// call returns at most one page, so reading only the first page silently
// truncated results on collections with more points than the page size.
func (s *QdrantStore) scrollEach(ctx context.Context, req *qdrant.ScrollPoints, visit func(*qdrant.RetrievedPoint)) error {
	req.Limit = qdrant.PtrOf(uint32(qdrantScrollPageSize))
	for {
		points, next, err := s.client.ScrollAndOffset(ctx, req)
		if err != nil {
			return err
		}
		for _, point := range points {
			visit(point)
		}
		if next == nil || len(points) == 0 {
			return nil
		}
		req.Offset = next
	}
}

func pointVector(point *qdrant.RetrievedPoint) []float32 {
	if point.Vectors != nil && point.Vectors.GetVector() != nil {
		if dense := point.Vectors.GetVector().GetDense(); dense != nil {
			return dense.GetData()
		}
	}
	return nil
}

func (s *QdrantStore) GetDocument(ctx context.Context, filePath string) (*Document, error) {
	doc := &Document{Path: filePath}
	err := s.scrollEach(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{qdrant.NewMatch("file_path", filePath)},
		},
		WithPayload: qdrant.NewWithPayloadInclude("file_path", "file_hash"),
	}, func(point *qdrant.RetrievedPoint) {
		doc.ChunkIDs = append(doc.ChunkIDs, point.GetId().String())
		if doc.Hash == "" {
			// Older points have no file_hash; keep Hash empty so the hash
			// comparison requests re-indexing rather than using a chunk hash.
			doc.Hash = point.GetPayload()["file_hash"].GetStringValue()
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	if len(doc.ChunkIDs) == 0 {
		return nil, nil
	}
	return doc, nil
}

// SaveDocument is a no-op: document metadata is stored in chunk payloads by SaveChunks.
func (s *QdrantStore) SaveDocument(ctx context.Context, doc Document) error {
	return nil
}

// DeleteDocument is a no-op: DeleteByFile removes the chunks and their document metadata.
func (s *QdrantStore) DeleteDocument(ctx context.Context, filePath string) error {
	return nil
}

func (s *QdrantStore) ListDocuments(ctx context.Context) ([]string, error) {
	pathsMap := make(map[string]bool)
	err := s.scrollEach(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		WithPayload:    qdrant.NewWithPayloadInclude("file_path"),
	}, func(point *qdrant.RetrievedPoint) {
		if val, ok := point.Payload["file_path"]; ok {
			pathsMap[val.GetStringValue()] = true
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
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

	files, err := s.ListFilesWithStats(ctx)
	if err != nil {
		return nil, err
	}
	var lastUpdated time.Time
	for _, f := range files {
		if f.ModTime.After(lastUpdated) {
			lastUpdated = f.ModTime
		}
	}

	return &IndexStats{
		TotalFiles:  len(files),
		TotalChunks: int(pointsCount),
		IndexSize:   0, // not exposed by Qdrant
		LastUpdated: lastUpdated,
	}, nil
}

func (s *QdrantStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	fileStats := make(map[string]*FileStats)
	err := s.scrollEach(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "updated_at"),
	}, func(point *qdrant.RetrievedPoint) {
		filePath := point.Payload["file_path"].GetStringValue()
		if filePath == "" {
			return
		}
		stat, exists := fileStats[filePath]
		if !exists {
			stat = &FileStats{Path: filePath}
			fileStats[filePath] = stat
		}
		stat.ChunkCount++
		// Last update of the file = most recent chunk write.
		if t, err := time.Parse(time.RFC3339, point.Payload["updated_at"].GetStringValue()); err == nil && t.After(stat.ModTime) {
			stat.ModTime = t
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	result := make([]FileStats, 0, len(fileStats))
	for _, stat := range fileStats {
		result = append(result, *stat)
	}
	return result, nil
}

func (s *QdrantStore) GetChunksForFile(ctx context.Context, filePath string) ([]Chunk, error) {
	var chunks []Chunk
	err := s.scrollEach(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{qdrant.NewMatch("file_path", filePath)},
		},
		WithPayload: qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
		WithVectors: qdrant.NewWithVectors(true),
	}, func(point *qdrant.RetrievedPoint) {
		chunk := s.parseChunkPayload(point.Payload)
		chunk.Vector = pointVector(point)
		// The payload has no chunk ID: use the point ID, so each chunk keeps a
		// distinct ID (RPG chunk nodes are keyed by it).
		chunk.ID = point.GetId().GetUuid()
		chunks = append(chunks, *chunk)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}
	return chunks, nil
}

func (s *QdrantStore) GetAllChunks(ctx context.Context) ([]Chunk, error) {
	var chunks []Chunk
	err := s.scrollEach(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
	}, func(point *qdrant.RetrievedPoint) {
		chunk := s.parseChunkPayload(point.Payload)
		chunk.Vector = pointVector(point)
		chunks = append(chunks, *chunk)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get all chunks: %w", err)
	}
	return chunks, nil
}

// LookupByContentHash searches Qdrant for a point matching the content hash.
func (s *QdrantStore) LookupByContentHash(ctx context.Context, contentHash, contentPrefix string) ([]float32, bool, error) {
	if contentHash == "" {
		return nil, false, nil
	}

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewMatch("content_hash", contentHash),
		},
	}

	// Qdrant cannot match a payload prefix without a full-text index: check
	// the content of the points sharing this content hash (usually one or a
	// few). Only the first qdrantLookupLimit are checked: past that, a miss
	// just re-embeds the chunk, which is cheaper than scanning every copy.
	points, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter:         filter,
		Limit:          qdrant.PtrOf(uint32(qdrantLookupLimit)),
		WithPayload:    qdrant.NewWithPayloadInclude("content"),
		WithVectors:    qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to lookup by content hash: %w", err)
	}
	// Stored content is sanitized (see buildChunkPayload): compare likewise.
	contentPrefix = sanitizeUTF8(contentPrefix)
	for _, point := range points {
		if !strings.HasPrefix(point.GetPayload()["content"].GetStringValue(), contentPrefix) {
			continue
		}
		if vec := pointVector(point); vec != nil {
			return vec, true, nil
		}
	}
	return nil, false, nil
}

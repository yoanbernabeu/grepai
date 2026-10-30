package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"github.com/yoanbernabeu/grepai/git"
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
	// namespace scopes every point to one project inside a collection that
	// several projects may share. Empty keeps the layout of collections named
	// after the project path: point IDs and payloads exactly as before (#247).
	namespace string
	// noLegacyVectors stops the embedding cache from reusing vectors of
	// points without a namespace, which carry no model information: the
	// indexer sets it after its initial scan and after a model change.
	noLegacyVectors bool
	// migrating is set when the store opened on a namespace without points:
	// the project has not been indexed since points got a namespace.
	migrating bool
	// readLegacy makes search and status read the points an earlier version
	// wrote without a namespace, while the project has none of its own yet
	// (just after upgrading, before it is re-indexed). The indexer never sets
	// it, so it still sees an empty namespace and re-indexes the project.
	readLegacy bool
}

// qdrantNamespaceField is the payload field holding a point's project
// namespace. Points written without a namespace (default collections, or
// explicit collections indexed before #247) do not have it.
const qdrantNamespaceField = "project_namespace"

// QdrantProjectNamespace returns the namespace a project uses inside its
// Qdrant collection. A collection derived from the project path belongs to that
// project alone and needs none. An explicitly configured collection can be
// shared by several projects, so their points are kept apart (#247) by
// repository: the normalized origin remote URL (plus the project's directory
// inside the repository), so that clones on other machines and linked
// worktrees of one repository still share an index. Without a git repository
// or a remote that names one, the resolved project root is used; when git is
// missing or fails (timeout, untrusted repository) too, with a warning, since
// processes that can read the repository then use another namespace.
func QdrantProjectNamespace(configuredCollection, projectRoot string) string {
	if configuredCollection == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(projectRoot); err == nil {
		projectRoot = resolved
	}
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	projectRoot = filepath.Clean(projectRoot)
	if cached, ok := qdrantNamespaces.Load(projectRoot); ok {
		if namespace, ok := cached.(string); ok {
			return namespace
		}
	}
	namespace := projectRoot
	identity, ok, err := git.RepoIdentity(projectRoot)
	switch {
	case errors.Is(err, exec.ErrNotFound):
		log.Printf("Warning: git is not installed: the shared Qdrant collection %q keeps %s apart by its path, not by its repository", configuredCollection, projectRoot)
	case err != nil:
		// Refusing to start would break setups that worked before (e.g. a
		// container running as another user than the repository owner).
		log.Printf("Warning: cannot identify the git repository of %s (%v): the shared Qdrant collection %q keeps it apart by its path, so grepai processes that can read the repository use another namespace", projectRoot, err, configuredCollection)
		return namespace // not cached: the next store retries git
	case ok:
		namespace = identity
	}
	qdrantNamespaces.Store(projectRoot, namespace)
	return namespace
}

// qdrantNamespaceIndexed records the collections whose project_namespace
// payload index this process already requested.
var qdrantNamespaceIndexed sync.Map

// qdrantNamespaces caches QdrantProjectNamespace per project root, so a
// long-running process (MCP server) does not run git on every store it opens.
var qdrantNamespaces sync.Map

// qdrantLegacyWarned remembers the collections already checked for points
// without a namespace, so a long-running process (MCP server, watcher) counts
// and warns once per collection instead of on every store it opens.
var qdrantLegacyWarned sync.Map

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
	return NewQdrantStoreWithNamespace(ctx, endpoint, port, useTLS, collection, apiKey, dimensions, "", false)
}

// NewQdrantStoreWithNamespace opens a Qdrant store whose reads and writes are
// limited to the given project namespace (see QdrantProjectNamespace). An empty
// namespace behaves exactly like NewQdrantStore. With legacyReadFallback, for
// stores that only search or report status, a namespace that holds no points
// yet reads the points written before namespaces existed instead.
func NewQdrantStoreWithNamespace(ctx context.Context, endpoint string, port int, useTLS bool, collection, apiKey string, dimensions int, namespace string, legacyReadFallback bool) (*QdrantStore, error) {
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
		namespace:      namespace,
	}

	if err := store.ensureCollection(ctx); err != nil {
		return nil, err
	}
	if namespace != "" {
		empty, err := store.namespaceEmpty(ctx)
		if err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("failed to check project namespace: %w", err)
		}
		store.readLegacy = legacyReadFallback && empty
		store.migrating = empty
	}
	store.warnLegacyPoints(ctx, legacyReadFallback)

	return store, nil
}

// warnLegacyPoints tells the user about points written without a project
// namespace in a namespaced collection. They are no longer searched; indexing
// replaces the ones whose files belong to this project, and the rest (files
// deleted since, or other projects sharing the collection) stay until removed.
func (s *QdrantStore) warnLegacyPoints(ctx context.Context, readOnly bool) {
	// Search and status only mention old points while they serve them;
	// leftovers after the migration are reported by index and watch.
	if s.namespace == "" || (readOnly && !s.readLegacy) {
		return
	}
	key := s.address + "/" + s.collectionName + "\x00" + s.namespace
	if _, done := qdrantLegacyWarned.LoadOrStore(key, true); done {
		return
	}
	legacy := &qdrant.Filter{Must: []*qdrant.Condition{qdrant.NewIsEmpty(qdrantNamespaceField)}}
	// Most opens find none: test for one point before saying anything.
	points, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter:         legacy,
		Limit:          qdrant.PtrOf(uint32(1)),
	})
	if err != nil {
		qdrantLegacyWarned.Delete(key)
		log.Printf("Warning: failed to check Qdrant collection %q for points without a project namespace: %v", s.collectionName, err)
		return
	}
	switch {
	case len(points) == 0:
		return
	case s.readLegacy:
		log.Printf("Warning: Qdrant collection %q has no points for this project yet: searching the points written by an earlier grepai version until 'grepai index' or 'grepai watch' re-indexes the project (unchanged chunks reuse their vectors).", s.collectionName)
		return
	case s.migrating:
		// Index and watch are about to replace them: nothing is left over yet.
		log.Printf("Qdrant collection %q has no points for this project yet: indexing replaces the points an earlier grepai version wrote for its files, reusing their vectors for unchanged chunks.", s.collectionName)
		return
	}
	count, err := s.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: s.collectionName,
		Filter:         legacy,
		Exact:          qdrant.PtrOf(true),
	})
	if err != nil {
		qdrantLegacyWarned.Delete(key)
		log.Printf("Warning: failed to count Qdrant points without a project namespace in collection %q: %v", s.collectionName, err)
		return
	}
	if count > 0 {
		log.Printf("Warning: Qdrant collection %q holds %d points from an earlier grepai version, which are no longer searched. "+
			"'grepai index' or 'grepai watch' re-indexes this project and replaces its own (unchanged chunks reuse their vectors); "+
			"see the Qdrant \"Sharing a Collection\" docs to remove the rest.",
			s.collectionName, count)
	}
}

// filter limits conditions to this store's project namespace. It returns nil
// when there is nothing to filter on.
func (s *QdrantStore) filter(conditions ...*qdrant.Condition) *qdrant.Filter {
	if s.namespace != "" {
		conditions = append(conditions, qdrant.NewMatch(qdrantNamespaceField, s.namespace))
	}
	if len(conditions) == 0 {
		return nil
	}
	return &qdrant.Filter{Must: conditions}
}

// readFilter is the filter of search and status reads: the project's
// namespace, or the points without a namespace while readLegacy is set.
func (s *QdrantStore) readFilter(conditions ...*qdrant.Condition) *qdrant.Filter {
	if s.readLegacy {
		return &qdrant.Filter{Must: append(conditions, qdrant.NewIsEmpty(qdrantNamespaceField))}
	}
	return s.filter(conditions...)
}

// namespaceEmpty reports whether no point carries this store's namespace yet.
func (s *QdrantStore) namespaceEmpty(ctx context.Context) (bool, error) {
	points, err := s.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter:         s.filter(),
		Limit:          qdrant.PtrOf(uint32(1)),
	})
	if err != nil {
		return false, err
	}
	return len(points) == 0, nil
}

// filterWithLegacy is like filter but also matches points written without a
// namespace, so that re-indexing a project can reuse and then replace the
// points an earlier version wrote for it.
func (s *QdrantStore) filterWithLegacy(conditions ...*qdrant.Condition) *qdrant.Filter {
	if s.namespace == "" {
		return &qdrant.Filter{Must: conditions}
	}
	return &qdrant.Filter{
		Must: conditions,
		Should: []*qdrant.Condition{
			qdrant.NewMatch(qdrantNamespaceField, s.namespace),
			qdrant.NewIsEmpty(qdrantNamespaceField),
		},
	}
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
	if s.namespace != "" {
		key := s.address + "/" + s.collectionName
		if _, done := qdrantNamespaceIndexed.Load(key); !done {
			// An error (the index may already exist) is retried by the next
			// store; success is remembered for the life of the process.
			if _, err := s.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
				CollectionName: s.collectionName,
				FieldName:      qdrantNamespaceField,
				FieldType:      qdrant.PtrOf(qdrant.FieldType_FieldTypeKeyword),
			}); err == nil {
				qdrantNamespaceIndexed.Store(key, true)
			}
		}
	}

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
	if s.namespace != "" {
		// Chunk IDs are relative paths, identical across projects: without the
		// project namespace two projects overwrite each other's points (#247).
		return uuid.NewSHA1(namespace, []byte(s.namespace+"\x00"+chunkID))
	}
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
		// Wait until the points are applied, so a read that follows (RPG
		// chunk linking, document lookup) sees them.
		Wait:   qdrant.PtrOf(true),
		Points: points,
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

	if s.namespace != "" {
		namespaceVal, err := qdrant.NewValue(s.namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s value: %w", qdrantNamespaceField, err)
		}
		payload[qdrantNamespaceField] = namespaceVal
	}

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
	// Points an earlier version wrote for this path without a namespace go too:
	// the file is being re-indexed or removed, and they would otherwise be left
	// unreachable in the collection.
	filter := s.filterWithLegacy(qdrant.NewMatch("file_path", filePath))

	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collectionName,
		Wait:           qdrant.PtrOf(true),
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
			Filter:         s.readFilter(),
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
			// Hybrid search fuses results by chunk ID: use the point ID, as
			// GetAllChunks does, so results stay distinct and match.
			chunk.ID = point.GetId().GetUuid()
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
		Filter:         s.filter(qdrant.NewMatch("file_path", filePath)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "file_hash"),
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

// DisableLegacyVectorReuse stops LookupByContentHash from reusing vectors of
// points written before namespaces existed. The indexer calls it when the
// embedding model changed: those points were not removed with the project's
// namespaced documents and hold vectors of the previous model.
func (s *QdrantStore) DisableLegacyVectorReuse() {
	s.noLegacyVectors = true
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
		Filter:         s.filter(),
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
	files, err := s.ListFilesWithStats(ctx)
	if err != nil {
		return nil, err
	}

	var totalChunks int
	if s.namespace != "" {
		// The collection may hold other projects' points: count only ours,
		// which the namespaced file listing has already scrolled.
		for _, f := range files {
			totalChunks += f.ChunkCount
		}
	} else {
		collectionInfo, err := s.client.GetCollectionInfo(ctx, s.collectionName)
		if err != nil {
			return nil, fmt.Errorf("failed to get collection info: %w", err)
		}
		pointsCount := *collectionInfo.PointsCount
		if pointsCount > uint64(^uint(0)>>1) {
			return nil, fmt.Errorf("points count %d exceeds maximum int value", pointsCount)
		}
		totalChunks = int(pointsCount)
	}

	var lastUpdated time.Time
	for _, f := range files {
		if f.ModTime.After(lastUpdated) {
			lastUpdated = f.ModTime
		}
	}

	return &IndexStats{
		TotalFiles:  len(files),
		TotalChunks: totalChunks,
		IndexSize:   0, // not exposed by Qdrant
		LastUpdated: lastUpdated,
	}, nil
}

func (s *QdrantStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	fileStats := make(map[string]*FileStats)
	err := s.scrollEach(ctx, &qdrant.ScrollPoints{
		CollectionName: s.collectionName,
		Filter:         s.readFilter(),
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
		Filter:         s.readFilter(qdrant.NewMatch("file_path", filePath)),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
		WithVectors:    qdrant.NewWithVectors(true),
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
		Filter:         s.readFilter(),
		WithPayload:    qdrant.NewWithPayloadInclude("file_path", "start_line", "end_line", "content", "hash", "updated_at"),
	}, func(point *qdrant.RetrievedPoint) {
		chunk := s.parseChunkPayload(point.Payload)
		chunk.Vector = pointVector(point)
		chunk.ID = point.GetId().GetUuid()
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

	// Vectors of points an earlier version wrote without a namespace are
	// reused too, so moving an explicit collection to namespaced points does
	// not embed unchanged chunks again.
	filter := s.filterWithLegacy(qdrant.NewMatch("content_hash", contentHash))
	if s.noLegacyVectors {
		filter = s.filter(qdrant.NewMatch("content_hash", contentHash))
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

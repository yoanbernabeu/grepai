package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

type PostgresStore struct {
	pool       *pgxpool.Pool
	projectID  string
	dimensions int
}

func NewPostgresStore(ctx context.Context, dsn string, projectID string, vectorDimensions int) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	store := &PostgresStore{
		pool:       pool,
		projectID:  projectID,
		dimensions: vectorDimensions,
	}

	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *PostgresStore) ensureSchema(ctx context.Context) error {
	queries := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			file_path TEXT NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			content TEXT NOT NULL,
			vector vector(768),
			hash TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_project ON chunks(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_file ON chunks(project_id, file_path)`,
		`CREATE TABLE IF NOT EXISTS documents (
			path TEXT NOT NULL,
			project_id TEXT NOT NULL,
			hash TEXT NOT NULL,
			mod_time TIMESTAMP NOT NULL,
			chunk_ids TEXT[] NOT NULL,
			PRIMARY KEY (project_id, path)
		)`,
		buildEnsureVectorSQL(s.dimensions),
	}

	for _, query := range queries {
		if _, err := s.pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to execute schema query: %w", err)
		}
	}

	return nil
}

func (s *PostgresStore) SaveChunks(ctx context.Context, chunks []Chunk) error {
	batch := &pgx.Batch{}

	for _, chunk := range chunks {
		vec := pgvector.NewVector(chunk.Vector)
		batch.Queue(
			`INSERT INTO chunks (id, project_id, file_path, start_line, end_line, content, vector, hash, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				file_path = EXCLUDED.file_path,
				start_line = EXCLUDED.start_line,
				end_line = EXCLUDED.end_line,
				content = EXCLUDED.content,
				vector = EXCLUDED.vector,
				hash = EXCLUDED.hash,
				updated_at = EXCLUDED.updated_at`,
			chunk.ID, s.projectID, chunk.FilePath, chunk.StartLine, chunk.EndLine,
			chunk.Content, vec, chunk.Hash, chunk.UpdatedAt,
		)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()

	for range chunks {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("failed to save chunk: %w", err)
		}
	}

	return nil
}

func (s *PostgresStore) DeleteByFile(ctx context.Context, filePath string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM chunks WHERE project_id = $1 AND file_path = $2`,
		s.projectID, filePath,
	)
	if err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}
	return nil
}

func (s *PostgresStore) Search(ctx context.Context, queryVector []float32, limit int) ([]SearchResult, error) {
	vec := pgvector.NewVector(queryVector)

	rows, err := s.pool.Query(ctx,
		`SELECT id, file_path, start_line, end_line, content, vector, hash, updated_at,
			1 - (vector <=> $1) as score
		FROM chunks
		WHERE project_id = $2
		ORDER BY vector <=> $1
		LIMIT $3`,
		vec, s.projectID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var chunk Chunk
		var vec pgvector.Vector
		var score float32

		if err := rows.Scan(
			&chunk.ID, &chunk.FilePath, &chunk.StartLine, &chunk.EndLine,
			&chunk.Content, &vec, &chunk.Hash, &chunk.UpdatedAt, &score,
		); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		chunk.Vector = vec.Slice()
		results = append(results, SearchResult{
			Chunk: chunk,
			Score: score,
		})
	}

	return results, rows.Err()
}

func (s *PostgresStore) GetDocument(ctx context.Context, filePath string) (*Document, error) {
	var doc Document
	var modTime time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT path, hash, mod_time, chunk_ids FROM documents WHERE project_id = $1 AND path = $2`,
		s.projectID, filePath,
	).Scan(&doc.Path, &doc.Hash, &modTime, &doc.ChunkIDs)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	doc.ModTime = modTime
	return &doc, nil
}

func (s *PostgresStore) SaveDocument(ctx context.Context, doc Document) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO documents (path, project_id, hash, mod_time, chunk_ids)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, path) DO UPDATE SET
			hash = EXCLUDED.hash,
			mod_time = EXCLUDED.mod_time,
			chunk_ids = EXCLUDED.chunk_ids`,
		doc.Path, s.projectID, doc.Hash, doc.ModTime, doc.ChunkIDs,
	)
	if err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteDocument(ctx context.Context, filePath string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM documents WHERE project_id = $1 AND path = $2`,
		s.projectID, filePath,
	)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListDocuments(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT path FROM documents WHERE project_id = $1`,
		s.projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("failed to scan path: %w", err)
		}
		paths = append(paths, path)
	}

	return paths, rows.Err()
}

func (s *PostgresStore) Load(ctx context.Context) error {
	// No-op for Postgres, data is already persistent
	return nil
}

func (s *PostgresStore) Persist(ctx context.Context) error {
	// No-op for Postgres, data is already persistent
	return nil
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

func (s *PostgresStore) GetStats(ctx context.Context) (*IndexStats, error) {
	var stats IndexStats

	// Get file count
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM documents WHERE project_id = $1`,
		s.projectID,
	).Scan(&stats.TotalFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	// Get chunk count and last updated
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(MAX(updated_at), '1970-01-01'::timestamp) FROM chunks WHERE project_id = $1`,
		s.projectID,
	).Scan(&stats.TotalChunks, &stats.LastUpdated)
	if err != nil {
		return nil, fmt.Errorf("failed to count chunks: %w", err)
	}

	// IndexSize not applicable for Postgres (data stored remotely)
	stats.IndexSize = 0

	return &stats, nil
}

func (s *PostgresStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT path, mod_time, array_length(chunk_ids, 1) FROM documents WHERE project_id = $1`,
		s.projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer rows.Close()

	var files []FileStats
	for rows.Next() {
		var f FileStats
		var chunkCount *int
		if err := rows.Scan(&f.Path, &f.ModTime, &chunkCount); err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		if chunkCount != nil {
			f.ChunkCount = *chunkCount
		}
		files = append(files, f)
	}

	return files, rows.Err()
}

func (s *PostgresStore) GetChunksForFile(ctx context.Context, filePath string) ([]Chunk, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, file_path, start_line, end_line, content, hash, updated_at
		FROM chunks WHERE project_id = $1 AND file_path = $2
		ORDER BY start_line`,
		s.projectID, filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.FilePath, &c.StartLine, &c.EndLine, &c.Content, &c.Hash, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}

	return chunks, rows.Err()
}

func (s *PostgresStore) GetAllChunks(ctx context.Context) ([]Chunk, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, file_path, start_line, end_line, content, hash, updated_at
		FROM chunks WHERE project_id = $1`,
		s.projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get all chunks: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.FilePath, &c.StartLine, &c.EndLine, &c.Content, &c.Hash, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}

	return chunks, rows.Err()
}

// buildEnsureVectorSQL returns a SQL block that alters the "chunks.vector" column
// only if its current dimension differs from the specified one.
func buildEnsureVectorSQL(dim int) string {
	return fmt.Sprintf(`
DO $$
DECLARE
	current_length int;
BEGIN
	SELECT atttypmod - 4
	INTO current_length
	FROM pg_attribute
	WHERE attrelid = 'chunks'::regclass
	  AND attname = 'vector';

	IF current_length IS DISTINCT FROM %d THEN
		RAISE NOTICE 'Altering vector size from %% to %d', current_length;
		EXECUTE 'ALTER TABLE chunks ALTER COLUMN vector TYPE vector(%d)';
	ELSE
		RAISE NOTICE 'Vector size already %d, skipping ALTER';
	END IF;
END$$;
`, dim, dim, dim, dim)
}

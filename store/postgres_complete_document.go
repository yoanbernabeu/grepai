package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Each referenced chunk ID must have at least one valid matching chunk: same
// project and file, non-null vector, and an ID equal to the reference or, when
// a non-empty prefix is given, prefix+"/"+reference. Both forms also accept
// the slash-normalized reference so references recorded by a Windows writer
// resolve to the slash-normalized IDs the writer stores — raw ones under the
// prefix, already-prefixed ones without adding the prefix a second time. The
// literal reference itself is never rewritten, so an empty prefix matches
// exact IDs only. The per-reference nested NOT EXISTS keeps a bad exact-ID
// row (wrong file or NULL vector) from rejecting a reference that a valid
// prefixed row satisfies.
const getCompleteDocumentSQL = `
SELECT d.path, d.hash, d.mod_time, d.mod_time_ns, d.chunk_ids
FROM documents d
WHERE d.project_id = $1
  AND d.path = $2
  AND cardinality(d.chunk_ids) > 0
  AND NOT EXISTS (
    SELECT 1
    FROM unnest(d.chunk_ids) AS referenced(id)
    WHERE NOT EXISTS (
      SELECT 1
      FROM chunks c
      WHERE c.project_id = d.project_id
        AND c.file_path = d.path
        AND c.vector IS NOT NULL
        AND (c.id = referenced.id OR ($3 <> '' AND (
          c.id = $3 || '/' || referenced.id
          OR c.id = replace(referenced.id, '\', '/')
          OR c.id = $3 || '/' || replace(referenced.id, '\', '/')
        )))
    )
  )`

func (s *PostgresStore) GetCompleteDocument(ctx context.Context, filePath string) (*Document, error) {
	return s.GetCompleteDocumentWithPrefix(ctx, filePath, "")
}

func (s *PostgresStore) GetCompleteDocumentWithPrefix(ctx context.Context, filePath, chunkIDPrefix string) (*Document, error) {
	var doc Document
	var modTime time.Time
	var modTimeNS *int64
	err := s.pool.QueryRow(ctx, getCompleteDocumentSQL, s.projectID, filePath, chunkIDPrefix).Scan(
		&doc.Path, &doc.Hash, &modTime, &modTimeNS, &doc.ChunkIDs,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get complete document: %w", err)
	}

	doc.ModTime, doc.HasExactModTime = decodeExactModTime(modTime, modTimeNS)
	doc.ChunkIDs = append([]string(nil), doc.ChunkIDs...)
	return &doc, nil
}

var _ CompleteDocumentSource = (*PostgresStore)(nil)
var _ PrefixedCompleteDocumentSource = (*PostgresStore)(nil)

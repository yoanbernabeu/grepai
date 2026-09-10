package store

import (
	"context"
	"fmt"
	"time"
)

const listDocumentMetadataSQL = `SELECT path, hash, COALESCE(cardinality(chunk_ids), 0) > 0, mod_time, mod_time_ns FROM documents WHERE project_id = $1`

func (s *PostgresStore) ListDocumentMetadata(ctx context.Context) ([]DocumentMetadata, error) {
	rows, err := s.pool.Query(ctx, listDocumentMetadataSQL, s.projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list document metadata: %w", err)
	}
	defer rows.Close()
	var metas []DocumentMetadata
	for rows.Next() {
		var m DocumentMetadata
		var legacy time.Time
		var nanos *int64
		if err := rows.Scan(&m.Path, &m.Hash, &m.HasChunks, &legacy, &nanos); err != nil {
			return nil, fmt.Errorf("failed to scan document metadata: %w", err)
		}
		m.ModTime, m.HasExactModTime = decodeExactModTime(legacy, nanos)
		metas = append(metas, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate document metadata: %w", err)
	}
	return metas, nil
}

var _ DocumentMetadataSource = (*PostgresStore)(nil)

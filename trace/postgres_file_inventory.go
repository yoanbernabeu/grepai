package trace

import (
	"context"
	"fmt"
	"sort"
)

// ListIndexedFiles returns a detached snapshot of every indexed file path
// for this store's project, read with a single project-scoped query
// against symbol_files. Files indexed with zero symbols (or zero
// references) are included because a symbol_files row exists per SaveFile
// call, so the result is complete file inventory rather than a
// symbol-derived enumeration.
//
// The returned slice is sorted for deterministic consumption and is
// caller-owned: later store mutations never alter it.
func (s *PostgresSymbolStore) ListIndexedFiles(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT path FROM symbol_files WHERE project_id=$1`, identityBytes(s.projectID))
	if err != nil {
		return nil, fmt.Errorf("failed to list indexed files: %w", err)
	}
	defer rows.Close()

	paths := []string{}
	for rows.Next() {
		var path []byte
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("failed to scan indexed file path: %w", err)
		}
		paths = append(paths, string(path))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read indexed files: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

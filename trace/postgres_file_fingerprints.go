package trace

import (
	"context"
	"fmt"
)

// ListFileFingerprints returns a detached snapshot of every indexed file
// mapped to its persisted content hash and extractor version, read with a
// single project-scoped query against symbol_files. Files indexed with zero
// symbols are included because symbol_files rows exist per SaveFile call.
//
// Legacy rows created before extractor signatures (or content hashes)
// existed store the empty string in the corresponding TEXT column. The
// snapshot preserves the per-file getter semantics for those rows: the file
// remains present (it is indexed), and the missing fingerprint fields report
// false so callers treat the file as not yet fingerprinted.
func (s *PostgresSymbolStore) ListFileFingerprints(ctx context.Context) (map[string]FileFingerprint, error) {
	rows, err := s.pool.Query(ctx, `SELECT path, content_hash, extractor_version FROM symbol_files WHERE project_id=$1`, identityBytes(s.projectID))
	if err != nil {
		return nil, fmt.Errorf("failed to list file fingerprints: %w", err)
	}
	defer rows.Close()

	fingerprints := map[string]FileFingerprint{}
	for rows.Next() {
		var path []byte
		var contentHash, extractorVersion string
		if err := rows.Scan(&path, &contentHash, &extractorVersion); err != nil {
			return nil, fmt.Errorf("failed to scan file fingerprint: %w", err)
		}
		fingerprint := FileFingerprint{}
		// Empty strings are the legacy "no fingerprint recorded"
		// marker on TEXT columns; the per-file getters report those as
		// unavailable and so must the snapshot.
		if contentHash != "" {
			fingerprint.ContentHash = contentHash
			fingerprint.HasContentHash = true
		}
		if extractorVersion != "" {
			fingerprint.ExtractorVersion = extractorVersion
			fingerprint.HasExtractorVersion = true
		}
		fingerprints[string(path)] = fingerprint
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file fingerprints: %w", err)
	}
	return fingerprints, nil
}

var _ FileFingerprintSource = (*PostgresSymbolStore)(nil)

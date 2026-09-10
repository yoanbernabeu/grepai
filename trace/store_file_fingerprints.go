package trace

import "context"

// ListFileFingerprints returns a detached snapshot, including files that
// produced zero symbols.
func (s *GOBSymbolStore) ListFileFingerprints(ctx context.Context) (map[string]FileFingerprint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fingerprints := make(map[string]FileFingerprint, len(s.fileIndex))
	for path := range s.fileIndex {
		fingerprint := FileFingerprint{}
		if hash, ok := s.fileContentHashes[path]; ok {
			fingerprint.ContentHash, fingerprint.HasContentHash = hash, true
		}
		if version, ok := s.fileExtractorVersions[path]; ok {
			fingerprint.ExtractorVersion, fingerprint.HasExtractorVersion = version, true
		}
		fingerprints[path] = fingerprint
	}
	return fingerprints, nil
}

var _ FileFingerprintSource = (*GOBSymbolStore)(nil)

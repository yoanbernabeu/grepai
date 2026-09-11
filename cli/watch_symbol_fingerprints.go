package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/yoanbernabeu/grepai/trace"
)

type watchSymbolFingerprintLookup interface {
	IsFileIndexed(string) bool
	GetFileContentHash(string) (string, bool)
	GetFileExtractorVersion(string) (string, bool)
}

type watchSymbolFingerprintSnapshot struct {
	fingerprints map[string]trace.FileFingerprint
}

func (s watchSymbolFingerprintSnapshot) IsFileIndexed(path string) bool {
	_, ok := s.fingerprints[path]
	return ok
}

func (s watchSymbolFingerprintSnapshot) GetFileContentHash(path string) (string, bool) {
	fingerprint, ok := s.fingerprints[path]
	return fingerprint.ContentHash, ok && fingerprint.HasContentHash
}

func (s watchSymbolFingerprintSnapshot) GetFileExtractorVersion(path string) (string, bool) {
	fingerprint, ok := s.fingerprints[path]
	return fingerprint.ExtractorVersion, ok && fingerprint.HasExtractorVersion
}

func loadWatchSymbolFingerprints(ctx context.Context, symbolStore trace.SymbolStore) (watchSymbolFingerprintLookup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fingerprints, err := trace.LoadFileFingerprints(ctx, symbolStore)
	if err == nil {
		return watchSymbolFingerprintSnapshot{fingerprints: fingerprints}, nil
	}
	if !errors.Is(err, trace.ErrFileFingerprintsUnsupported) {
		return nil, fmt.Errorf("failed to load symbol file fingerprints: %w", err)
	}

	// Legacy GOB stores retain their point-getter behavior. The snapshot
	// capability remains optional so SymbolStore does not gain a new required API.
	legacy, ok := symbolStore.(watchSymbolFingerprintLookup)
	if !ok {
		return nil, fmt.Errorf("symbol store does not support file fingerprints")
	}
	return legacy, nil
}

package trace

import (
	"context"
	"errors"
)

// ErrFileFingerprintsUnsupported reports that a SymbolStore cannot enumerate
// its indexed file inventory without materializing unrelated graph data.
var ErrFileFingerprintsUnsupported = errors.New("file fingerprint snapshots unsupported")

// FileFingerprint records the persisted extraction fingerprints for one
// indexed file. The bool fields distinguish "fingerprint absent" from
// "fingerprint present but empty" so callers can treat legacy indexes
// uniformly: when HasContentHash is false, ContentHash carries no meaning
// and the file must be treated as not yet fingerprinted.
//
// This is a value-only snapshot type; it never aliases store-internal state.
type FileFingerprint struct {
	ContentHash         string
	ExtractorVersion    string
	HasContentHash      bool
	HasExtractorVersion bool
}

// FileFingerprintSource is an optional capability interface implemented by
// symbol stores that can enumerate every indexed file's fingerprints in a
// single snapshot, without materializing symbols, references, or call
// graphs.
//
// Startup consumers use it to answer two questions in one query:
//
//   - which fingerprints does the index hold (per-file dedup decisions), and
//   - which indexed files are missing from the caller's current view
//     (orphan cleanup), including files that were indexed with zero
//     symbols and therefore appear in no symbol- or reference-derived
//     enumeration.
//
// Map membership means the file is indexed by the store. An empty map means
// the store indexes nothing.
type FileFingerprintSource interface {
	// ListFileFingerprints returns a detached snapshot of every indexed
	// file path mapped to its persisted fingerprints. The caller owns the
	// result; later store mutations must not alter it.
	ListFileFingerprints(ctx context.Context) (map[string]FileFingerprint, error)
}

// LoadFileFingerprints returns a snapshot of every indexed file's
// fingerprints from store, preferring the optional FileFingerprintSource
// capability when the store implements it.
//
// When the store implements FileFingerprintSource, the source result (or
// error) is returned verbatim — a failing source never silently falls back,
// because the fallback would report a partial snapshot as complete.
//
// Stores without the optional capability return
// ErrFileFingerprintsUnsupported. Callers that only need per-file cache
// decisions may retain their legacy point-getter behavior, but must not treat
// a partial graph-derived file list as complete inventory.
func LoadFileFingerprints(ctx context.Context, store SymbolStore) (map[string]FileFingerprint, error) {
	if source, ok := store.(FileFingerprintSource); ok {
		return source.ListFileFingerprints(ctx)
	}
	return nil, ErrFileFingerprintsUnsupported
}

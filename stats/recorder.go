package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Recorder appends stat entries to the local NDJSON stats file.
type Recorder struct {
	statsPath string
	lockPath  string
}

// NewRecorder creates a Recorder that writes to the stats file inside projectRoot.
func NewRecorder(projectRoot string) *Recorder {
	return &Recorder{
		statsPath: StatsPath(projectRoot),
		lockPath:  LockPath(projectRoot),
	}
}

// Record appends one entry to the stats NDJSON file.
// The write is protected by a file lock for cross-process safety.
// If the context is canceled or the write fails, the error is returned
// but the caller is expected to discard it (fire-and-forget pattern).
func (r *Recorder) Record(ctx context.Context, e Entry) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("stats: marshal entry: %w", err)
	}
	line = append(line, '\n')

	lockFile, err := os.OpenFile(r.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		// Proceed without locking rather than failing the caller.
		return r.appendLine(line)
	}
	defer lockFile.Close()

	if err := flockExclusive(lockFile); err != nil {
		return r.appendLine(line)
	}
	defer func() { _ = funlock(lockFile) }()

	return r.appendLine(line)
}

func (r *Recorder) appendLine(line []byte) error {
	if err := os.MkdirAll(filepath.Dir(r.statsPath), 0o755); err != nil {
		return fmt.Errorf("stats: create dir: %w", err)
	}
	f, err := os.OpenFile(r.statsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("stats: open file: %w", err)
	}
	defer f.Close()

	_, err = f.Write(line)
	return err
}

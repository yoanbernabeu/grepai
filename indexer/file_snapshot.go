package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"
)

const maxSnapshotAttempts = 3

func readFileSnapshot(path, relPath string) (*FileInfo, error) {
	return readFileSnapshotWithHooks(path, relPath, snapshotHooks{})
}

func readFileSnapshotWithHook(path, relPath string, afterRead func(attempt int) error) (*FileInfo, error) {
	return readFileSnapshotWithHooks(path, relPath, snapshotHooks{afterRead: afterRead})
}

type snapshotHooks struct {
	afterStat func(attempt int) error
	afterRead func(attempt int) error
}

func readFileSnapshotWithHooks(path, relPath string, hooks snapshotHooks) (*FileInfo, error) {
	for attempt := 0; attempt < maxSnapshotAttempts; attempt++ {
		preflight, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !preflight.Mode().IsRegular() || preflight.Size() > maxFileSize {
			return nil, nil
		}
		file, err := openSnapshotFile(path)
		if err != nil {
			return nil, err
		}
		before, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if !before.Mode().IsRegular() || before.Size() > maxFileSize {
			_ = file.Close()
			return nil, nil
		}
		if hooks.afterStat != nil {
			if err := hooks.afterStat(attempt); err != nil {
				_ = file.Close()
				return nil, err
			}
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxFileSize+1))
		after, statErr := file.Stat()
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if statErr != nil {
			return nil, statErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(content) > maxFileSize {
			return nil, nil
		}
		if hooks.afterRead != nil {
			if err := hooks.afterRead(attempt); err != nil {
				return nil, err
			}
		}
		current, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		stable := os.SameFile(after, current) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) && after.Size() == current.Size() && after.ModTime().Equal(current.ModTime()) && int64(len(content)) == after.Size()
		if !stable {
			continue
		}
		if !utf8.Valid(content) || containsNull(content) {
			return nil, nil
		}
		hash := sha256.Sum256(content)
		return &FileInfo{Path: relPath, Size: after.Size(), ModTime: after.ModTime().Unix(), ObservedModTime: after.ModTime(), Hash: hex.EncodeToString(hash[:]), Content: string(content)}, nil
	}
	return nil, fmt.Errorf("file changed during %d snapshot attempts", maxSnapshotAttempts)
}

func exactUnixNano(t time.Time) (int64, bool) {
	const minSecond, maxSecond = int64(-9223372037), int64(9223372036)
	if t.Unix() < minSecond || t.Unix() > maxSecond {
		return 0, false
	}
	ns := t.UnixNano()
	return ns, time.Unix(0, ns).Equal(t)
}

func hasExactTimestamp(t time.Time) bool { _, ok := exactUnixNano(t); return ok }

func persistedFileModTime(file FileInfo) (time.Time, bool) {
	if file.ObservedModTime.IsZero() {
		return time.Unix(file.ModTime, 0), false
	}
	return file.ObservedModTime, hasExactTimestamp(file.ObservedModTime)
}

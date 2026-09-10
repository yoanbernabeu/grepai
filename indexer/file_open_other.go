//go:build !unix && !windows

package indexer

import "os"

func openSnapshotFile(path string) (*os.File, error) { return os.Open(path) }

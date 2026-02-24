package cli

import (
	"context"
	"testing"

	"github.com/yoanbernabeu/grepai/store"
)

func TestLoadStatusFiles_SkipsWhenUIOff(t *testing.T) {
	called := 0

	files, err := loadStatusFiles(context.Background(), false, func(context.Context) ([]store.FileStats, error) {
		called++
		return []store.FileStats{{Path: "b"}, {Path: "a"}}, nil
	})
	if err != nil {
		t.Fatalf("loadStatusFiles() error: %v", err)
	}
	if called != 0 {
		t.Fatalf("list function called %d times, want 0", called)
	}
	if len(files) != 0 {
		t.Fatalf("files length = %d, want 0", len(files))
	}
}

func TestLoadStatusFiles_LoadsAndSortsWhenUIOn(t *testing.T) {
	files, err := loadStatusFiles(context.Background(), true, func(context.Context) ([]store.FileStats, error) {
		return []store.FileStats{
			{Path: "z-last.go"},
			{Path: "a-first.go"},
			{Path: "m-mid.go"},
		}, nil
	})
	if err != nil {
		t.Fatalf("loadStatusFiles() error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("files length = %d, want 3", len(files))
	}
	if files[0].Path != "a-first.go" || files[1].Path != "m-mid.go" || files[2].Path != "z-last.go" {
		t.Fatalf("files not sorted by path: %+v", files)
	}
}

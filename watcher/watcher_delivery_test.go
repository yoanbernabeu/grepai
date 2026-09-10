package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yoanbernabeu/grepai/indexer"
)

func TestFlushDeliversLargeBatchLosslessly(t *testing.T) {
	root := t.TempDir()
	w := newDirectoryTestWatcher(t, root)
	w.debounceMs = int(time.Hour / time.Millisecond)

	const count = 300
	created := filepath.Join(root, "generated")
	if err := os.Mkdir(created, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		if err := os.WriteFile(filepath.Join(created, fmt.Sprintf("file-%03d.go", i)), []byte("package generated"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.handleEvent(fsnotify.Event{Name: created, Op: fsnotify.Create}); err != nil {
		t.Fatal(err)
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	const sentinel = "__delivery_barrier__"
	for i := 0; i < cap(w.events); i++ {
		w.events <- FileEvent{Path: sentinel}
	}

	flushed := make(chan struct{})
	go func() {
		w.flush()
		close(flushed)
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	w.awaitPendingDrained(t, deadline.C)
	select {
	case <-flushed:
		t.Fatal("flush bypassed backpressure from a full event channel")
	default:
	}

	seen := make(map[string]int, count)
	sentinels := 0
	flushDone := false
	for len(seen) < count || sentinels < cap(w.events) || !flushDone {
		select {
		case event := <-w.Events():
			if event.Path == sentinel {
				sentinels++
			} else {
				seen[event.Path]++
			}
		case <-flushed:
			flushDone = true
			flushed = nil
		case <-deadline.C:
			t.Fatalf("received %d/%d events", len(seen), count)
		}
	}
	for path, deliveries := range seen {
		if deliveries != 1 {
			t.Fatalf("%s delivered %d times", path, deliveries)
		}
	}
}

func TestFlushStopsWhenWatcherClosesWithoutConsumer(t *testing.T) {
	root := t.TempDir()
	ignore, err := indexer.NewIgnoreMatcher(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher(root, ignore, 10)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		w.pending[fmt.Sprintf("file-%03d.go", i)] = FileEvent{Type: EventCreate, Path: fmt.Sprintf("file-%03d.go", i)}
	}
	for i := 0; i < cap(w.events); i++ {
		w.events <- FileEvent{Path: "barrier"}
	}

	flushed := make(chan struct{})
	go func() {
		w.flush()
		close(flushed)
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	w.awaitPendingDrained(t, deadline.C)
	select {
	case <-flushed:
		t.Fatal("flush bypassed backpressure from a full event channel")
	default:
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-flushed:
	case <-deadline.C:
		t.Fatal("flush remained blocked after Close")
	}
}

func (w *Watcher) awaitPendingDrained(t *testing.T, deadline <-chan time.Time) {
	t.Helper()
	for {
		w.pendingMu.Lock()
		drained := len(w.pending) == 0
		w.pendingMu.Unlock()
		if drained {
			return
		}
		select {
		case <-deadline:
			t.Fatal("flush did not take the pending batch")
		default:
			runtime.Gosched()
		}
	}
}

func TestDebouncePreservesDirectoryIdentity(t *testing.T) {
	tests := []struct {
		name string
		seq  []FileEvent
		want EventType
	}{
		{"rename then create", []FileEvent{{Type: EventRename, IsDir: true}, {Type: EventCreate}}, EventCreate},
		{"delete then create", []FileEvent{{Type: EventDelete, IsDir: true}, {Type: EventCreate}}, EventDelete},
		{"create then delete", []FileEvent{{Type: EventCreate}, {Type: EventDelete, IsDir: true}}, EventDelete},
		{"delete then modify", []FileEvent{{Type: EventDelete, IsDir: true}, {Type: EventModify}}, EventDelete},
		{"file delete then directory rename", []FileEvent{{Type: EventDelete}, {Type: EventRename, IsDir: true}}, EventDelete},
		{"file delete then directory create", []FileEvent{{Type: EventDelete}, {Type: EventCreate, IsDir: true}}, EventDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newDirectoryTestWatcher(t, t.TempDir())
			w.debounceMs = int(time.Hour / time.Millisecond)
			for _, event := range tt.seq {
				event.Path = filepath.Join("tree", "entry.go")
				w.debounceEvent(event)
			}
			if w.timer != nil {
				w.timer.Stop()
			}
			got := w.pending[filepath.Join("tree", "entry.go")]
			if got.Type != tt.want || !got.IsDir {
				t.Fatalf("coalesced event = %#v, want type %v with IsDir", got, tt.want)
			}
		})
	}
}

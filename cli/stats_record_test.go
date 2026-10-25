package cli

import (
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/stats"
)

// A CLI command exits right after recording its stats, so the entry must be
// on disk when recordStatsEntry returns.
func TestRecordStatsEntry_WrittenBeforeReturn(t *testing.T) {
	dir := t.TempDir()

	recordStatsEntry(dir, stats.Entry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		CommandType: stats.Search,
		OutputMode:  stats.Compact,
		ResultCount: 3,
	})

	entries, err := stats.ReadAll(stats.StatsPath(dir))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].CommandType != stats.Search {
		t.Errorf("CommandType = %q, want %q", entries[0].CommandType, stats.Search)
	}
}

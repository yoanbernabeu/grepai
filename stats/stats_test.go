package stats_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/stats"
)

// ---- helpers ----

func writeStatsFile(t *testing.T, dir string, lines []string) {
	t.Helper()
	path := filepath.Join(dir, stats.StatsFileName)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create stats file: %v", err)
	}
	defer f.Close()
	for _, l := range lines {
		f.WriteString(l + "\n")
	}
}

func entryJSON(t *testing.T, e stats.Entry) string {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	return string(b)
}

func makeEntry(ts, ct, mode string, results, out, grep int) stats.Entry {
	return stats.Entry{
		Timestamp:    ts,
		CommandType:  ct,
		OutputMode:   mode,
		ResultCount:  results,
		OutputTokens: out,
		GrepTokens:   grep,
	}
}

// ---- GrepEquivalentTokens ----

func TestGrepEquivalentTokens_NonZero(t *testing.T) {
	got := stats.GrepEquivalentTokens(4)
	want := 4 * stats.DefaultChunkTokens * stats.GrepExpansionFactor
	if got != want {
		t.Errorf("GrepEquivalentTokens(4) = %d, want %d", got, want)
	}
}

func TestGrepEquivalentTokens_Zero(t *testing.T) {
	got := stats.GrepEquivalentTokens(0)
	if got != stats.MinGrepTokens {
		t.Errorf("GrepEquivalentTokens(0) = %d, want %d", got, stats.MinGrepTokens)
	}
}

// ---- Round-trip Record → ReadAll ----

func TestRecordReadAll_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	rec := stats.NewRecorder(dir)
	ctx := context.Background()

	entries := []stats.Entry{
		makeEntry(time.Now().UTC().Format(time.RFC3339), stats.Search, stats.Compact, 5, 100, 2560),
		makeEntry(time.Now().UTC().Format(time.RFC3339), stats.TraceCallers, stats.Full, 3, 300, 1536),
	}
	for _, e := range entries {
		if err := rec.Record(ctx, e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, err := stats.ReadAll(stats.StatsPath(dir))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("ReadAll returned %d entries, want %d", len(got), len(entries))
	}
	for i, e := range got {
		if e.CommandType != entries[i].CommandType {
			t.Errorf("[%d] CommandType = %q, want %q", i, e.CommandType, entries[i].CommandType)
		}
		if e.OutputTokens != entries[i].OutputTokens {
			t.Errorf("[%d] OutputTokens = %d, want %d", i, e.OutputTokens, entries[i].OutputTokens)
		}
	}
}

// ---- ReadAll: file not found ----

func TestReadAll_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	entries, err := stats.ReadAll(filepath.Join(dir, stats.StatsFileName))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(entries))
	}
}

// ---- ReadAll: corrupted line is skipped ----

func TestReadAll_CorruptedLineSkipped(t *testing.T) {
	dir := t.TempDir()
	good := makeEntry("2026-02-22T10:00:00Z", stats.Search, stats.Full, 2, 80, 1024)
	writeStatsFile(t, dir, []string{
		entryJSON(t, good),
		"THIS IS NOT JSON",
		entryJSON(t, good),
	})

	entries, err := stats.ReadAll(stats.StatsPath(dir))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(entries))
	}
}

// ---- Summarize ----

func TestSummarize_Totals(t *testing.T) {
	entries := []stats.Entry{
		makeEntry("2026-02-22T10:00:00Z", stats.Search, stats.Compact, 5, 100, 2560),
		makeEntry("2026-02-22T11:00:00Z", stats.TraceCallers, stats.Full, 3, 300, 1536),
	}
	s := stats.Summarize(entries, "ollama")

	if s.TotalQueries != 2 {
		t.Errorf("TotalQueries = %d, want 2", s.TotalQueries)
	}
	if s.OutputTokens != 400 {
		t.Errorf("OutputTokens = %d, want 400", s.OutputTokens)
	}
	if s.GrepTokens != 4096 {
		t.Errorf("GrepTokens = %d, want 4096", s.GrepTokens)
	}
	if s.TokensSaved != 3696 {
		t.Errorf("TokensSaved = %d, want 3696", s.TokensSaved)
	}
}

func TestSummarize_SavingsPct(t *testing.T) {
	entries := []stats.Entry{
		makeEntry("2026-02-22T10:00:00Z", stats.Search, stats.Full, 4, 200, 2048),
	}
	s := stats.Summarize(entries, "openai")
	want := float64(2048-200) / float64(2048) * 100
	if s.SavingsPct < want-0.01 || s.SavingsPct > want+0.01 {
		t.Errorf("SavingsPct = %.2f, want ~%.2f", s.SavingsPct, want)
	}
}

func TestSummarize_NoGrepTokens_NoPanic(t *testing.T) {
	entries := []stats.Entry{
		makeEntry("2026-02-22T10:00:00Z", stats.Search, stats.Full, 0, 10, stats.MinGrepTokens),
	}
	s := stats.Summarize(entries, "ollama")
	if s.SavingsPct < 0 {
		t.Errorf("SavingsPct should not be negative")
	}
}

// ---- Summarize: cloud vs local provider ----

func TestSummarize_CloudProvider_CostSet(t *testing.T) {
	entries := []stats.Entry{
		makeEntry("2026-02-22T10:00:00Z", stats.Search, stats.Compact, 5, 100, 2560),
	}
	s := stats.Summarize(entries, "openai")
	if s.CostSavedUSD == nil {
		t.Fatal("expected CostSavedUSD to be non-nil for cloud provider")
	}
	if *s.CostSavedUSD <= 0 {
		t.Errorf("CostSavedUSD = %f, want > 0", *s.CostSavedUSD)
	}
}

func TestSummarize_LocalProvider_CostNil(t *testing.T) {
	entries := []stats.Entry{
		makeEntry("2026-02-22T10:00:00Z", stats.Search, stats.Full, 5, 100, 2560),
	}
	for _, provider := range []string{"ollama", "lmstudio"} {
		s := stats.Summarize(entries, provider)
		if s.CostSavedUSD != nil {
			t.Errorf("provider %q: expected CostSavedUSD nil, got %v", provider, *s.CostSavedUSD)
		}
	}
}

// ---- HistoryByDay ----

func TestHistoryByDay_Grouping(t *testing.T) {
	entries := []stats.Entry{
		makeEntry("2026-02-20T10:00:00Z", stats.Search, stats.Full, 2, 80, 1024),
		makeEntry("2026-02-21T09:00:00Z", stats.Search, stats.Full, 3, 120, 1536),
		makeEntry("2026-02-21T15:00:00Z", stats.TraceCallers, stats.Compact, 1, 40, 512),
		makeEntry("2026-02-22T08:00:00Z", stats.Search, stats.Compact, 5, 100, 2560),
	}
	days := stats.HistoryByDay(entries)

	if len(days) != 3 {
		t.Fatalf("expected 3 days, got %d", len(days))
	}
	// Sorted descending
	if days[0].Date != "2026-02-22" {
		t.Errorf("days[0].Date = %q, want 2026-02-22", days[0].Date)
	}
	if days[1].Date != "2026-02-21" {
		t.Errorf("days[1].Date = %q, want 2026-02-21", days[1].Date)
	}
	if days[1].QueryCount != 2 {
		t.Errorf("days[1].QueryCount = %d, want 2", days[1].QueryCount)
	}
}

func TestHistoryByDay_Empty(t *testing.T) {
	days := stats.HistoryByDay(nil)
	if len(days) != 0 {
		t.Errorf("expected empty slice for nil entries")
	}
}

// ---- IsCloudProvider ----

func TestIsCloudProvider(t *testing.T) {
	cloud := []string{"openai", "openrouter", "synthetic"}
	for _, p := range cloud {
		if !stats.IsCloudProvider(p) {
			t.Errorf("IsCloudProvider(%q) = false, want true", p)
		}
	}
	local := []string{"ollama", "lmstudio", ""}
	for _, p := range local {
		if stats.IsCloudProvider(p) {
			t.Errorf("IsCloudProvider(%q) = true, want false", p)
		}
	}
}

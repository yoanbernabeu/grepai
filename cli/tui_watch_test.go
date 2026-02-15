package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/store"
)

func TestWatchUIModelLifecycleTransition(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIPhaseMsg{current: 1})
	m = next.(watchUIModel)
	if m.currentStep != 1 {
		t.Fatalf("currentStep = %d, want 1", m.currentStep)
	}

	next, _ = m.Update(watchUIEmbedMsg{completed: 3, total: 10})
	m = next.(watchUIModel)
	if m.currentStep != 2 {
		t.Fatalf("currentStep = %d, want 2", m.currentStep)
	}

	next, _ = m.Update(watchUISymbolMsg{count: 42})
	m = next.(watchUIModel)
	if m.currentStep != 3 {
		t.Fatalf("currentStep = %d, want 3", m.currentStep)
	}
	if m.symbolCount != 42 {
		t.Fatalf("symbolCount = %d, want 42", m.symbolCount)
	}
}

func TestWatchUIModelLedgerLimit(t *testing.T) {
	m := newWatchUIModel(nil)

	total := watchLedgerLimit + 25
	for i := 0; i < total; i++ {
		next, _ := m.Update(watchUILedgerMsg{level: "info", text: fmt.Sprintf("event-%d", i)})
		m = next.(watchUIModel)
	}

	if len(m.events) != watchLedgerLimit {
		t.Fatalf("ledger size = %d, want %d", len(m.events), watchLedgerLimit)
	}
	if !strings.Contains(m.events[len(m.events)-1].text, fmt.Sprintf("event-%d", total-1)) {
		t.Fatalf("last ledger event mismatch: got %q", m.events[len(m.events)-1].text)
	}
}

func TestWatchUIModelPauseSkipsLedgerEvents(t *testing.T) {
	m := newWatchUIModel(nil)
	m.paused = true

	next, _ := m.Update(watchUILedgerMsg{level: "info", text: "event-1"})
	m = next.(watchUIModel)
	if len(m.events) != 0 {
		t.Fatalf("expected no events while paused, got %d", len(m.events))
	}

	m.paused = false
	next, _ = m.Update(watchUILedgerMsg{level: "info", text: "event-2"})
	m = next.(watchUIModel)
	if len(m.events) != 1 {
		t.Fatalf("expected one event after unpause, got %d", len(m.events))
	}
}

func TestRenderStatusSummaryIncludesWatcherInfo(t *testing.T) {
	cfg := config.DefaultConfig()
	stats := &store.IndexStats{
		TotalFiles:  12,
		TotalChunks: 34,
		IndexSize:   1024,
		LastUpdated: time.Date(2026, 2, 15, 9, 30, 0, 0, time.UTC),
	}
	watch := watcherRuntimeStatus{
		running: true,
		pid:     999,
		logFile: "/tmp/grepai-watch.log",
	}

	out := renderStatusSummary(cfg, stats, watch)
	if !strings.Contains(out, "Files indexed: 12") {
		t.Fatalf("summary missing files count: %q", out)
	}
	if !strings.Contains(out, "Watcher: running (PID 999)") {
		t.Fatalf("summary missing watcher status: %q", out)
	}
	if !strings.Contains(out, "/tmp/grepai-watch.log") {
		t.Fatalf("summary missing watcher log path: %q", out)
	}
}

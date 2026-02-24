package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

	if len(m.ledger.entries) != watchLedgerLimit {
		t.Fatalf("ledger size = %d, want %d", len(m.ledger.entries), watchLedgerLimit)
	}
	if !strings.Contains(m.ledger.entries[len(m.ledger.entries)-1].text, fmt.Sprintf("event-%d", total-1)) {
		t.Fatalf("last ledger event mismatch: got %q", m.ledger.entries[len(m.ledger.entries)-1].text)
	}
}

func TestWatchUIModelPauseBuffersLedgerEvents(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUILedgerMsg{level: "info", text: "event-1"})
	m = next.(watchUIModel)
	if len(m.ledger.entries) != 1 {
		t.Fatalf("expected initial event to be recorded, got %d", len(m.ledger.entries))
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(watchUIModel)
	if !m.ledger.paused {
		t.Fatal("expected paused mode after pressing 'p'")
	}

	next, _ = m.Update(watchUILedgerMsg{level: "info", text: "event-2"})
	m = next.(watchUIModel)
	// In the new implementation, events are added to entries even when paused.
	// The pause only affects auto-scrolling.
	// The test original logic: "expected buffered events while paused" -> logic for "buffer" was implicit in old code?
	// The old code had:
	// if m.paused { ... entries = m.events[:m.pausedAtIdx] ... } for VIEW.
	// But underlying m.events GREW.
	// So len(m.ledger.entries) SHOULD be 2.

	if len(m.ledger.entries) != 2 {
		t.Fatalf("expected events to be recorded while paused, got %d", len(m.ledger.entries))
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(watchUIModel)
	if m.ledger.paused {
		t.Fatal("expected unpaused mode after pressing 'p' again")
	}
}

func TestWatchUIModelScopeTracksReadyProjects(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIScopeMsg{totalProjects: 3})
	m = next.(watchUIModel)
	if m.totalProjects != 3 {
		t.Fatalf("totalProjects = %d, want 3", m.totalProjects)
	}

	next, _ = m.Update(watchUIReadyMsg{projectRoot: "/tmp/a"})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUIReadyMsg{projectRoot: "/tmp/b"})
	m = next.(watchUIModel)
	if m.readyProjects != 2 {
		t.Fatalf("readyProjects = %d, want 2", m.readyProjects)
	}
}

func TestWatchUIModelSnapshotRebaseAvoidsDoubleCount(t *testing.T) {
	m := newWatchUIModel(nil)
	root := "/tmp/main"

	next, _ := m.Update(watchUIContextMsg{projectRoot: root})
	m = next.(watchUIModel)

	next, _ = m.Update(watchUIStatsMsg{
		projectRoot: root,
		delta: watchStatsDelta{
			Snapshot:      true,
			FilesIndexed:  10,
			ChunksCreated: 100,
			SymbolsFound:  40,
		},
	})
	m = next.(watchUIModel)
	if m.filesIndexed != 10 || m.chunksCreated != 100 || m.symbolCount != 40 {
		t.Fatalf(
			"after baseline snapshot, files=%d chunks=%d symbols=%d, want 10/100/40",
			m.filesIndexed,
			m.chunksCreated,
			m.symbolCount,
		)
	}

	next, _ = m.Update(watchUIStatsMsg{
		projectRoot: root,
		delta: watchStatsDelta{
			FilesIndexed:  1,
			ChunksCreated: 5,
			SymbolsFound:  2,
		},
	})
	m = next.(watchUIModel)
	if m.filesIndexed != 11 || m.chunksCreated != 105 || m.symbolCount != 42 {
		t.Fatalf(
			"after incremental drift, files=%d chunks=%d symbols=%d, want 11/105/42",
			m.filesIndexed,
			m.chunksCreated,
			m.symbolCount,
		)
	}

	// Re-emitting snapshot after session restart should rebase, not double-count.
	next, _ = m.Update(watchUIStatsMsg{
		projectRoot: root,
		delta: watchStatsDelta{
			Snapshot:      true,
			FilesIndexed:  11,
			ChunksCreated: 105,
			SymbolsFound:  42,
		},
	})
	m = next.(watchUIModel)
	if m.filesIndexed != 11 || m.chunksCreated != 105 || m.symbolCount != 42 {
		t.Fatalf(
			"after restart snapshot rebase, files=%d chunks=%d symbols=%d, want 11/105/42",
			m.filesIndexed,
			m.chunksCreated,
			m.symbolCount,
		)
	}
}

func TestWatchUIModelSessionFocusCyclesWithTab(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIScopeMsg{totalProjects: 3})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/main", state: "queued", note: "primary"})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "queued", note: "linked"})
	m = next.(watchUIModel)

	if got := m.selectedSessionRoot(); got != "" {
		t.Fatalf("selected session = %q, want all", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	if got := m.selectedSessionRoot(); got != "/tmp/main" {
		t.Fatalf("selected session after 1st tab = %q, want /tmp/main", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	if got := m.selectedSessionRoot(); got != "/tmp/wt-a" {
		t.Fatalf("selected session after 2nd tab = %q, want /tmp/wt-a", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	if got := m.selectedSessionRoot(); got != "" {
		t.Fatalf("selected session after 3rd tab = %q, want all", got)
	}
}

func TestWatchUIModelFocusResetsWhenFocusedSessionRemoved(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIScopeMsg{totalProjects: 2})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/main", state: "running", note: "primary"})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "running", note: "linked"})
	m = next.(watchUIModel)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	if got := m.selectedSessionRoot(); got != "/tmp/wt-a" {
		t.Fatalf("selected session before remove = %q, want /tmp/wt-a", got)
	}

	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "removed", note: "deleted"})
	m = next.(watchUIModel)
	if got := m.selectedSessionRoot(); got != "" {
		t.Fatalf("selected session after remove = %q, want all", got)
	}
}

func TestWatchUIModelScopeUpdatesOnAddRemove(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIScopeMsg{totalProjects: 1})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/main", state: "running", note: "primary"})
	m = next.(watchUIModel)
	if m.totalProjects != 1 || m.readyProjects != 1 {
		t.Fatalf("unexpected initial scope=%d ready=%d", m.totalProjects, m.readyProjects)
	}

	next, _ = m.Update(watchUIScopeMsg{totalProjects: 2})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "running", note: "linked"})
	m = next.(watchUIModel)
	if m.totalProjects != 2 || m.readyProjects != 2 {
		t.Fatalf("unexpected expanded scope=%d ready=%d", m.totalProjects, m.readyProjects)
	}

	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "removed", note: "deleted"})
	m = next.(watchUIModel)
	if m.readyProjects != 1 {
		t.Fatalf("readyProjects after remove = %d, want 1", m.readyProjects)
	}

	next, _ = m.Update(watchUIScopeMsg{totalProjects: 1})
	m = next.(watchUIModel)
	if m.totalProjects != 1 || m.readyProjects != 1 {
		t.Fatalf("unexpected reduced scope=%d ready=%d", m.totalProjects, m.readyProjects)
	}
}

func TestWatchUIModelRemoveSessionClearsStatsContribution(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIContextMsg{projectRoot: "/tmp/main"})
	m = next.(watchUIModel)

	next, _ = m.Update(watchUIStatsMsg{
		projectRoot: "/tmp/main",
		delta: watchStatsDelta{
			Snapshot:      true,
			FilesIndexed:  10,
			ChunksCreated: 100,
			SymbolsFound:  40,
		},
	})
	m = next.(watchUIModel)

	next, _ = m.Update(watchUIStatsMsg{
		projectRoot: "/tmp/wt-a",
		delta: watchStatsDelta{
			Snapshot:      true,
			FilesIndexed:  3,
			ChunksCreated: 15,
			SymbolsFound:  6,
		},
	})
	m = next.(watchUIModel)

	if m.filesIndexed != 13 || m.chunksCreated != 115 || m.symbolCount != 46 {
		t.Fatalf("unexpected aggregate before remove: files=%d chunks=%d symbols=%d", m.filesIndexed, m.chunksCreated, m.symbolCount)
	}

	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "removed", note: "deleted"})
	m = next.(watchUIModel)

	if m.filesIndexed != 10 || m.chunksCreated != 100 || m.symbolCount != 40 {
		t.Fatalf("removed session contribution not cleared: files=%d chunks=%d symbols=%d", m.filesIndexed, m.chunksCreated, m.symbolCount)
	}
	if _, ok := m.snapshots["/tmp/wt-a"]; ok {
		t.Fatal("removed session snapshot should be cleared")
	}
	if _, ok := m.snapshotDrift["/tmp/wt-a"]; ok {
		t.Fatal("removed session drift should be cleared")
	}
}

func TestWatchUIModelRemovedSessionPrunesAfterTTL(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIScopeMsg{totalProjects: 2})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/main", state: "running", note: "primary"})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "removed", note: "deleted"})
	m = next.(watchUIModel)

	session, ok := m.sessions["/tmp/wt-a"]
	if !ok {
		t.Fatal("removed session should remain visible before TTL")
	}

	next, _ = m.Update(watchUIPruneMsg{at: session.removedAt.Add(removedSessionTTL - time.Millisecond)})
	m = next.(watchUIModel)
	if _, exists := m.sessions["/tmp/wt-a"]; !exists {
		t.Fatal("removed session pruned before TTL elapsed")
	}

	next, _ = m.Update(watchUIPruneMsg{at: session.removedAt.Add(removedSessionTTL + time.Millisecond)})
	m = next.(watchUIModel)
	if _, exists := m.sessions["/tmp/wt-a"]; exists {
		t.Fatal("removed session should be pruned after TTL")
	}
}

func TestWatchUIModelLedgerPanelFiltersByFocusedSession(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIContextMsg{projectRoot: "/tmp/main"})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUIScopeMsg{totalProjects: 3})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/main", state: "running", note: "primary"})
	m = next.(watchUIModel)
	next, _ = m.Update(watchUISessionMsg{projectRoot: "/tmp/wt-a", state: "running", note: "linked"})
	m = next.(watchUIModel)

	next, _ = m.Update(watchUILedgerMsg{source: "/tmp/wt-a", level: "info", text: "linked early event"})
	m = next.(watchUIModel)
	for i := 0; i < 5; i++ {
		next, _ = m.Update(watchUILedgerMsg{source: "/tmp/main", level: "info", text: fmt.Sprintf("main event %d", i)})
		m = next.(watchUIModel)
	}
	next, _ = m.Update(watchUILedgerMsg{source: "/tmp/wt-a", level: "info", text: "linked recent event"})
	m = next.(watchUIModel)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	if got := m.selectedSessionRoot(); got != "/tmp/wt-a" {
		t.Fatalf("focused session = %q, want /tmp/wt-a", got)
	}

	// Simulate resizing so viewports are initialized with non-zero size
	m.width = 100
	m.height = 40
	m.recalculateLayout()

	_ = m.renderLedgerPanel(120, 6)
	filtered := m.ledger.renderContent()
	if !strings.Contains(filtered, "linked early event") {
		t.Fatalf("filtered ledger missing linked entry: %q", filtered)
	}
	if !strings.Contains(filtered, "linked recent event") {
		t.Fatalf("filtered ledger missing linked recent entry: %q", filtered)
	}
	if strings.Contains(filtered, "main event") {
		t.Fatalf("filtered ledger should exclude focused-out session events: %q", filtered)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(watchUIModel)
	if got := m.selectedSessionRoot(); got != "" {
		t.Fatalf("focused session after cycling to all = %q, want empty", got)
	}

	unfiltered := m.ledger.renderContent()
	if !strings.Contains(unfiltered, "main event") {
		t.Fatalf("all-session ledger should include main events: %q", unfiltered)
	}
}

func TestWatchUIModelRenderProgressPanelDoesNotShowSkipped(t *testing.T) {
	m := newWatchUIModel(nil)
	m.width = 100
	m.height = 40
	m.recalculateLayout()

	panel := m.renderProgressPanel(80, 12)
	if strings.Contains(panel, "skipped=") {
		t.Fatalf("progress panel should not show skipped count: %q", panel)
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

func TestWatchUILogLevel(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{line: "Warning: failed to persist index", want: "error"},
		{line: "warning: retry scheduled", want: "warn"},
		{line: "watch session started", want: "info"},
	}

	for _, tc := range tests {
		got := watchUILogLevel(tc.line)
		if got != tc.want {
			t.Fatalf("watchUILogLevel(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

func TestWatchUILogSourceResolver(t *testing.T) {
	mainRoot := "/tmp/main"
	linkedRoot := "/tmp/wt-a"
	register, resolve := newWatchUILogSourceResolver(mainRoot)

	if got := resolve("watching project: /tmp/main"); got != mainRoot {
		t.Fatalf("resolve(main) = %q, want %q", got, mainRoot)
	}

	register(linkedRoot)
	if got := resolve("failed to load config for /tmp/wt-a"); got != linkedRoot {
		t.Fatalf("resolve(linked) = %q, want %q", got, linkedRoot)
	}

	if got := resolve("generic warning without root"); got != watchUILogSystem {
		t.Fatalf("resolve(fallback) = %q, want %q", got, watchUILogSystem)
	}
}

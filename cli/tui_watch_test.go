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

	if len(m.events) != watchLedgerLimit {
		t.Fatalf("ledger size = %d, want %d", len(m.events), watchLedgerLimit)
	}
	if !strings.Contains(m.events[len(m.events)-1].text, fmt.Sprintf("event-%d", total-1)) {
		t.Fatalf("last ledger event mismatch: got %q", m.events[len(m.events)-1].text)
	}
}

func TestWatchUIModelPauseBuffersLedgerEvents(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUILedgerMsg{level: "info", text: "event-1"})
	m = next.(watchUIModel)
	if len(m.events) != 1 {
		t.Fatalf("expected initial event to be recorded, got %d", len(m.events))
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(watchUIModel)
	if !m.paused {
		t.Fatal("expected paused mode after pressing 'p'")
	}

	next, _ = m.Update(watchUILedgerMsg{level: "info", text: "event-2"})
	m = next.(watchUIModel)
	if len(m.events) != 2 {
		t.Fatalf("expected buffered events while paused, got %d", len(m.events))
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(watchUIModel)
	if m.paused {
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

	rendered := m.renderLedgerPanel(120, 6) // maxRows=3 to force truncation behavior
	if strings.Contains(rendered, "main event 4") {
		t.Fatalf("ledger panel should hide non-focused session events: %q", rendered)
	}
	if !strings.Contains(rendered, "linked early event") {
		t.Fatalf("ledger panel should retain focused session history when non-focused events are noisy: %q", rendered)
	}
	if !strings.Contains(rendered, "linked recent event") {
		t.Fatalf("ledger panel should include focused session event: %q", rendered)
	}
}

func TestWatchUIModelSessionPanelKeepsFocusedSessionVisible(t *testing.T) {
	m := newWatchUIModel(nil)

	next, _ := m.Update(watchUIScopeMsg{totalProjects: 5})
	m = next.(watchUIModel)
	for _, root := range []string{
		"/tmp/main",
		"/tmp/wt-a",
		"/tmp/wt-b",
		"/tmp/wt-c",
	} {
		next, _ = m.Update(watchUISessionMsg{projectRoot: root, state: "running", note: "steady"})
		m = next.(watchUIModel)
	}

	for i := 0; i < 4; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(watchUIModel)
	}
	if got := m.selectedSessionRoot(); got != "/tmp/wt-c" {
		t.Fatalf("selected session = %q, want /tmp/wt-c", got)
	}

	rendered := m.renderSessionPanel(50, 7)
	if !strings.Contains(rendered, "tmp/wt-c") {
		t.Fatalf("focused session should stay visible in small panel: %q", rendered)
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

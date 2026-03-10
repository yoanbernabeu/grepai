package cli

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yoanbernabeu/grepai/stats"
)

var testEntries = []stats.Entry{
	{Timestamp: "2026-01-15T10:00:00Z", CommandType: "search", OutputMode: "compact", ResultCount: 3, OutputTokens: 100, GrepTokens: 500},
	{Timestamp: "2026-01-15T11:00:00Z", CommandType: "trace-callers", OutputMode: "full", ResultCount: 2, OutputTokens: 80, GrepTokens: 300},
	{Timestamp: "2026-01-16T09:00:00Z", CommandType: "search", OutputMode: "toon", ResultCount: 1, OutputTokens: 50, GrepTokens: 150},
}

// TestStatsUIModelInit vérifie l'état initial du modèle.
func TestStatsUIModelInit(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")

	if m.view != statsViewSummary {
		t.Errorf("view: got %v, want statsViewSummary", m.view)
	}
	if m.selected != 0 {
		t.Errorf("selected: got %d, want 0", m.selected)
	}
	// 3 entries réparties sur 2 jours distincts
	if len(m.days) != 2 {
		t.Errorf("days: got %d, want 2", len(m.days))
	}
}

// TestStatsViewSummaryEmpty vérifie l'affichage avec des entrées vides.
func TestStatsViewSummaryEmpty(t *testing.T) {
	summary := stats.Summarize(nil, "ollama")
	m := newStatsUIModel(summary, nil, "ollama")
	out := stripANSI(m.View())

	if !strings.Contains(out, "0") {
		t.Errorf("expected '0' queries in empty summary, got:\n%s", out)
	}
	if !strings.Contains(out, "0.0%") {
		t.Errorf("expected '0.0%%' savings in empty summary, got:\n%s", out)
	}
}

// TestStatsViewSummaryCloudProvider vérifie l'affichage du coût pour un provider cloud.
func TestStatsViewSummaryCloudProvider(t *testing.T) {
	summary := stats.Summarize(testEntries, "openai")
	m := newStatsUIModel(summary, testEntries, "openai")
	out := stripANSI(m.View())

	if !strings.Contains(out, "Cost saved") {
		t.Errorf("expected 'Cost saved' for cloud provider, got:\n%s", out)
	}
}

// TestStatsViewSummaryLocalProvider vérifie l'absence du coût pour un provider local.
func TestStatsViewSummaryLocalProvider(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	out := stripANSI(m.View())

	if strings.Contains(out, "Cost saved") {
		t.Errorf("unexpected 'Cost saved' for local provider, got:\n%s", out)
	}
}

// TestStatsViewSummaryBreakdown vérifie que le breakdown par commande et mode est affiché.
func TestStatsViewSummaryBreakdown(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	out := stripANSI(m.View())

	if !strings.Contains(out, "search") {
		t.Errorf("expected 'search' in command breakdown, got:\n%s", out)
	}
	if !strings.Contains(out, "compact") {
		t.Errorf("expected 'compact' in mode breakdown, got:\n%s", out)
	}
}

// TestStatsViewHistoryHeader vérifie l'en-tête de la vue historique.
func TestStatsViewHistoryHeader(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory
	out := stripANSI(m.View())

	if !strings.Contains(out, "History — Token Savings") {
		t.Errorf("expected history header, got:\n%s", out)
	}
}

// TestStatsViewHistoryLines vérifie que les lignes par date sont affichées.
func TestStatsViewHistoryLines(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory
	out := stripANSI(m.View())

	if !strings.Contains(out, "2026-01-16") {
		t.Errorf("expected date '2026-01-16', got:\n%s", out)
	}
	if !strings.Contains(out, "2026-01-15") {
		t.Errorf("expected date '2026-01-15', got:\n%s", out)
	}
}

// TestStatsViewHistorySelectedMarker vérifie que la ligne sélectionnée a le marqueur ">".
func TestStatsViewHistorySelectedMarker(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory
	m.selected = 0
	out := stripANSI(m.View())

	// La première ligne de données doit avoir ">"
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, ">") && strings.Contains(line, "2026-01") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '>' marker on selected row, got:\n%s", out)
	}
}

// TestStatsViewHistoryPagination vérifie le message de pagination quand > maxVisible jours.
func TestStatsViewHistoryPagination(t *testing.T) {
	// Créer suffisamment d'entrées pour dépasser maxVisible (15 par défaut)
	var manyEntries []stats.Entry
	for i := 1; i <= 20; i++ {
		manyEntries = append(manyEntries, stats.Entry{
			Timestamp:    "2026-01-" + pad2(i) + "T10:00:00Z",
			CommandType:  "search",
			OutputMode:   "compact",
			ResultCount:  1,
			OutputTokens: 50,
			GrepTokens:   200,
		})
	}
	summary := stats.Summarize(manyEntries, "ollama")
	m := newStatsUIModel(summary, manyEntries, "ollama")
	m.view = statsViewHistory
	// Sans height défini, maxVisible = 15, donc 20 > 15 → pagination
	out := stripANSI(m.View())

	if !strings.Contains(out, "showing") {
		t.Errorf("expected pagination message with 'showing', got:\n%s", out)
	}
	if !strings.Contains(out, "of 20 days") {
		t.Errorf("expected 'of 20 days' in pagination, got:\n%s", out)
	}
}

func pad2(n int) string {
	return fmt.Sprintf("%02d", n)
}

// TestStatsUpdateTabToHistory vérifie la navigation h/tab depuis summary → history.
func TestStatsUpdateTabToHistory(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m2 := newModel.(statsUIModel)

	if m2.view != statsViewHistory {
		t.Errorf("expected statsViewHistory after 'h', got %v", m2.view)
	}
	if m2.selected != 0 {
		t.Errorf("expected selected=0 after switch to history, got %d", m2.selected)
	}
}

// TestStatsUpdateTabBackToSummary vérifie le retour en summary avec tab depuis history.
func TestStatsUpdateTabBackToSummary(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := newModel.(statsUIModel)

	if m2.view != statsViewSummary {
		t.Errorf("expected statsViewSummary after tab from history, got %v", m2.view)
	}
}

// TestStatsUpdateEscBackToSummary vérifie le retour en summary avec esc.
func TestStatsUpdateEscBackToSummary(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := newModel.(statsUIModel)

	if m2.view != statsViewSummary {
		t.Errorf("expected statsViewSummary after esc, got %v", m2.view)
	}
}

// TestStatsUpdateNavigationDown vérifie que down/j incrémente selected.
func TestStatsUpdateNavigationDown(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory
	m.selected = 0

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := newModel.(statsUIModel)

	if m2.selected != 1 {
		t.Errorf("expected selected=1 after down, got %d", m2.selected)
	}
}

// TestStatsUpdateNavigationUp vérifie que up/k décrémente selected.
func TestStatsUpdateNavigationUp(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory
	m.selected = 1

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m2 := newModel.(statsUIModel)

	if m2.selected != 0 {
		t.Errorf("expected selected=0 after up, got %d", m2.selected)
	}
}

// TestStatsUpdateNavigationBounds vérifie que les bornes sont respectées.
func TestStatsUpdateNavigationBounds(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")
	m.view = statsViewHistory
	m.selected = 0

	// Ne peut pas aller en dessous de 0
	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := newModel.(statsUIModel)
	if m2.selected != 0 {
		t.Errorf("expected selected=0 at lower bound, got %d", m2.selected)
	}

	// Ne peut pas dépasser len(days)-1
	m.selected = len(m.days) - 1
	newModel, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3 := newModel.(statsUIModel)
	if m3.selected != len(m.days)-1 {
		t.Errorf("expected selected=%d at upper bound, got %d", len(m.days)-1, m3.selected)
	}
}

// TestStatsUpdateQuit vérifie que q retourne tea.Quit.
func TestStatsUpdateQuit(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected a command after 'q', got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

// TestStatsUpdateWindowSize vérifie que WindowSizeMsg met à jour width/height.
func TestStatsUpdateWindowSize(t *testing.T) {
	summary := stats.Summarize(testEntries, "ollama")
	m := newStatsUIModel(summary, testEntries, "ollama")

	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := newModel.(statsUIModel)

	if m2.width != 120 {
		t.Errorf("expected width=120, got %d", m2.width)
	}
	if m2.height != 40 {
		t.Errorf("expected height=40, got %d", m2.height)
	}
}

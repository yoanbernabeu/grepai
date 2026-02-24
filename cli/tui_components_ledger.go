package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type ledgerEntry struct {
	source string
	at     time.Time
	level  string
	text   string
}

type ledgerModel struct {
	viewport     viewport.Model
	entries      []ledgerEntry
	width        int
	height       int
	theme        tuiTheme
	paused       bool
	autoScroll   bool
	sourceFilter string
}

func newLedgerModel(theme tuiTheme) ledgerModel {
	vp := viewport.New(0, 0)
	vp.YPosition = 0

	return ledgerModel{
		viewport:   vp,
		entries:    make([]ledgerEntry, 0, 1000),
		theme:      theme,
		autoScroll: true,
	}
}

func (m ledgerModel) Init() tea.Cmd {
	return nil
}

func (m ledgerModel) Update(msg tea.Msg) (ledgerModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.autoScroll = false
		case "down", "j":
			if m.viewport.AtBottom() {
				m.autoScroll = true
			}
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)

	// Re-enable autoscroll if we hit the bottom
	if m.viewport.AtBottom() {
		m.autoScroll = true
	}

	return m, cmd
}

func (m *ledgerModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.Width = w
	m.viewport.Height = h
	// Re-render content with new width (if we were wrapping)
	// For now, just set content again
	m.updateContent()
}

const watchLedgerLimit = 300

func (m *ledgerModel) addEntry(e ledgerEntry) {
	m.entries = append(m.entries, e)
	if len(m.entries) > watchLedgerLimit {
		m.entries = m.entries[len(m.entries)-watchLedgerLimit:]
	}
	m.updateContent()
}

func (m *ledgerModel) setSourceFilter(source string) {
	if m.sourceFilter == source {
		return
	}
	m.sourceFilter = source
	m.updateContent()
}

func (m *ledgerModel) updateContent() {
	content := m.renderContent()
	m.viewport.SetContent(content)
	if m.autoScroll && !m.paused {
		m.viewport.GotoBottom()
	}
}

func (m *ledgerModel) togglePause() {
	m.paused = !m.paused
}

func (m ledgerModel) renderContent() string {
	var b strings.Builder
	for _, ev := range m.entries {
		if m.sourceFilter != "" && ev.source != m.sourceFilter {
			continue
		}

		levelStyle := m.theme.info
		switch ev.level {
		case "warn":
			levelStyle = m.theme.warn
		case "error":
			levelStyle = m.theme.danger
		case "ok":
			levelStyle = m.theme.ok
		}

		ts := ev.at.Format("15:04:05")
		// Using simple truncation for source
		sourceLabel := ev.source
		if len(sourceLabel) > 20 {
			sourceLabel = "..." + sourceLabel[len(sourceLabel)-17:]
		}

		// We format the line. viewport handles horizontal scrolling if lines are too long,
		// or wrapping if enabled. Default is no wrap.
		line := fmt.Sprintf("%s %s %s %s",
			m.theme.muted.Render(ts),
			levelStyle.Render(strings.ToUpper(ev.level)),
			m.theme.muted.Render(sourceLabel),
			ev.text)

		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m ledgerModel) View() string {
	return m.viewport.View()
}

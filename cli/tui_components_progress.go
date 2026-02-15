package cli

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type progressModel struct {
	scanBar  progress.Model
	embedBar progress.Model

	scanCurrent int
	scanTotal   int

	embedCurrent int
	embedTotal   int

	width int
	theme tuiTheme
}

func newProgressModel(theme tuiTheme) progressModel {
	scan := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)
	embed := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)

	return progressModel{
		scanBar:  scan,
		embedBar: embed,
		theme:    theme,
	}
}

func (m progressModel) Init() tea.Cmd {
	return nil
}

func (m progressModel) Update(msg tea.Msg) (progressModel, tea.Cmd) {
	// If we wanted animated progress bars we'd update them here.
	// For now they are static based on values.
	return m, nil
}

func (m *progressModel) setSize(w int) {
	m.width = w
	// Calculate available width for the bar itself
	// Label "Scan ": 5 chars
	// Status " 100/100": approx 15 chars
	// Padding: 2
	available := w - 25
	if available < 10 {
		available = 10
	}
	m.scanBar.Width = available
	m.embedBar.Width = available
}

func (m *progressModel) setScanProgress(current, total int) {
	m.scanCurrent = current
	m.scanTotal = total
}

func (m *progressModel) setEmbedProgress(current, total int) {
	m.embedCurrent = current
	m.embedTotal = total
}

func (m progressModel) View() string {
	scanPct := 0.0
	if m.scanTotal > 0 {
		scanPct = float64(m.scanCurrent) / float64(m.scanTotal)
	}

	embedPct := 0.0
	if m.embedTotal > 0 {
		embedPct = float64(m.embedCurrent) / float64(m.embedTotal)
	}

	scanStatus := fmt.Sprintf("%d/%d", m.scanCurrent, m.scanTotal)
	embedStatus := fmt.Sprintf("%d/%d", m.embedCurrent, m.embedTotal)

	scanView := lipgloss.JoinHorizontal(lipgloss.Center,
		m.theme.text.Width(6).Render("Scan"),
		m.scanBar.ViewAs(scanPct),
		m.theme.muted.Render(" "+scanStatus),
	)

	embedView := lipgloss.JoinHorizontal(lipgloss.Center,
		m.theme.text.Width(6).Render("Embed"),
		m.embedBar.ViewAs(embedPct),
		m.theme.muted.Render(" "+embedStatus),
	)

	return lipgloss.JoinVertical(lipgloss.Left, scanView, embedView)
}

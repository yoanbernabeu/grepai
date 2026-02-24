package cli

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type progressModel struct {
	scanBar    progress.Model
	embedBar   progress.Model
	rpgNodeBar progress.Model
	rpgEdgeBar progress.Model

	scanCurrent int
	scanTotal   int

	embedCurrent int
	embedTotal   int

	rpgNodeStep    string
	rpgNodeCurrent int
	rpgNodeTotal   int

	rpgEdgeStep    string
	rpgEdgeCurrent int
	rpgEdgeTotal   int

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

		rpgNodeBar: progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage()),
		rpgEdgeBar: progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage()),
		theme:      theme,
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
	m.rpgNodeBar.Width = available
	m.rpgEdgeBar.Width = available
}

func (m *progressModel) setScanProgress(current, total int) {
	m.scanCurrent = current
	m.scanTotal = total
}

func (m *progressModel) setEmbedProgress(current, total int) {
	m.embedCurrent = current
	m.embedTotal = total
}

func (m *progressModel) setRPGNodeProgress(step string, current, total int) {
	m.rpgNodeStep = step
	m.rpgNodeCurrent = current
	m.rpgNodeTotal = total
}

func (m *progressModel) setRPGEdgeProgress(step string, current, total int) {
	m.rpgEdgeStep = step
	m.rpgEdgeCurrent = current
	m.rpgEdgeTotal = total
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

	rpgNodePct := 0.0
	if m.rpgNodeTotal > 0 {
		rpgNodePct = float64(m.rpgNodeCurrent) / float64(m.rpgNodeTotal)
	}
	rpgNodeStatus := fmt.Sprintf("%s %d/%d", m.rpgNodeStep, m.rpgNodeCurrent, m.rpgNodeTotal)

	rpgEdgePct := 0.0
	if m.rpgEdgeTotal > 0 {
		rpgEdgePct = float64(m.rpgEdgeCurrent) / float64(m.rpgEdgeTotal)
	}
	rpgEdgeStatus := fmt.Sprintf("%s %d/%d", m.rpgEdgeStep, m.rpgEdgeCurrent, m.rpgEdgeTotal)

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

	if m.rpgNodeTotal == 0 && m.rpgEdgeTotal == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, scanView, embedView)
	}

	rpgNodeView := lipgloss.JoinHorizontal(lipgloss.Center,
		m.theme.text.Width(6).Render("Nodes"),
		m.rpgNodeBar.ViewAs(rpgNodePct),
		m.theme.muted.Render(" "+rpgNodeStatus),
	)

	rpgEdgeView := lipgloss.JoinHorizontal(lipgloss.Center,
		m.theme.text.Width(6).Render("Edges"),
		m.rpgEdgeBar.ViewAs(rpgEdgePct),
		m.theme.muted.Render(" "+rpgEdgeStatus),
	)

	return lipgloss.JoinVertical(lipgloss.Left, scanView, embedView, rpgNodeView, rpgEdgeView)
}

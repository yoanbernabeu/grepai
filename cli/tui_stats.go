package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yoanbernabeu/grepai/stats"
)

type statsView int

const (
	statsViewSummary statsView = iota
	statsViewHistory
)

type statsUIModel struct {
	theme    tuiTheme
	summary  stats.Summary
	entries  []stats.Entry
	days     []stats.DaySummary
	provider string
	view     statsView
	selected int
	width    int
	height   int
}

func newStatsUIModel(summary stats.Summary, entries []stats.Entry, provider string) statsUIModel {
	days := stats.HistoryByDay(entries)
	return statsUIModel{
		theme:    newTUITheme(),
		summary:  summary,
		entries:  entries,
		days:     days,
		provider: provider,
		view:     statsViewSummary,
		selected: 0,
	}
}

func (m statsUIModel) Init() tea.Cmd { return nil }

func (m statsUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "h", "tab":
			if m.view == statsViewSummary {
				m.view = statsViewHistory
				m.selected = 0
			} else {
				m.view = statsViewSummary
			}

		case "esc":
			if m.view == statsViewHistory {
				m.view = statsViewSummary
			}

		case "up", "k":
			if m.view == statsViewHistory && m.selected > 0 {
				m.selected--
			}

		case "down", "j":
			if m.view == statsViewHistory && m.selected < len(m.days)-1 {
				m.selected++
			}
		}
	}

	return m, nil
}

func (m statsUIModel) View() string {
	switch m.view {
	case statsViewHistory:
		return m.viewHistory()
	default:
		return m.viewSummary()
	}
}

func (m statsUIModel) viewSummary() string {
	t := m.theme
	var sb strings.Builder

	sb.WriteString(t.title.Render("Token Savings — grepai"))
	sb.WriteString("\n\n")

	label := t.muted.Width(20)
	value := t.text

	sb.WriteString(label.Render("Queries"))
	sb.WriteString(value.Render(fmt.Sprintf("%d", m.summary.TotalQueries)))
	sb.WriteString("\n")

	sb.WriteString(label.Render("Tokens saved"))
	sb.WriteString(value.Render(formatInt(m.summary.TokensSaved)))
	sb.WriteString("\n")

	sb.WriteString(label.Render("Savings"))
	sb.WriteString(t.ok.Render(fmt.Sprintf("%.1f%%", m.summary.SavingsPct)))
	sb.WriteString("\n")

	if m.summary.CostSavedUSD != nil {
		sb.WriteString(label.Render("Cost saved"))
		sb.WriteString(value.Render(fmt.Sprintf("~$%.4f", *m.summary.CostSavedUSD)))
		sb.WriteString(t.muted.Render("  (cloud provider)"))
		sb.WriteString("\n")
	}

	// Command breakdown
	sb.WriteString("\n")
	cmdParts := []string{}
	for _, k := range []string{"search", "trace-callers", "trace-callees", "trace-graph"} {
		if v := m.summary.ByCommandType[k]; v > 0 {
			cmdParts = append(cmdParts, fmt.Sprintf("%s %d", k, v))
		}
	}
	if len(cmdParts) > 0 {
		sb.WriteString(t.muted.Render("By command:  " + strings.Join(cmdParts, " · ")))
		sb.WriteString("\n")
	}

	modeParts := []string{}
	for _, k := range []string{"full", "compact", "toon"} {
		if v := m.summary.ByOutputMode[k]; v > 0 {
			modeParts = append(modeParts, fmt.Sprintf("%s %d", k, v))
		}
	}
	if len(modeParts) > 0 {
		sb.WriteString(t.muted.Render("By mode:     " + strings.Join(modeParts, " · ")))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(t.help.Render("[h/tab] history  [q] quit"))

	return t.panel.Render(sb.String())
}

func (m statsUIModel) viewHistory() string {
	t := m.theme
	var sb strings.Builder

	sb.WriteString(t.title.Render("History — Token Savings"))
	sb.WriteString("\n\n")

	colDate := 16
	colQueries := 10
	colSaved := 16
	colPct := 10

	header := t.muted.Render(
		fmt.Sprintf("%-*s %-*s %-*s %-*s",
			colDate, "Date",
			colQueries, "Queries",
			colSaved, "Tokens saved",
			colPct, "Savings",
		),
	)
	sep := t.muted.Render(
		fmt.Sprintf("%-*s %-*s %-*s %-*s",
			colDate, "────────────────",
			colQueries, "─────────",
			colSaved, "───────────────",
			colPct, "────────",
		),
	)
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(sep)
	sb.WriteString("\n")

	maxVisible := 15
	if m.height > 0 {
		maxVisible = m.height - 12
	}
	if maxVisible < 5 {
		maxVisible = 5
	}

	start := 0
	if m.selected >= maxVisible {
		start = m.selected - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.days) {
		end = len(m.days)
	}

	for i := start; i < end; i++ {
		d := m.days[i]
		pct := 0.0
		if d.GrepTokens > 0 {
			pct = float64(d.TokensSaved) / float64(d.GrepTokens) * 100
		}
		row := fmt.Sprintf("%-*s %-*d %-*s %-*.1f%%",
			colDate, d.Date,
			colQueries, d.QueryCount,
			colSaved, formatInt(d.TokensSaved),
			colPct-1, pct,
		)
		if i == m.selected {
			sb.WriteString(t.highlight.Render("> " + row))
		} else {
			sb.WriteString(t.text.Render("  " + row))
		}
		sb.WriteString("\n")
	}

	if len(m.days) > maxVisible {
		sb.WriteString("\n")
		sb.WriteString(t.muted.Render(fmt.Sprintf("... showing %d-%d of %d days", start+1, end, len(m.days))))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(t.help.Render("[↑/↓] navigate  [esc/tab] back  [q] quit"))

	return t.panel.Render(sb.String())
}

func runStatsUI(summary stats.Summary, entries []stats.Entry, provider string) error {
	m := newStatsUIModel(summary, entries, provider)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

package cli

import "github.com/charmbracelet/lipgloss"

type tuiTheme struct {
	canvas      lipgloss.Style
	panel       lipgloss.Style
	title       lipgloss.Style
	subtitle    lipgloss.Style
	text        lipgloss.Style
	muted       lipgloss.Style
	ok          lipgloss.Style
	warn        lipgloss.Style
	danger      lipgloss.Style
	info        lipgloss.Style
	highlight   lipgloss.Style
	help        lipgloss.Style
	railDone    lipgloss.Style
	railCurrent lipgloss.Style
	railPending lipgloss.Style
}

func newTUITheme() tuiTheme {
	return tuiTheme{
		canvas: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D7DBE0")).
			Background(lipgloss.Color("#0E1116")),
		panel: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#3D4752")).
			Padding(0, 1),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#9FD3FF")),
		subtitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#C0C8D4")),
		text: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D7DBE0")),
		muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6E7B88")),
		ok: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#63C17A")),
		warn: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E7B65A")),
		danger: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E06B75")),
		info: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#65B5FF")),
		highlight: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0E1116")).
			Background(lipgloss.Color("#65B5FF")),
		help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8FA0B3")),
		railDone: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#63C17A")),
		railCurrent: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#65B5FF")),
		railPending: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6E7B88")),
	}
}

package cli

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yoanbernabeu/grepai/trace"
)

type traceRow struct {
	title  string
	detail []string
}

type traceUIModel struct {
	theme tuiTheme

	width  int
	height int

	view   traceViewKind
	result trace.TraceResult
	rows   []traceRow

	selected int
}

func newTraceUIModel(result trace.TraceResult, view traceViewKind) traceUIModel {
	return traceUIModel{
		theme:    newTUITheme(),
		view:     view,
		result:   result,
		rows:     buildTraceRows(result, view),
		selected: 0,
	}
}

func (m traceUIModel) Init() tea.Cmd { return nil }

func (m traceUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.rows)-1 {
				m.selected++
			}
		}
	}
	return m, nil
}

func (m traceUIModel) View() string {
	if m.width == 0 {
		return "Loading trace UI..."
	}

	title := "Trace"
	switch m.view {
	case traceViewCallers:
		title = "Trace Callers"
	case traceViewCallees:
		title = "Trace Callees"
	case traceViewGraph:
		title = "Trace Graph"
	}

	headerLines := []string{
		m.theme.title.Render(title),
		m.theme.text.Render(fmt.Sprintf("Query: %s  Mode: %s", m.result.Query, m.result.Mode)),
	}
	if m.result.Graph != nil {
		headerLines = append(headerLines, m.theme.text.Render(fmt.Sprintf("Depth: %d", m.result.Graph.Depth)))
	}
	header := m.theme.panel.Width(m.width - 2).Render(strings.Join(headerLines, "\n"))

	if len(m.rows) == 0 {
		emptyTitle, emptyWhy, emptyAction := traceEmptyState(m.view, m.result)
		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			renderActionCard(
				m.theme,
				emptyTitle,
				emptyWhy,
				emptyAction,
				m.width-2,
			),
			m.theme.panel.Width(m.width-2).Render(m.theme.help.Render("q quit")),
		)
	}

	contentWidth := m.width - 2
	contentHeight := m.height - 6
	if contentHeight < 6 {
		contentHeight = 6
	}
	if contentWidth < 62 {
		topH, bottomH := panelHeights(contentHeight)
		items := make([]string, 0, len(m.rows))
		for _, row := range m.rows {
			items = append(items, row.title)
		}
		list := renderSelectableList(m.theme, "Symbols/Nodes", items, m.selected, contentWidth, topH)
		detail := m.renderDetailPanel(contentWidth, bottomH)
		footer := m.theme.panel.Width(contentWidth).Render(m.theme.help.Render("up/down select | q quit"))
		return lipgloss.JoinVertical(lipgloss.Left, header, list, detail, footer)
	}

	leftW := int(float64(contentWidth) * 0.42)
	if leftW < 30 {
		leftW = 30
	}
	rightW := contentWidth - leftW
	if rightW < 32 {
		rightW = 32
	}

	items := make([]string, 0, len(m.rows))
	for _, row := range m.rows {
		items = append(items, row.title)
	}
	list := renderSelectableList(m.theme, "Symbols/Nodes", items, m.selected, leftW, contentHeight)
	detail := m.renderDetailPanel(rightW, contentHeight)
	footer := m.theme.panel.Width(contentWidth).Render(m.theme.help.Render("up/down select | q quit"))
	return lipgloss.JoinVertical(lipgloss.Left, header, lipgloss.JoinHorizontal(lipgloss.Top, list, detail), footer)
}

func (m traceUIModel) renderDetailPanel(width, height int) string {
	row := m.rows[m.selected]
	lines := []string{
		m.theme.subtitle.Render("Detail"),
		"",
		m.theme.text.Render(row.title),
		"",
	}
	lines = append(lines, row.detail...)
	return m.theme.panel.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func buildTraceRows(result trace.TraceResult, view traceViewKind) []traceRow {
	rows := make([]traceRow, 0)

	switch view {
	case traceViewCallers:
		if result.Symbol != nil {
			rows = append(rows, traceRow{
				title: fmt.Sprintf("target: %s", result.Symbol.Name),
				detail: []string{
					fmt.Sprintf("kind: %s", result.Symbol.Kind),
					fmt.Sprintf("defined: %s:%d", result.Symbol.File, result.Symbol.Line),
					fmt.Sprintf("feature: %s", safeValue(result.Symbol.FeaturePath)),
					fmt.Sprintf("callers: %d", len(result.Callers)),
				},
			})
		}
		for _, c := range result.Callers {
			rows = append(rows, traceRow{
				title: c.Symbol.Name,
				detail: []string{
					fmt.Sprintf("defined: %s:%d", c.Symbol.File, c.Symbol.Line),
					fmt.Sprintf("feature: %s", safeValue(c.Symbol.FeaturePath)),
					fmt.Sprintf("callsite: %s:%d", c.CallSite.File, c.CallSite.Line),
					fmt.Sprintf("context: %s", safeValue(c.CallSite.Context)),
				},
			})
		}
	case traceViewCallees:
		if result.Symbol != nil {
			rows = append(rows, traceRow{
				title: fmt.Sprintf("target: %s", result.Symbol.Name),
				detail: []string{
					fmt.Sprintf("kind: %s", result.Symbol.Kind),
					fmt.Sprintf("defined: %s:%d", result.Symbol.File, result.Symbol.Line),
					fmt.Sprintf("feature: %s", safeValue(result.Symbol.FeaturePath)),
					fmt.Sprintf("callees: %d", len(result.Callees)),
				},
			})
		}
		for _, c := range result.Callees {
			rows = append(rows, traceRow{
				title: c.Symbol.Name,
				detail: []string{
					fmt.Sprintf("defined: %s:%d", c.Symbol.File, c.Symbol.Line),
					fmt.Sprintf("feature: %s", safeValue(c.Symbol.FeaturePath)),
					fmt.Sprintf("callsite: %s:%d", c.CallSite.File, c.CallSite.Line),
					fmt.Sprintf("context: %s", safeValue(c.CallSite.Context)),
				},
			})
		}
	case traceViewGraph:
		if result.Graph == nil {
			return rows
		}
		nodeNames := make([]string, 0, len(result.Graph.Nodes))
		for name := range result.Graph.Nodes {
			nodeNames = append(nodeNames, name)
		}
		sort.Strings(nodeNames)
		for _, name := range nodeNames {
			sym := result.Graph.Nodes[name]
			edges := make([]string, 0)
			for _, e := range result.Graph.Edges {
				if e.Caller == name {
					edges = append(edges, fmt.Sprintf("%s -> %s (%s:%d)", e.Caller, e.Callee, e.File, e.Line))
				}
			}
			if len(edges) == 0 {
				edges = append(edges, "no outgoing edges")
			}
			detail := []string{
				fmt.Sprintf("kind: %s", sym.Kind),
				fmt.Sprintf("defined: %s:%d", sym.File, sym.Line),
				fmt.Sprintf("feature: %s", safeValue(sym.FeaturePath)),
				"",
				"outgoing edges:",
			}
			detail = append(detail, edges...)
			rows = append(rows, traceRow{
				title:  name,
				detail: detail,
			})
		}
		if len(rows) == 0 && len(result.Graph.Edges) > 0 {
			edges := append([]trace.CallEdge(nil), result.Graph.Edges...)
			sort.Slice(edges, func(i, j int) bool {
				if edges[i].Caller != edges[j].Caller {
					return edges[i].Caller < edges[j].Caller
				}
				if edges[i].Callee != edges[j].Callee {
					return edges[i].Callee < edges[j].Callee
				}
				if edges[i].File != edges[j].File {
					return edges[i].File < edges[j].File
				}
				return edges[i].Line < edges[j].Line
			})
			for _, edge := range edges {
				detail := []string{
					fmt.Sprintf("caller: %s", safeValue(edge.Caller)),
					fmt.Sprintf("callee: %s", safeValue(edge.Callee)),
					fmt.Sprintf("callsite: %s:%d", safeValue(edge.File), edge.Line),
				}
				if strings.TrimSpace(edge.CallType) != "" {
					detail = append(detail, fmt.Sprintf("type: %s", edge.CallType))
				}
				rows = append(rows, traceRow{
					title:  fmt.Sprintf("%s -> %s", edge.Caller, edge.Callee),
					detail: detail,
				})
			}
		}
	}

	return rows
}

func traceEmptyState(view traceViewKind, result trace.TraceResult) (string, string, string) {
	switch view {
	case traceViewCallers, traceViewCallees:
		if result.Symbol == nil {
			return "No symbol match", "No symbol matched this query in the current index.", "Try a different symbol name, or run `grepai watch` if the index is stale"
		}
	case traceViewGraph:
		if result.Graph == nil || (len(result.Graph.Nodes) == 0 && len(result.Graph.Edges) == 0) {
			return "No graph data", "No call graph nodes or edges were found for this symbol.", "Try a different symbol or increase `--depth`"
		}
	}
	return "No trace rows", "No symbol or edge data found for this query.", "Run `grepai watch` then retry"
}

func safeValue(v string) string {
	if strings.TrimSpace(v) == "" {
		return "n/a"
	}
	return v
}

func runTraceResultUI(result trace.TraceResult, view traceViewKind) error {
	model := newTraceUIModel(result, view)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

type traceActionCardModel struct {
	theme  tuiTheme
	width  int
	height int
	title  string
	why    string
	action string
}

func (m traceActionCardModel) Init() tea.Cmd { return nil }

func (m traceActionCardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" || msg.String() == "enter" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m traceActionCardModel) View() string {
	width := m.width - 2
	if width <= 0 {
		width = 70
	}
	card := renderActionCard(m.theme, m.title, m.why, m.action, width)
	help := m.theme.panel.Width(width).Render(m.theme.help.Render("Enter/q quit"))
	return lipgloss.JoinVertical(lipgloss.Left, card, help)
}

func runTraceActionCardUI(title, why, action string) error {
	model := traceActionCardModel{
		theme:  newTUITheme(),
		title:  title,
		why:    why,
		action: action,
	}
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

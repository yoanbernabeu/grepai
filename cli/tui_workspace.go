package cli

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
)

type workspaceCreateStep int

const (
	workspaceStepBackend workspaceCreateStep = iota
	workspaceStepProvider
	workspaceStepReview
)

type workspaceCreateModel struct {
	theme tuiTheme

	width  int
	height int

	workspaceName string
	step          workspaceCreateStep
	backendIdx    int
	providerIdx   int

	done     bool
	canceled bool
	result   *config.Workspace
}

func newWorkspaceCreateModel(workspaceName string) workspaceCreateModel {
	return workspaceCreateModel{
		theme:         newTUITheme(),
		workspaceName: workspaceName,
		step:          workspaceStepBackend,
		backendIdx:    1, // qdrant default to match --yes path
		providerIdx:   0, // ollama
	}
}

func (m workspaceCreateModel) Init() tea.Cmd { return nil }

func (m workspaceCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.canceled = true
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.step == workspaceStepBackend {
				m.backendIdx = wrapIndex(m.backendIdx-1, 2)
			} else if m.step == workspaceStepProvider {
				m.providerIdx = wrapIndex(m.providerIdx-1, 3)
			}
		case "down", "j":
			if m.step == workspaceStepBackend {
				m.backendIdx = wrapIndex(m.backendIdx+1, 2)
			} else if m.step == workspaceStepProvider {
				m.providerIdx = wrapIndex(m.providerIdx+1, 3)
			}
		case "b":
			if m.step > workspaceStepBackend {
				m.step--
			}
		case "enter", "a":
			if m.step == workspaceStepReview {
				ws := buildWorkspaceFromSelection(m.workspaceName, m.backendIdx, m.providerIdx)
				m.result = ws
				m.done = true
				return m, tea.Quit
			}
			m.step++
		}
	}
	return m, nil
}

func (m workspaceCreateModel) View() string {
	if m.width == 0 {
		return "Loading workspace wizard..."
	}
	phases := []string{"Backend", "Provider", "Review"}
	header := m.theme.panel.Width(m.width - 2).Render(strings.Join([]string{
		m.theme.title.Render("workspace create UI"),
		m.theme.muted.Render("name: " + m.workspaceName),
	}, "\n"))
	rail := m.theme.panel.Width(m.width - 2).Render(renderLifecycleRail(m.theme, phases, int(m.step)))
	body := m.theme.panel.Width(m.width - 2).Height(m.height - 9).Render(m.renderStep())
	footer := m.theme.panel.Width(m.width - 2).Render(m.theme.help.Render("up/down choose | enter next | b back | a apply(review) | q cancel"))
	return lipgloss.JoinVertical(lipgloss.Left, header, rail, body, footer)
}

func (m workspaceCreateModel) renderStep() string {
	switch m.step {
	case workspaceStepBackend:
		options := []string{"postgres", "qdrant"}
		lines := []string{m.theme.subtitle.Render("Select storage backend"), ""}
		for i, opt := range options {
			prefix := "  "
			style := m.theme.text
			if i == m.backendIdx {
				prefix = "> "
				style = m.theme.highlight
			}
			lines = append(lines, prefix+style.Render(opt))
		}
		return strings.Join(lines, "\n")
	case workspaceStepProvider:
		options := []string{"ollama", "openai", "lmstudio"}
		lines := []string{m.theme.subtitle.Render("Select embedding provider"), ""}
		for i, opt := range options {
			prefix := "  "
			style := m.theme.text
			if i == m.providerIdx {
				prefix = "> "
				style = m.theme.highlight
			}
			lines = append(lines, prefix+style.Render(opt))
		}
		return strings.Join(lines, "\n")
	case workspaceStepReview:
		ws := buildWorkspaceFromSelection(m.workspaceName, m.backendIdx, m.providerIdx)
		lines := []string{
			m.theme.subtitle.Render("Review"),
			"",
			m.theme.text.Render(fmt.Sprintf("Backend:  %s", ws.Store.Backend)),
			m.theme.text.Render(fmt.Sprintf("Provider: %s", ws.Embedder.Provider)),
			m.theme.text.Render(fmt.Sprintf("Model:    %s", ws.Embedder.Model)),
		}
		switch ws.Store.Backend {
		case "postgres":
			lines = append(lines, m.theme.text.Render(fmt.Sprintf("DSN:      %s", ws.Store.Postgres.DSN)))
		case "qdrant":
			lines = append(lines,
				m.theme.text.Render(fmt.Sprintf("Qdrant endpoint: %s", ws.Store.Qdrant.Endpoint)),
				m.theme.text.Render(fmt.Sprintf("Qdrant port:     %d", ws.Store.Qdrant.Port)),
			)
		}
		lines = append(lines, "", m.theme.info.Render("Press A (or Enter) to create workspace."))
		return strings.Join(lines, "\n")
	default:
		return ""
	}
}

func buildWorkspaceFromSelection(name string, backendIdx, providerIdx int) *config.Workspace {
	backend := "postgres"
	if backendIdx == 1 {
		backend = "qdrant"
	}

	provider := "ollama"
	switch providerIdx {
	case 1:
		provider = "openai"
	case 2:
		provider = "lmstudio"
	}

	ws, err := buildWorkspaceFromFlags(name, backend, provider, "", "", "", "", 0, "", true)
	if err != nil {
		return &config.Workspace{
			Name:     name,
			Projects: []config.ProjectEntry{},
		}
	}
	return ws
}

func createWorkspaceTUI(workspaceName string) (*config.Workspace, error) {
	model := newWorkspaceCreateModel(workspaceName)
	program := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return nil, err
	}
	result, ok := finalModel.(workspaceCreateModel)
	if !ok {
		return nil, fmt.Errorf("unexpected workspace create model type")
	}
	if result.canceled || result.result == nil {
		return nil, fmt.Errorf("workspace creation canceled")
	}
	return result.result, nil
}

type workspaceStatusModel struct {
	theme tuiTheme

	width  int
	height int

	workspaces []config.Workspace
	entries    []string
	selected   int
	logDir     string
}

func newWorkspaceStatusModel(cfg *config.WorkspaceConfig, onlyName string) workspaceStatusModel {
	model := workspaceStatusModel{
		theme:    newTUITheme(),
		selected: 0,
	}

	for _, ws := range cfg.Workspaces {
		if onlyName != "" && ws.Name != onlyName {
			continue
		}
		model.workspaces = append(model.workspaces, ws)
		model.entries = append(model.entries, fmt.Sprintf("%s (%s, %d projects)", ws.Name, ws.Store.Backend, len(ws.Projects)))
	}

	logDir, err := daemon.GetDefaultLogDir()
	if err == nil {
		model.logDir = logDir
	}

	return model
}

func (m workspaceStatusModel) Init() tea.Cmd { return nil }

func (m workspaceStatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.selected < len(m.entries)-1 {
				m.selected++
			}
		}
	}
	return m, nil
}

func (m workspaceStatusModel) View() string {
	if m.width == 0 {
		return "Loading workspace status UI..."
	}
	if len(m.workspaces) == 0 {
		return renderActionCard(
			m.theme,
			"No workspaces",
			"No workspace configuration found.",
			"Run: grepai workspace create <name>",
			m.width-2,
		)
	}

	contentWidth := m.width - 2
	contentHeight := m.height - 5
	if contentHeight < 6 {
		contentHeight = 6
	}
	if contentWidth < 60 {
		topH, bottomH := panelHeights(contentHeight)
		listPanel := renderSelectableList(m.theme, "Workspaces", m.entries, m.selected, contentWidth, topH)
		detailPanel := m.renderWorkspaceDetail(contentWidth, bottomH)
		footer := m.theme.panel.Width(contentWidth).Render(m.theme.help.Render("up/down select | q quit"))
		return lipgloss.JoinVertical(lipgloss.Left, listPanel, detailPanel, footer)
	}

	leftW := int(float64(contentWidth) * 0.45)
	if leftW < 30 {
		leftW = 30
	}
	rightW := contentWidth - leftW
	if rightW < 30 {
		rightW = 30
		leftW = contentWidth - rightW
	}

	listPanel := renderSelectableList(m.theme, "Workspaces", m.entries, m.selected, leftW, contentHeight)
	detailPanel := m.renderWorkspaceDetail(rightW, contentHeight)
	footer := m.theme.panel.Width(contentWidth).Render(m.theme.help.Render("up/down select | q quit"))
	return lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, listPanel, detailPanel), footer)
}

func (m workspaceStatusModel) renderWorkspaceDetail(width, height int) string {
	ws := m.workspaces[m.selected]
	lines := []string{
		m.theme.subtitle.Render("Workspace Detail"),
		"",
		m.theme.text.Render(fmt.Sprintf("Name: %s", ws.Name)),
		m.theme.text.Render(fmt.Sprintf("Backend: %s", ws.Store.Backend)),
		m.theme.text.Render(fmt.Sprintf("Provider: %s (%s)", ws.Embedder.Provider, ws.Embedder.Model)),
		m.theme.text.Render(fmt.Sprintf("Projects: %d", len(ws.Projects))),
	}

	if m.logDir != "" {
		pid, _ := daemon.GetRunningWorkspacePID(m.logDir, ws.Name)
		if pid > 0 {
			lines = append(lines, m.theme.ok.Render(fmt.Sprintf("Watcher: running (PID %d)", pid)))
			lines = append(lines, m.theme.text.Render(fmt.Sprintf("Log file: %s", daemon.GetWorkspaceLogFile(m.logDir, ws.Name))))
		} else {
			lines = append(lines, m.theme.muted.Render("Watcher: not running"))
		}
	}

	lines = append(lines, "")
	for _, p := range ws.Projects {
		exists := "ok"
		style := m.theme.ok
		if _, err := os.Stat(p.Path); os.IsNotExist(err) {
			exists = "missing"
			style = m.theme.danger
		}
		lines = append(lines, fmt.Sprintf("- %s", p.Name))
		lines = append(lines, fmt.Sprintf("  %s %s", style.Render(exists), m.theme.muted.Render(p.Path)))
	}
	return m.theme.panel.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func runWorkspaceStatusUI(cfg *config.WorkspaceConfig, args []string) error {
	onlyName := ""
	if len(args) > 0 {
		onlyName = args[0]
	}
	model := newWorkspaceStatusModel(cfg, onlyName)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

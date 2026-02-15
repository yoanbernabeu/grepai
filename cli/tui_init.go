package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/git"
)

type initWizardStep int

const (
	initStepEnv initWizardStep = iota
	initStepInherit
	initStepProvider
	initStepBackend
	initStepReview
)

var initProviderOptions = []string{"ollama", "lmstudio", "openai"}
var initBackendOptions = []string{"gob", "postgres", "qdrant"}

type initUIModel struct {
	theme tuiTheme

	width  int
	height int

	cwd string

	step initWizardStep

	allowInherit bool
	inherit      bool
	worktreeInfo *git.DetectInfo
	mainCfg      *config.Config

	providerIdx int
	backendIdx  int

	canceled bool
	done     bool
	result   *config.Config
}

func newInitUIModel(cwd string, baseCfg *config.Config, gitInfo *git.DetectInfo, mainCfg *config.Config, forceInherit bool) initUIModel {
	providerIdx := optionIndex(initProviderOptions, initProvider)
	backendIdx := optionIndex(initBackendOptions, initBackend)

	if providerIdx < 0 && baseCfg != nil {
		providerIdx = optionIndex(initProviderOptions, baseCfg.Embedder.Provider)
	}
	if backendIdx < 0 && baseCfg != nil {
		backendIdx = optionIndex(initBackendOptions, baseCfg.Store.Backend)
	}
	if providerIdx < 0 {
		providerIdx = 0
	}
	if backendIdx < 0 {
		backendIdx = 0
	}

	model := initUIModel{
		theme:        newTUITheme(),
		cwd:          cwd,
		step:         initStepEnv,
		allowInherit: gitInfo != nil && mainCfg != nil,
		inherit:      forceInherit,
		worktreeInfo: gitInfo,
		mainCfg:      mainCfg,
		providerIdx:  providerIdx,
		backendIdx:   backendIdx,
	}

	if model.allowInherit && forceInherit {
		model.step = initStepReview
	}

	return model
}

func (m initUIModel) Init() tea.Cmd { return nil }

func (m initUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "b":
			m.stepBack()
		case "enter", "a":
			if m.step == initStepReview {
				cfg, err := m.buildConfig()
				if err != nil {
					m.canceled = true
				} else {
					m.result = cfg
				}
				m.done = true
				return m, tea.Quit
			}
			m.stepForward()
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Allow direct selection by number (1-based index)
			idx := int(msg.String()[0] - '1')
			m.selectByIndex(idx)
		}
	}
	return m, nil
}

func (m initUIModel) View() string {
	if m.width == 0 {
		return "Loading init wizard..."
	}

	phases := []string{"Env", "Inherit", "Provider", "Backend", "Review"}
	current := int(m.step)
	if !m.allowInherit {
		phases = []string{"Env", "Provider", "Backend", "Review"}
		if m.step > initStepEnv {
			current = int(m.step) - 1
		}
	}

	header := m.theme.panel.Width(m.width - 2).Render(strings.Join([]string{
		m.theme.title.Render("grepai init UI"),
		m.theme.muted.Render("Configure provider/backend with guided steps"),
		m.theme.text.Render("cwd: " + m.cwd),
	}, "\n"))
	rail := m.theme.panel.Width(m.width - 2).Render(renderLifecycleRail(m.theme, phases, current))
	body := m.theme.panel.Width(m.width - 2).Height(m.height - 10).Render(m.renderStepContent())
	footer := m.theme.panel.Width(m.width - 2).Render(m.theme.help.Render("up/down choose | enter next | b back | a apply(review) | q cancel"))
	return strings.Join([]string{header, rail, body, footer}, "\n")
}

func (m *initUIModel) moveSelection(delta int) {
	switch m.step {
	case initStepInherit:
		if !m.allowInherit {
			return
		}
		m.inherit = !m.inherit
	case initStepProvider:
		m.providerIdx = wrapIndex(m.providerIdx+delta, len(initProviderOptions))
	case initStepBackend:
		m.backendIdx = wrapIndex(m.backendIdx+delta, len(initBackendOptions))
	}
}

func (m *initUIModel) selectByIndex(idx int) {
	switch m.step {
	case initStepProvider:
		if idx >= 0 && idx < len(initProviderOptions) {
			m.providerIdx = idx
		}
	case initStepBackend:
		if idx >= 0 && idx < len(initBackendOptions) {
			m.backendIdx = idx
		}
	}
}

func (m *initUIModel) stepForward() {
	switch m.step {
	case initStepEnv:
		if m.allowInherit {
			m.step = initStepInherit
			return
		}
		m.step = initStepProvider
	case initStepInherit:
		if m.inherit {
			m.step = initStepReview
			return
		}
		m.step = initStepProvider
	case initStepProvider:
		m.step = initStepBackend
	case initStepBackend:
		m.step = initStepReview
	}
}

func (m *initUIModel) stepBack() {
	switch m.step {
	case initStepReview:
		if m.allowInherit && m.inherit {
			m.step = initStepInherit
			return
		}
		m.step = initStepBackend
	case initStepBackend:
		m.step = initStepProvider
	case initStepProvider:
		if m.allowInherit {
			m.step = initStepInherit
			return
		}
		m.step = initStepEnv
	case initStepInherit:
		m.step = initStepEnv
	}
}

func (m initUIModel) renderStepContent() string {
	switch m.step {
	case initStepEnv:
		return strings.Join([]string{
			m.theme.subtitle.Render("Environment Check"),
			"",
			m.theme.text.Render("Project path is available and writable."),
			m.theme.text.Render("Init will create .grepai/config.yaml and update .gitignore if present."),
			"",
			m.theme.info.Render("Press Enter to continue."),
		}, "\n")
	case initStepInherit:
		choice := "No"
		if m.inherit {
			choice = "Yes"
		}
		lines := []string{
			m.theme.subtitle.Render("Inherit Main Worktree Configuration"),
			"",
			m.theme.text.Render(fmt.Sprintf("Main worktree: %s", m.worktreeInfo.MainWorktree)),
			m.theme.text.Render(fmt.Sprintf("Worktree ID:   %s", m.worktreeInfo.WorktreeID)),
			m.theme.text.Render(fmt.Sprintf("Use inherited config: %s", choice)),
			"",
			m.theme.muted.Render("Use up/down to toggle yes/no, Enter to continue."),
		}
		return strings.Join(lines, "\n")
	case initStepProvider:
		return m.renderOptionStep("Embedding Provider", initProviderOptions, m.providerIdx, "Select provider. Endpoint/model defaults will be applied.")
	case initStepBackend:
		return m.renderOptionStep("Storage Backend", initBackendOptions, m.backendIdx, "Select backend. You can edit details manually in .grepai/config.yaml later.")
	case initStepReview:
		cfg, _ := m.buildConfig()
		return m.renderReview(cfg)
	default:
		return ""
	}
}

func (m initUIModel) renderOptionStep(title string, options []string, selected int, description string) string {
	lines := []string{m.theme.subtitle.Render(title), "", m.theme.muted.Render(description), ""}
	for i, opt := range options {
		prefix := "  "
		style := m.theme.text
		if i == selected {
			prefix = "> "
			style = m.theme.highlight
		}
		lines = append(lines, prefix+style.Render(opt))
	}
	lines = append(lines, "", m.theme.help.Render("up/down choose, Enter continue"))
	return strings.Join(lines, "\n")
}

func (m initUIModel) renderReview(cfg *config.Config) string {
	if cfg == nil {
		return m.theme.danger.Render("Failed to build configuration preview.")
	}
	lines := []string{
		m.theme.subtitle.Render("Review"),
		"",
		m.theme.text.Render(fmt.Sprintf("Provider: %s", cfg.Embedder.Provider)),
		m.theme.text.Render(fmt.Sprintf("Model:    %s", cfg.Embedder.Model)),
		m.theme.text.Render(fmt.Sprintf("Endpoint: %s", cfg.Embedder.Endpoint)),
		m.theme.text.Render(fmt.Sprintf("Backend:  %s", cfg.Store.Backend)),
	}
	switch cfg.Store.Backend {
	case "postgres":
		lines = append(lines, m.theme.text.Render(fmt.Sprintf("DSN:      %s", cfg.Store.Postgres.DSN)))
	case "qdrant":
		lines = append(lines,
			m.theme.text.Render(fmt.Sprintf("Qdrant endpoint: %s", cfg.Store.Qdrant.Endpoint)),
			m.theme.text.Render(fmt.Sprintf("Qdrant port:     %d", cfg.Store.Qdrant.Port)),
		)
	}
	lines = append(lines, "", m.theme.info.Render("Press A (or Enter) to apply configuration."))
	return strings.Join(lines, "\n")
}

func (m initUIModel) buildConfig() (*config.Config, error) {
	if m.allowInherit && m.inherit && m.mainCfg != nil {
		cloned := *m.mainCfg
		return &cloned, nil
	}

	cfg := config.DefaultConfig()
	if cfg == nil {
		return nil, fmt.Errorf("failed to build default config")
	}

	switch initProviderOptions[m.providerIdx] {
	case "ollama":
		cfg.Embedder.Provider = "ollama"
		cfg.Embedder.Model = "nomic-embed-text"
		cfg.Embedder.Endpoint = "http://localhost:11434"
		dim := 768
		cfg.Embedder.Dimensions = &dim
	case "lmstudio":
		cfg.Embedder.Provider = "lmstudio"
		cfg.Embedder.Model = "text-embedding-nomic-embed-text-v1.5"
		cfg.Embedder.Endpoint = "http://127.0.0.1:1234"
		dim := lmStudioEmbeddingDimensions
		cfg.Embedder.Dimensions = &dim
	case "openai":
		cfg.Embedder.Provider = "openai"
		cfg.Embedder.Model = "text-embedding-3-small"
		cfg.Embedder.Endpoint = "https://api.openai.com/v1"
		cfg.Embedder.Dimensions = nil
	}

	switch initBackendOptions[m.backendIdx] {
	case "gob":
		cfg.Store.Backend = "gob"
	case "postgres":
		cfg.Store.Backend = "postgres"
		cfg.Store.Postgres.DSN = "postgres://localhost:5432/grepai"
	case "qdrant":
		cfg.Store.Backend = "qdrant"
		cfg.Store.Qdrant.Endpoint = "localhost"
		cfg.Store.Qdrant.Port = 6334
		cfg.Store.Qdrant.UseTLS = false
		cfg.Store.Qdrant.Collection = ""
	}

	return cfg, nil
}

func runInitWizardUI(cwd string, baseCfg *config.Config, gitInfo *git.DetectInfo, mainCfg *config.Config, forceInherit bool) (*config.Config, error) {
	model := newInitUIModel(cwd, baseCfg, gitInfo, mainCfg, forceInherit)
	program := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return nil, err
	}
	result, ok := finalModel.(initUIModel)
	if !ok {
		return nil, fmt.Errorf("unexpected init UI model type")
	}
	if result.canceled || result.result == nil {
		return nil, fmt.Errorf("initialization canceled")
	}
	return result.result, nil
}

func optionIndex(options []string, value string) int {
	for i, v := range options {
		if v == value {
			return i
		}
	}
	return -1
}

func wrapIndex(v, size int) int {
	if size <= 0 {
		return 0
	}
	if v < 0 {
		return size - 1
	}
	if v >= size {
		return 0
	}
	return v
}

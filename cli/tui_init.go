package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/git"
)

type initWizardStep int

const (
	initStepEnv initWizardStep = iota
	initStepInherit
	initStepProvider
	initStepProviderConfig
	initStepBackend
	initStepBackendConfig
	initStepRPG
	initStepRPGMode
	initStepRPGConfig
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

	// RPG Config
	rpgEnabled bool
	rpgUseLLM  bool

	// Inputs for Provider Config
	providerInputs []textinput.Model
	// Inputs for Backend Config
	backendInputs []textinput.Model
	// Inputs for RPG Config
	rpgInputs []textinput.Model

	focusIndex int

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
		rpgEnabled:   false,
		rpgUseLLM:    false,
	}

	if model.allowInherit && forceInherit {
		model.step = initStepReview
	}

	return model
}

func (m initUIModel) Init() tea.Cmd { return textinput.Blink }

func (m initUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.canceled = true
			m.done = true
			return m, tea.Quit
		}
	}

	// Handle inputs if in config step
	if m.step == initStepProviderConfig || m.step == initStepBackendConfig || m.step == initStepRPGConfig {
		return m.updateInputs(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q": // Allow q to quit only in non-input steps
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

func (m initUIModel) updateInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var inputs []textinput.Model

	if m.step == initStepProviderConfig {
		inputs = m.providerInputs
	} else if m.step == initStepBackendConfig {
		inputs = m.backendInputs
	} else {
		inputs = m.rpgInputs
	}

	if len(inputs) == 0 {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.stepForward()
				return m, nil
			case "esc", "b":
				m.stepBack()
				return m, nil
			case "q":
				m.canceled = true
				m.done = true
				return m, tea.Quit
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.focusIndex == len(inputs)-1 {
				m.stepForward()
				return m, nil
			}
			m.focusIndex++
		case "up", "shift+tab":
			m.focusIndex--
		case "down", "tab":
			m.focusIndex++
		case "b":
			// Checking b here is risky if input allows 'b'.
			// But we already use 'esc' for back.
		case "esc":
			m.stepBack()
			return m, nil
		}
	}

	if m.focusIndex > len(inputs)-1 {
		m.focusIndex = len(inputs) - 1
	}
	if m.focusIndex < 0 {
		m.focusIndex = 0
	}

	cmds := make([]tea.Cmd, 0, len(inputs))
	for i := range inputs {
		inputs[i].Blur()
		if i == m.focusIndex {
			inputs[i].Focus()
		}
		var cmd tea.Cmd
		inputs[i], cmd = inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.step == initStepProviderConfig {
		m.providerInputs = inputs
	} else if m.step == initStepBackendConfig {
		m.backendInputs = inputs
	} else {
		m.rpgInputs = inputs
	}

	return m, tea.Batch(cmds...)
}

func (m initUIModel) View() string {
	if m.width == 0 {
		return "Loading init wizard..."
	}

	phases := []string{"Env", "Inherit", "Provider", "Config", "Backend", "Config", "RPG", "Mode", "Config", "Review"}
	current := int(m.step)
	// Adjust phase display based on skipped steps
	if !m.allowInherit {
		// If inherit is skipped
		phases = []string{"Env", "Provider", "Config", "Backend", "Config", "RPG", "Mode", "Config", "Review"}
		if m.step > initStepEnv {
			current = int(m.step) - 1
		}
	}

	header := m.theme.panel.Width(m.width - 2).Render(strings.Join([]string{
		m.theme.title.Render("grepai init UI"),
		m.theme.muted.Render("Configure provider/backend with guided steps"),
		m.theme.text.Render("cwd: " + m.cwd),
	}, "\n"))

	// Simplified rail for display
	rail := m.theme.panel.Width(m.width - 2).Render(m.theme.muted.Render(fmt.Sprintf("Step %d/%d", current+1, len(phases))))

	body := m.theme.panel.Width(m.width - 2).Height(m.height - 10).Render(m.renderStepContent())

	help := "up/down choose | enter next | b back | q cancel"
	if m.step == initStepProviderConfig || m.step == initStepBackendConfig || m.step == initStepRPGConfig {
		help = "tab/shift+tab nav | enter next/submit | esc back | ctrl+c cancel"
	}
	footer := m.theme.panel.Width(m.width - 2).Render(m.theme.help.Render(help))
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
	case initStepRPG:
		m.rpgEnabled = !m.rpgEnabled
	case initStepRPGMode:
		m.rpgUseLLM = !m.rpgUseLLM
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
		m.initProviderInputs()
		m.step = initStepProviderConfig
	case initStepProviderConfig:
		m.step = initStepBackend
	case initStepBackend:
		m.initBackendInputs()
		m.step = initStepBackendConfig
	case initStepBackendConfig:
		m.step = initStepRPG
	case initStepRPG:
		if m.rpgEnabled {
			m.step = initStepRPGMode
		} else {
			m.step = initStepReview
		}
	case initStepRPGMode:
		if m.rpgUseLLM {
			m.initRPGInputs()
			m.step = initStepRPGConfig
		} else {
			m.step = initStepReview
		}
	case initStepRPGConfig:
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
		if m.rpgEnabled {
			if m.rpgUseLLM {
				m.step = initStepRPGConfig
			} else {
				m.step = initStepRPGMode
			}
		} else {
			m.step = initStepRPG
		}
	case initStepRPGConfig:
		m.step = initStepRPGMode
	case initStepRPGMode:
		m.step = initStepRPG
	case initStepRPG:
		m.step = initStepBackendConfig
	case initStepBackendConfig:
		m.step = initStepBackend
	case initStepBackend:
		m.step = initStepProviderConfig
	case initStepProviderConfig:
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

func (m *initUIModel) initProviderInputs() {
	m.providerInputs = []textinput.Model{}
	m.focusIndex = 0

	provider := initProviderOptions[m.providerIdx]
	providerDefaults := config.DefaultEmbedderForProvider(provider)

	// Common Endpoint Input
	tiEndpoint := textinput.New()
	tiEndpoint.Placeholder = "Endpoint URL"
	tiEndpoint.CharLimit = 200
	tiEndpoint.Width = 50
	tiEndpoint.SetValue(providerDefaults.Endpoint)

	// Common Model Input (optional/readonly depending on provider)
	tiModel := textinput.New()
	tiModel.Placeholder = "Model Name"
	tiModel.CharLimit = 100
	tiModel.Width = 50
	tiModel.SetValue(providerDefaults.Model)

	m.providerInputs = append(m.providerInputs, tiEndpoint, tiModel)
}

func (m *initUIModel) initBackendInputs() {
	m.backendInputs = []textinput.Model{}
	m.focusIndex = 0

	backend := initBackendOptions[m.backendIdx]
	backendDefaults := config.DefaultStoreForBackend(backend)
	switch backend {
	case "gob":
		// No config needed
	case "postgres":
		tiDSN := textinput.New()
		tiDSN.Placeholder = "postgres://user:pass@localhost:5432/grepai"
		tiDSN.SetValue(backendDefaults.Postgres.DSN)
		tiDSN.Width = 60
		m.backendInputs = append(m.backendInputs, tiDSN)
	case "qdrant":
		tiEndpoint := textinput.New()
		tiEndpoint.Placeholder = "localhost"
		tiEndpoint.SetValue(backendDefaults.Qdrant.Endpoint)

		tiPort := textinput.New()
		tiPort.Placeholder = "6334"
		tiPort.SetValue(strconv.Itoa(backendDefaults.Qdrant.Port))

		tiCollection := textinput.New()
		tiCollection.Placeholder = "Collection Name (optional)"

		tiAPIKey := textinput.New()
		tiAPIKey.Placeholder = "API Key (optional)"
		tiAPIKey.EchoMode = textinput.EchoPassword

		m.backendInputs = append(m.backendInputs, tiEndpoint, tiPort, tiCollection, tiAPIKey)
	}
}

func (m *initUIModel) initRPGInputs() {
	m.rpgInputs = []textinput.Model{}
	m.focusIndex = 0

	// Use Provider defaults as starting point
	defProvider := "ollama"
	defEndpoint := "http://localhost:11434/v1"
	defModel := "llama3"

	// Try to be smart about defaults based on embedding provider selection
	provider := initProviderOptions[m.providerIdx]
	if provider == "ollama" {
		defProvider = "ollama"
		if len(m.providerInputs) > 0 {
			// Ollama embedding endpoint usually lacks /v1, but chat endpoint might need it depending on library
			// But config.go defaults to /v1 for LLM.
			// Let's use the input
			// If embedding endpoint is http://localhost:11434, LLM endpoint is usually same base.
			// Grepai config expects OpenAI-compatible endpoint style usually for LLM?
			// Actually config says: LLMEndpoint "http://localhost:11434/v1"
			defEndpoint = "http://localhost:11434/v1"
		}
	} else if provider == "openai" {
		defProvider = "openai"
		defEndpoint = "https://api.openai.com/v1"
		defModel = "gpt-4o"
	}

	tiProvider := textinput.New()
	tiProvider.Placeholder = "LLM Provider (ollama, openai, etc)"
	tiProvider.SetValue(defProvider)
	tiProvider.Width = 30

	tiEndpoint := textinput.New()
	tiEndpoint.Placeholder = "LLM Endpoint URL"
	tiEndpoint.SetValue(defEndpoint)
	tiEndpoint.Width = 50

	tiModel := textinput.New()
	tiModel.Placeholder = "Model Name"
	tiModel.SetValue(defModel)
	tiModel.Width = 30

	tiKey := textinput.New()
	tiKey.Placeholder = "API Key (optional)"
	tiKey.EchoMode = textinput.EchoPassword
	tiKey.Width = 50

	m.rpgInputs = append(m.rpgInputs, tiProvider, tiEndpoint, tiModel, tiKey)
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
		return m.renderOptionStep("Embedding Provider", initProviderOptions, m.providerIdx, "Select provider.")
	case initStepProviderConfig:
		return m.renderInputs("Provider Configuration", []string{"Endpoint", "Model"}, m.providerInputs)
	case initStepBackend:
		return m.renderOptionStep("Storage Backend", initBackendOptions, m.backendIdx, "Select backend.")
	case initStepBackendConfig:
		var labels []string
		if initBackendOptions[m.backendIdx] == "postgres" {
			labels = []string{"DSN"}
		} else if initBackendOptions[m.backendIdx] == "qdrant" {
			labels = []string{"Endpoint", "Port", "Collection", "API Key"}
		} else {
			return m.theme.text.Render("No configuration needed for GOB backend.\n\nPress Enter to continue.")
		}
		return m.renderInputs("Backend Configuration", labels, m.backendInputs)
	case initStepRPG:
		choice := "No"
		if m.rpgEnabled {
			choice = "Yes"
		}
		lines := []string{
			m.theme.subtitle.Render("Repository Planning Graph (RPG)"),
			"",
			m.theme.text.Render("Generate a high-fidelity semantic map of your codebase using the RPG-Encoder framework (arXiv:2602.02084)."),
			m.theme.text.Render("Bridge the reasoning gap between high-level intent and low-level code."),
			m.theme.text.Render("- Encodes code into a dual-layer graph: $V_L$ (Implementation) -> $V_H$ (Abstractions)"),
			m.theme.text.Render("- Enables structure-aware navigation for agents via RPG Operations"),
			m.theme.text.Render("- Tracks evolution and drift of implementation via Incremental Maintenance"),
			"",
			m.theme.text.Render(fmt.Sprintf("Enable RPG (Repository Planning Graph): %s", choice)),
			"",
			m.theme.muted.Render("Use up/down to toggle, Enter to continue."),
		}
		return strings.Join(lines, "\n")
	case initStepRPGMode:
		choice := "No (Local Mode)"
		if m.rpgUseLLM {
			choice = "Yes (AI Mode)"
		}
		desc := "Local Mode: Heuristic-based encoding. Fast, deterministic map using directory structure and verb rules."
		if m.rpgUseLLM {
			desc = "AI Mode: Semantic Lifting. Uses LLM to extract intent/concepts ($E_{feature}$) and induce functional clusters."
		}

		lines := []string{
			m.theme.subtitle.Render("RPG Intelligence Mode"),
			"",
			m.theme.text.Render("Do you want to use an AI Model for Semantic Lifting?"),
			m.theme.text.Render(fmt.Sprintf("Use AI Model: %s", choice)),
			"",
			m.theme.info.Render(desc),
			"",
			m.theme.muted.Render("Use up/down to toggle, Enter to continue."),
		}
		return strings.Join(lines, "\n")
	case initStepRPGConfig:
		return m.renderInputs("RPG AI Configuration", []string{"Provider", "Endpoint", "Model", "API Key"}, m.rpgInputs)
	case initStepReview:
		cfg, _ := m.buildConfig()
		return m.renderReview(cfg)
	default:
		return ""
	}
}

func (m initUIModel) renderInputs(title string, labels []string, inputs []textinput.Model) string {
	lines := []string{m.theme.subtitle.Render(title), ""}

	for i, input := range inputs {
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		lines = append(lines, m.theme.text.Render(label))
		lines = append(lines, input.View())
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
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
	rpgStatus := "Disabled"
	if cfg.RPG.Enabled {
		mode := cfg.RPG.FeatureMode
		if mode == "" {
			mode = "local"
		}
		rpgStatus = fmt.Sprintf("Enabled (%s)", mode)
	}

	lines := []string{
		m.theme.subtitle.Render("Review"),
		"",
		m.theme.text.Render(fmt.Sprintf("Provider: %s", cfg.Embedder.Provider)),
		m.theme.text.Render(fmt.Sprintf("Model:    %s", cfg.Embedder.Model)),
		m.theme.text.Render(fmt.Sprintf("Endpoint: %s", cfg.Embedder.Endpoint)),
		m.theme.text.Render(fmt.Sprintf("Backend:  %s", cfg.Store.Backend)),
		m.theme.text.Render(fmt.Sprintf("RPG Mode: %s", rpgStatus)),
	}

	if cfg.RPG.Enabled && cfg.RPG.FeatureMode != "local" {
		lines = append(lines, m.theme.muted.Render(fmt.Sprintf("  AI: %s @ %s", cfg.RPG.LLMModel, cfg.RPG.LLMProvider)))
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

	// RPG Config
	cfg.RPG.Enabled = m.rpgEnabled
	if m.rpgEnabled {
		if m.rpgUseLLM {
			cfg.RPG.FeatureMode = "hybrid" // or "llm" depending on preference, hybrid usually implies both
			// Actually config.go says "local | hybrid | llm".
			// Let's use "hybrid" as safe default for AI usage + static
			if len(m.rpgInputs) >= 3 {
				cfg.RPG.LLMProvider = m.rpgInputs[0].Value()
				cfg.RPG.LLMEndpoint = m.rpgInputs[1].Value()
				cfg.RPG.LLMModel = m.rpgInputs[2].Value()
			}
			if len(m.rpgInputs) >= 4 {
				cfg.RPG.LLMAPIKey = m.rpgInputs[3].Value()
			}
		} else {
			cfg.RPG.FeatureMode = "local"
		}
	}

	// Provider Config from Inputs
	provider := initProviderOptions[m.providerIdx]
	cfg.Embedder = config.DefaultEmbedderForProvider(provider)

	if len(m.providerInputs) >= 2 {
		cfg.Embedder.Endpoint = m.providerInputs[0].Value()
		cfg.Embedder.Model = m.providerInputs[1].Value()
	}

	// Backend Config from Inputs
	backend := initBackendOptions[m.backendIdx]
	cfg.Store = config.DefaultStoreForBackend(backend)

	switch cfg.Store.Backend {
	case "postgres":
		if len(m.backendInputs) > 0 {
			cfg.Store.Postgres.DSN = m.backendInputs[0].Value()
		}
	case "qdrant":
		if len(m.backendInputs) >= 2 {
			cfg.Store.Qdrant.Endpoint = m.backendInputs[0].Value()
			if port, err := strconv.Atoi(m.backendInputs[1].Value()); err == nil {
				cfg.Store.Qdrant.Port = port
			}
		}
		if len(m.backendInputs) >= 3 {
			cfg.Store.Qdrant.Collection = m.backendInputs[2].Value()
		}
		if len(m.backendInputs) >= 4 {
			cfg.Store.Qdrant.APIKey = m.backendInputs[3].Value()
		}
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

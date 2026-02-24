package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/watcher"
)

const (
	removedSessionTTL  = 10 * time.Second
	watchUIPrunePeriod = time.Second
	watchUILogSystem   = "__system__"
)

// Messages (kept from original)
type watchUIContextMsg struct {
	projectRoot string
	provider    string
	model       string
	backend     string
	rpg         string
}

type watchUIPhaseMsg struct {
	current int
}

type watchUIScanMsg struct {
	current int
	total   int
	file    string
}

type watchUIEmbedMsg struct {
	completed int
	total     int
	retrying  bool
	attempt   int
	status    int
}

type watchUIRPGMsg struct {
	step    string
	current int
	total   int
}

type watchUISymbolMsg struct {
	count int
}

type watchUILedgerMsg struct {
	source string
	level  string
	text   string
}

type watchUISessionMsg struct {
	projectRoot string
	state       string
	note        string
}

type watchUIHealthMsg struct {
	totalEvents int
	lastSuccess time.Time
}

type watchUIScopeMsg struct {
	totalProjects int
}

type watchUIReadyMsg struct {
	projectRoot string
}

type watchUIErrorMsg struct {
	err error
}

type watchUIDoneMsg struct{}
type watchUIPruneMsg struct {
	at time.Time
}

type watchUIActivityMsg struct {
	state string
	file  string
}

type watchUIStatsMsg struct {
	projectRoot string
	delta       watchStatsDelta
}

type watchUIModel struct {
	theme tuiTheme

	width  int
	height int

	cancel context.CancelFunc

	projectRoot string
	provider    string
	model       string
	backend     string
	rpg         string

	phases      []string
	currentStep int

	// Progress state (partially handled by progressModel, but we keep text details here)
	scanFile        string
	embedRetryInfo  string
	currentActivity string

	// Stats
	filesIndexed  int
	chunksCreated int
	filesRemoved  int
	symbolCount   int
	snapshots     map[string]watchStatsDelta
	snapshotDrift map[string]watchStatsDelta

	totalEvents int
	lastSuccess time.Time

	totalProjects int
	readyProjects int

	sessions     map[string]watchUISessionState
	sessionOrder []string
	sessionFocus int

	// Components
	ledger   ledgerModel
	progress progressModel

	showHelp bool
	stopping bool
	done     bool
	err      error
	started  time.Time
}

type watchUISessionState struct {
	projectRoot string
	label       string
	state       string
	note        string
	events      int
	updatedAt   time.Time
	removedAt   time.Time
}

func newWatchUIModel(cancel context.CancelFunc) watchUIModel {
	theme := newTUITheme()
	return watchUIModel{
		theme:         theme,
		cancel:        cancel,
		phases:        []string{"Preflight", "Scan", "Embed", "Symbol", "Steady"},
		currentStep:   0,
		totalProjects: 1,
		sessions:      make(map[string]watchUISessionState),
		sessionOrder:  make([]string, 0, 4),
		sessionFocus:  -1,
		started:       time.Now(),
		ledger:        newLedgerModel(theme),
		progress:      newProgressModel(theme),
		snapshots:     make(map[string]watchStatsDelta),
		snapshotDrift: make(map[string]watchStatsDelta),
	}
}

func (m watchUIModel) Init() tea.Cmd {
	return nil
}

func (m watchUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancel != nil && !m.stopping {
				m.stopping = true
				m.cancel()
				m.ledger.addEntry(ledgerEntry{
					at:    time.Now(),
					level: "warn",
					text:  "Stopping watcher and persisting index...",
				})
			}
		case "p":
			m.ledger.togglePause()
		case "?":
			m.showHelp = !m.showHelp
		case "tab":
			m.cycleSessionFocus()
		}

	case watchUIContextMsg:
		m.projectRoot = msg.projectRoot
		m.provider = msg.provider
		m.model = msg.model
		m.backend = msg.backend
		m.rpg = msg.rpg

	case watchUIPhaseMsg:
		if msg.current < 0 {
			msg.current = 0
		}
		if msg.current >= len(m.phases) {
			msg.current = len(m.phases) - 1
		}
		m.currentStep = msg.current

	case watchUIScanMsg:
		m.progress.setScanProgress(msg.current, msg.total)
		m.scanFile = msg.file
		if msg.total > 0 {
			m.currentStep = 1
		}

	case watchUIEmbedMsg:
		m.progress.setEmbedProgress(msg.completed, msg.total)
		if msg.total > 0 {
			m.currentStep = 2
		}
		if msg.retrying {
			m.embedRetryInfo = fmt.Sprintf("retry batch attempt %d (%s)", msg.attempt, describeRetryReason(msg.status))
		} else {
			m.embedRetryInfo = ""
		}

	case watchUIRPGMsg:
		if strings.Contains(msg.step, "node") || msg.step == "rpg-nodes" {
			m.progress.setRPGNodeProgress(msg.step, msg.current, msg.total)
		} else {
			// edges, chunks, etc.
			// "rpg-edges" is the intermediate step
			// "rpg-chunks" is the last step
			m.progress.setRPGEdgeProgress(msg.step, msg.current, msg.total)
		}

		if msg.total > 0 && m.currentStep < 3 {
			// RPG usually happens after embed, before steady state?
			// Or parallel? Let's assume it contributes to the "Symbol" phase or similar.
			// Actually RPG is built from symbols.
			m.currentStep = 3
		}

	case watchUISymbolMsg:
		m.symbolCount = msg.count
		m.currentStep = 3

	case watchUILedgerMsg:
		source := msg.source
		if source == "" {
			source = m.projectRoot
		}

		entry := ledgerEntry{
			source: source,
			at:     time.Now(),
			level:  msg.level,
			text:   msg.text,
		}
		m.ledger.addEntry(entry)

		if session, ok := m.sessions[source]; ok {
			session.events++
			session.updatedAt = time.Now()
			m.sessions[source] = session
		}

	case watchUIHealthMsg:
		m.totalEvents = msg.totalEvents
		m.lastSuccess = msg.lastSuccess

	case watchUIActivityMsg:
		activity := msg.state
		if msg.file != "" {
			activity += " (" + filepath.Base(msg.file) + ")"
		}
		m.currentActivity = activity

	case watchUIStatsMsg:
		if msg.delta.Snapshot {
			m.applySnapshotStats(msg.projectRoot, msg.delta)
		} else {
			m.applyIncrementalStats(msg.projectRoot, msg.delta)
		}

	case watchUIScopeMsg:
		if msg.totalProjects < 1 {
			msg.totalProjects = 1
		}
		m.totalProjects = msg.totalProjects
		m.recomputeReadyProjects()
		m.ensureSessionFocusValid()

	case watchUIReadyMsg:
		m.upsertSession(msg.projectRoot, "running", "steady")
		m.recomputeReadyProjects()
		m.ensureSessionFocusValid()

	case watchUISessionMsg:
		m.upsertSession(msg.projectRoot, msg.state, msg.note)
		m.recomputeReadyProjects()
		m.ensureSessionFocusValid()
		if msg.state == "removed" {
			cmds = append(cmds, watchUIPruneCmd())
		}

	case watchUIErrorMsg:
		m.err = msg.err
		m.ledger.addEntry(ledgerEntry{
			source: m.projectRoot,
			at:     time.Now(),
			level:  "error",
			text:   msg.err.Error(),
		})

	case watchUIDoneMsg:
		m.done = true
		return m, tea.Quit

	case watchUIPruneMsg:
		if m.pruneRemovedSessions(msg.at) {
			cmds = append(cmds, watchUIPruneCmd())
		}
	}

	m.syncLedgerFilter()

	// Update components
	m.ledger, cmd = m.ledger.Update(msg)
	cmds = append(cmds, cmd)

	m.progress, cmd = m.progress.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *watchUIModel) applyStatsDelta(delta watchStatsDelta) {
	m.filesIndexed += delta.FilesIndexed
	m.filesRemoved += delta.FilesRemoved
	m.chunksCreated += delta.ChunksCreated - delta.ChunksRemoved
	m.symbolCount += delta.SymbolsFound - delta.SymbolsLost
}

func (m *watchUIModel) applyIncrementalStats(projectRoot string, delta watchStatsDelta) {
	m.applyStatsDelta(delta)

	root := projectRoot
	if root == "" {
		root = m.projectRoot
	}
	if root == "" {
		return
	}

	drift := m.snapshotDrift[root]
	drift.FilesIndexed += delta.FilesIndexed
	drift.FilesRemoved += delta.FilesRemoved
	drift.ChunksCreated += delta.ChunksCreated
	drift.ChunksRemoved += delta.ChunksRemoved
	drift.SymbolsFound += delta.SymbolsFound
	drift.SymbolsLost += delta.SymbolsLost
	m.snapshotDrift[root] = drift
}

func (m *watchUIModel) applySnapshotContribution(delta watchStatsDelta, sign int) {
	m.filesIndexed += sign * delta.FilesIndexed
	m.filesRemoved += sign * delta.FilesRemoved
	m.chunksCreated += sign * (delta.ChunksCreated - delta.ChunksRemoved)
	m.symbolCount += sign * (delta.SymbolsFound - delta.SymbolsLost)
}

func (m *watchUIModel) applySnapshotStats(projectRoot string, delta watchStatsDelta) {
	root := projectRoot
	if root == "" {
		root = m.projectRoot
	}
	if root == "" {
		// Fallback to incremental semantics when no source is available.
		m.applyStatsDelta(delta)
		return
	}

	if prev, ok := m.snapshots[root]; ok {
		m.applySnapshotContribution(prev, -1)
	}

	// Rebase incremental activity since the previous snapshot for this project.
	// The refreshed snapshot already includes these changes.
	if drift, ok := m.snapshotDrift[root]; ok {
		m.applySnapshotContribution(drift, -1)
	}

	m.applySnapshotContribution(delta, 1)
	m.snapshots[root] = delta
	delete(m.snapshotDrift, root)
}

func (m *watchUIModel) clearProjectStats(projectRoot string) {
	if projectRoot == "" {
		return
	}

	if snapshot, ok := m.snapshots[projectRoot]; ok {
		m.applySnapshotContribution(snapshot, -1)
		delete(m.snapshots, projectRoot)
	}

	if drift, ok := m.snapshotDrift[projectRoot]; ok {
		m.applySnapshotContribution(drift, -1)
		delete(m.snapshotDrift, projectRoot)
	}
}

func (m *watchUIModel) recalculateLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	leftW := int(float64(m.width-2) * 0.68)
	if leftW < 40 {
		leftW = m.width - 2
	}
	rightW := (m.width - 2) - leftW
	if rightW < 24 {
		rightW = 24
		if leftW+rightW > m.width-2 {
			leftW = (m.width - 2) - rightW
		}
	}

	// Layout logic:
	// Header: approx 5 lines? We need to measure renders really, but let's assume fixed.
	// RenderFooter: 1 line + padding
	// Header ~4 lines
	// LifecycleRail ~1 line
	// Help/Footer ~3 lines
	// Total chrome ~8 lines.
	// Use explicit calculations if possible, but safe estimate:
	availableHeight := m.height - 11

	_, bottomHeight := panelHeights(availableHeight)

	m.ledger.setSize(leftW-2, bottomHeight-2) // -2 for borders
	m.progress.setSize(leftW - 2)
}

func (m watchUIModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading watch UI..."
	}

	top := m.renderHeader()
	rail := m.theme.panel.Width(m.width - 2).Render(renderLifecycleRail(m.theme, m.phases, m.currentStep))
	content := m.renderMainPanels()
	help := m.renderFooter()

	if m.showHelp {
		controls := "q: graceful stop | p: pause ledger | ?: toggle help"
		if m.totalProjects > 1 {
			controls += " | tab: cycle session focus"
		}
		helpCard := renderActionCard(
			m.theme,
			"Controls",
			"Interactive watch controls",
			controls,
			m.width-2,
		)
		return m.theme.canvas.Render(lipgloss.JoinVertical(lipgloss.Left, top, rail, content, helpCard, help))
	}

	return m.theme.canvas.Render(lipgloss.JoinVertical(lipgloss.Left, top, rail, content, help))
}

func (m *watchUIModel) upsertSession(projectRoot, state, note string) {
	if projectRoot == "" {
		return
	}
	session, ok := m.sessions[projectRoot]
	if !ok {
		session = watchUISessionState{
			projectRoot: projectRoot,
			label:       watchSessionLabel(projectRoot),
		}
		m.sessionOrder = append(m.sessionOrder, projectRoot)
	}
	session.state = state
	session.note = note
	session.updatedAt = time.Now()
	if state == "removed" {
		m.clearProjectStats(projectRoot)
		session.removedAt = session.updatedAt
	} else {
		session.removedAt = time.Time{}
	}
	m.sessions[projectRoot] = session
}

func (m *watchUIModel) cycleSessionFocus() {
	if m.totalProjects <= 1 || len(m.sessionOrder) == 0 {
		m.sessionFocus = -1
		return
	}
	maxFocus := len(m.sessionOrder) - 1
	if m.sessionFocus < maxFocus {
		m.sessionFocus++
		return
	}
	m.sessionFocus = -1
}

func (m *watchUIModel) recomputeReadyProjects() {
	ready := 0
	for _, session := range m.sessions {
		if session.state == "running" {
			ready++
		}
	}
	if ready > m.totalProjects {
		ready = m.totalProjects
	}
	if ready < 0 {
		ready = 0
	}
	m.readyProjects = ready
}

func (m *watchUIModel) ensureSessionFocusValid() {
	if m.totalProjects <= 1 {
		m.sessionFocus = -1
		return
	}
	if m.sessionFocus < 0 {
		return
	}
	if m.sessionFocus >= len(m.sessionOrder) {
		m.sessionFocus = -1
		return
	}
	root := m.sessionOrder[m.sessionFocus]
	session, exists := m.sessions[root]
	if !exists {
		m.sessionFocus = -1
		return
	}
	if session.state == "removed" {
		m.sessionFocus = -1
	}
}

func (m *watchUIModel) syncLedgerFilter() {
	m.ledger.setSourceFilter(m.selectedSessionRoot())
}

func (m *watchUIModel) pruneRemovedSessions(now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}

	removed := make(map[string]bool)
	hasPendingRemoved := false
	for root, session := range m.sessions {
		if session.state != "removed" || session.removedAt.IsZero() {
			continue
		}
		if now.Sub(session.removedAt) >= removedSessionTTL {
			removed[root] = true
			continue
		}
		hasPendingRemoved = true
	}

	if len(removed) == 0 {
		return hasPendingRemoved
	}

	for root := range removed {
		m.clearProjectStats(root)
		delete(m.sessions, root)
	}
	filtered := make([]string, 0, len(m.sessionOrder))
	for _, root := range m.sessionOrder {
		if removed[root] {
			continue
		}
		filtered = append(filtered, root)
	}
	m.sessionOrder = filtered
	m.ensureSessionFocusValid()
	m.recomputeReadyProjects()

	return hasPendingRemoved
}

func (m watchUIModel) selectedSessionRoot() string {
	if m.sessionFocus < 0 || m.sessionFocus >= len(m.sessionOrder) {
		return ""
	}
	return m.sessionOrder[m.sessionFocus]
}

func (m watchUIModel) selectedSessionLabel() string {
	root := m.selectedSessionRoot()
	if root == "" {
		return "all sessions"
	}
	if session, ok := m.sessions[root]; ok && session.label != "" {
		return session.label
	}
	return watchSessionLabel(root)
}

func watchSessionLabel(path string) string {
	if path == watchUILogSystem {
		return "system"
	}
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	parent := filepath.Base(filepath.Dir(clean))
	if base == "." || base == string(filepath.Separator) {
		return clean
	}
	if parent == "." || parent == string(filepath.Separator) || parent == base {
		return base
	}
	return parent + "/" + base
}

func (m watchUIModel) renderHeader() string {
	uptime := time.Since(m.started).Round(time.Second)
	title := m.theme.title.Render("grepai watch")
	meta := m.theme.muted.Render(fmt.Sprintf(
		"project=%s  scope=%d/%d ready  focus=%s",
		m.projectRoot,
		m.readyProjects,
		m.totalProjects,
		m.selectedSessionLabel(),
	))
	info := m.theme.text.Render(fmt.Sprintf("provider=%s/%s  backend=%s  rpg=%s  uptime=%s",
		m.provider, m.model, m.backend, m.rpg, uptime))
	if m.stopping {
		info = m.theme.warn.Render(info)
	}
	if m.err != nil {
		info = m.theme.danger.Render(info)
	}
	return m.theme.panel.Width(m.width - 2).Render(lipgloss.JoinVertical(lipgloss.Left, title, meta, info))
}

func (m watchUIModel) renderMainPanels() string {
	leftW := int(float64(m.width-2) * 0.68)
	if leftW < 40 {
		leftW = m.width - 2
	}
	rightW := (m.width - 2) - leftW
	if rightW < 24 {
		rightW = 24
		if leftW+rightW > m.width-2 {
			leftW = (m.width - 2) - rightW
		}
	}

	topHeight, bottomHeight := panelHeights(m.height - 11)

	leftTop := m.renderProgressPanel(leftW, topHeight)
	leftBottom := m.renderLedgerPanel(leftW, bottomHeight)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, leftTop, leftBottom)

	rightCol := m.renderHealthPanel(rightW, topHeight+bottomHeight)
	if m.totalProjects > 1 {
		sessionHeight := topHeight
		if sessionHeight < 6 {
			sessionHeight = 6
		}
		healthHeight := topHeight + bottomHeight - sessionHeight
		if healthHeight < 6 {
			healthHeight = 6
		}
		sessionPanel := m.renderSessionPanel(rightW, sessionHeight)
		healthPanel := m.renderHealthPanel(rightW, healthHeight)
		rightCol = lipgloss.JoinVertical(lipgloss.Left, sessionPanel, healthPanel)
	}

	if leftW <= 0 {
		return rightCol
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
}

func (m watchUIModel) renderProgressPanel(width, height int) string {
	lines := []string{
		m.theme.subtitle.Render("Lifecycle Snapshot"),
	}

	// Delegate bars to progress component
	lines = append(lines, m.progress.View())

	// Add other stats
	lines = append(lines,
		m.theme.text.Render(fmt.Sprintf("Symbol: %d extracted", m.symbolCount)),
		m.theme.text.Render(fmt.Sprintf("Stats: indexed=%d chunks=%d removed=%d",
			m.filesIndexed, m.chunksCreated, m.filesRemoved)),
	)

	if m.scanFile != "" {
		lines = append(lines, m.theme.muted.Render("Current file: "+truncateRunes(m.scanFile, width-6)))
	}
	if m.currentActivity != "" {
		lines = append(lines, m.theme.info.Render("Activity: "+truncateRunes(m.currentActivity, width-6)))
	}
	if m.embedRetryInfo != "" {
		lines = append(lines, m.theme.warn.Render("Embed "+m.embedRetryInfo))
	}
	return m.theme.panel.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m watchUIModel) renderLedgerPanel(width, height int) string {
	label := m.theme.subtitle.Render("Event Ledger · " + m.selectedSessionLabel())
	if m.ledger.paused {
		label += m.theme.warn.Render(" [paused]")
	}

	content := m.ledger.View()
	return m.theme.panel.Width(width).Height(height).Render(lipgloss.JoinVertical(lipgloss.Left, label, content))
}

func (m watchUIModel) renderSessionPanel(width, height int) string {
	lines := []string{m.theme.subtitle.Render("Worktree Sessions")}
	lines = append(lines, m.theme.muted.Render("tab cycles focus (all -> session...)"))

	if len(m.sessionOrder) == 0 {
		lines = append(lines, m.theme.muted.Render("No sessions yet"))
		return m.theme.panel.Width(width).Height(height).Render(strings.Join(lines, "\n"))
	}

	allMarker := " "
	if m.selectedSessionRoot() == "" {
		allMarker = ">"
	}
	lines = append(lines, fmt.Sprintf("%s %s", allMarker, m.theme.text.Render("all sessions")))

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	visibleRows := maxRows - 1 // reserve one row for "all sessions"
	if visibleRows < 0 {
		visibleRows = 0
	}

	orderedRoots := make([]string, 0, len(m.sessionOrder))
	for _, root := range m.sessionOrder {
		if _, ok := m.sessions[root]; ok {
			orderedRoots = append(orderedRoots, root)
		}
	}

	selectedRoot := m.selectedSessionRoot()
	selectedIndex := -1
	for idx, root := range orderedRoots {
		if root == selectedRoot {
			selectedIndex = idx
			break
		}
	}

	start := 0
	if visibleRows > 0 && selectedIndex >= visibleRows {
		start = selectedIndex - visibleRows + 1
	}
	end := len(orderedRoots)
	if visibleRows > 0 && start+visibleRows < end {
		end = start + visibleRows
	}

	for _, root := range orderedRoots[start:end] {
		session, ok := m.sessions[root]
		if !ok {
			continue
		}
		marker := " "
		if m.selectedSessionRoot() == root {
			marker = ">"
		}
		stateStyle := m.theme.info
		switch session.state {
		case "running":
			stateStyle = m.theme.ok
		case "retrying":
			stateStyle = m.theme.warn
		case "error":
			stateStyle = m.theme.danger
		case "removed", "stopped":
			stateStyle = m.theme.warn
		}
		line := fmt.Sprintf(
			"%s %s %s e=%d",
			marker,
			stateStyle.Render(strings.ToUpper(session.state)),
			truncateRunes(session.label, width-24),
			session.events,
		)
		lines = append(lines, line)
	}

	return m.theme.panel.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m watchUIModel) renderHealthPanel(width, height int) string {
	lastSuccess := "n/a"
	if !m.lastSuccess.IsZero() {
		lastSuccess = m.lastSuccess.Format("15:04:05")
	}

	state := m.theme.ok.Render("steady")
	if m.stopping {
		state = m.theme.warn.Render("stopping")
	}
	if m.err != nil {
		state = m.theme.danger.Render("error")
	}
	if m.currentStep < len(m.phases)-1 {
		state = m.theme.info.Render(strings.ToLower(m.phases[m.currentStep]))
	}

	lines := []string{
		m.theme.subtitle.Render("Health"),
		m.theme.text.Render("State: " + state),
		m.theme.text.Render(fmt.Sprintf("Watch scope: %d/%d ready", m.readyProjects, m.totalProjects)),
		m.theme.text.Render(fmt.Sprintf("Total events: %d", m.totalEvents)),
		m.theme.text.Render(fmt.Sprintf("Last success: %s", lastSuccess)),
		m.theme.text.Render(fmt.Sprintf("Indexed files: %d", m.filesIndexed)),
		m.theme.text.Render(fmt.Sprintf("Chunks created: %d", m.chunksCreated)),
		m.theme.text.Render(fmt.Sprintf("Symbols: %d", m.symbolCount)),
	}

	if m.err != nil {
		lines = append(lines, m.theme.danger.Render("Error: "+truncateRunes(m.err.Error(), width-8)))
	}
	return m.theme.panel.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m watchUIModel) renderFooter() string {
	parts := []string{
		m.theme.help.Render("q stop"),
		m.theme.help.Render("p pause ledger"),
		m.theme.help.Render("? help"),
	}
	if m.totalProjects > 1 {
		parts = append(parts, m.theme.help.Render("tab session"))
		parts = append(parts, m.theme.muted.Render("focus="+m.selectedSessionLabel()))
	}
	retrying := 0
	removed := 0
	for _, session := range m.sessions {
		if session.state == "retrying" {
			retrying++
		}
		if session.state == "removed" {
			removed++
		}
	}
	if retrying > 0 {
		parts = append(parts, m.theme.warn.Render(fmt.Sprintf("retrying=%d", retrying)))
	}
	if removed > 0 {
		parts = append(parts, m.theme.warn.Render(fmt.Sprintf("removed=%d", removed)))
	}
	if m.ledger.paused {
		parts = append(parts, m.theme.warn.Render("ledger paused"))
	}
	return m.theme.panel.Width(m.width - 2).Render(strings.Join(parts, "  |  "))
}

func runWatchForegroundUI() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	model := newWatchUIModel(cancel)
	p := tea.NewProgram(model, tea.WithAltScreen())

	workerErrCh := make(chan error, 1)
	go func() {
		workerErrCh <- runWatchUIWorker(ctx, p)
	}()

	_, runErr := p.Run()
	cancel()
	workerErr := <-workerErrCh

	if runErr != nil {
		return runErr
	}
	if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
		return workerErr
	}
	return nil
}

func sendWatchUILedger(p *tea.Program, source, level, text string) {
	p.Send(watchUILedgerMsg{
		source: source,
		level:  level,
		text:   text,
	})
}

func watchUIPruneCmd() tea.Cmd {
	return tea.Tick(watchUIPrunePeriod, func(at time.Time) tea.Msg {
		return watchUIPruneMsg{at: at}
	})
}

type watchUILogForwarder struct {
	p             *tea.Program
	resolveSource func(line string) string
	mu            sync.Mutex
	pending       string
}

func (w *watchUILogForwarder) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pending += string(p)
	for {
		newline := strings.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		line := strings.TrimSpace(w.pending[:newline])
		w.pending = w.pending[newline+1:]
		w.emitLine(line)
	}
	return len(p), nil
}

func (w *watchUILogForwarder) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	line := strings.TrimSpace(w.pending)
	w.pending = ""
	w.emitLine(line)
}

func (w *watchUILogForwarder) emitLine(line string) {
	if line == "" {
		return
	}
	source := watchUILogSystem
	if w.resolveSource != nil {
		if resolved := w.resolveSource(line); resolved != "" {
			source = resolved
		}
	}
	sendWatchUILedger(w.p, source, watchUILogLevel(line), line)
}

func watchUILogLevel(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
		return "error"
	}
	if strings.Contains(lower, "warn") {
		return "warn"
	}
	return "info"
}

func newWatchUILogSourceResolver(initialRoots ...string) (func(projectRoot string), func(line string) string) {
	var mu sync.RWMutex
	known := make(map[string]bool, len(initialRoots))
	for _, root := range initialRoots {
		if root == "" {
			continue
		}
		known[root] = true
	}

	register := func(projectRoot string) {
		if projectRoot == "" {
			return
		}
		mu.Lock()
		known[projectRoot] = true
		mu.Unlock()
	}

	resolve := func(line string) string {
		mu.RLock()
		defer mu.RUnlock()
		match := ""
		for root := range known {
			if strings.Contains(line, root) && len(root) > len(match) {
				match = root
			}
		}
		if match == "" {
			return watchUILogSystem
		}
		return match
	}

	return register, resolve
}

func captureWatchUILogs(p *tea.Program, resolveSource func(line string) string) func() {
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()

	forwarder := &watchUILogForwarder{
		p:             p,
		resolveSource: resolveSource,
	}
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(forwarder)

	return func() {
		forwarder.flush()
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}
}

func runWatchUIWorker(ctx context.Context, p *tea.Program) (err error) {
	defer func() {
		if err != nil {
			p.Send(watchUIErrorMsg{err: err})
		}
		p.Send(watchUIDoneMsg{})
	}()

	p.Send(watchUIPhaseMsg{current: 0})

	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	initialLinked := discoverWorktreesForWatch(projectRoot)

	registerLogSource, resolveLogSource := newWatchUILogSourceResolver(projectRoot)
	for _, linkedRoot := range initialLinked {
		registerLogSource(linkedRoot)
	}

	restoreLogs := captureWatchUILogs(p, resolveLogSource)
	defer restoreLogs()

	rpgState := "disabled"
	if cfg.RPG.Enabled {
		rpgState = "enabled"
	}
	p.Send(watchUIContextMsg{
		projectRoot: projectRoot,
		provider:    cfg.Embedder.Provider,
		model:       cfg.Embedder.Model,
		backend:     cfg.Store.Backend,
		rpg:         rpgState,
	})
	sendWatchUILedger(p, projectRoot, "info", "Starting watcher runtime")
	p.Send(watchUIPhaseMsg{current: 1})

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	emb, err := initializeEmbedder(watchCtx, cfg)
	if err != nil {
		return err
	}
	defer emb.Close()

	totalEvents := 0
	var lastSuccess time.Time
	var healthMu sync.Mutex
	emitHealth := func() {
		healthMu.Lock()
		defer healthMu.Unlock()
		p.Send(watchUIHealthMsg{
			totalEvents: totalEvents,
			lastSuccess: lastSuccess,
		})
	}

	return runDynamicWatchSupervisor(
		watchCtx,
		projectRoot,
		emb,
		withWatchSupervisorBackgroundChild(true),
		withWatchSupervisorInitialLinkedWorktrees(initialLinked),
		withWatchSupervisorScopeObserver(func(totalProjects int) {
			p.Send(watchUIScopeMsg{totalProjects: totalProjects})
		}),
		withWatchSupervisorInitialReadyObserver(func(totalProjects int) {
			sendWatchUILedger(
				p,
				projectRoot,
				"ok",
				fmt.Sprintf("Watching %d project(s) for changes", totalProjects),
			)
			p.Send(watchUIPhaseMsg{current: 4})
		}),
		withWatchSupervisorLifecycleObserver(func(root, state, note string) {
			registerLogSource(root)
			p.Send(watchUISessionMsg{
				projectRoot: root,
				state:       state,
				note:        note,
			})

			switch state {
			case "starting":
				sendWatchUILedger(p, root, "info", "Session starting")
			case "running":
				p.Send(watchUIReadyMsg{projectRoot: root})
				sendWatchUILedger(p, root, "ok", "Session running")
			case "retrying":
				sendWatchUILedger(p, root, "warn", "Retry scheduled: "+note)
			case "error":
				sendWatchUILedger(p, root, "error", note)
			case "removed":
				sendWatchUILedger(p, root, "warn", "Session removed")
			case "stopped":
				sendWatchUILedger(p, root, "warn", "Session stopped")
			}
		}),
		withWatchSupervisorEventObserver(func(sourceRoot string, event watcher.FileEvent) {
			sendWatchUILedger(p, sourceRoot, "info", fmt.Sprintf("[%s] %s", event.Type.String(), event.Path))
			healthMu.Lock()
			totalEvents++
			lastSuccess = time.Now()
			healthMu.Unlock()
			emitHealth()
		}),
		withWatchSupervisorScanObserver(func(current, total int, file string) {
			p.Send(watchUIScanMsg{
				current: current,
				total:   total,
				file:    file,
			})
			if total > 0 && current < total {
				p.Send(watchUIPhaseMsg{current: 1}) // Scanning
			}
		}),
		withWatchSupervisorEmbedObserver(func(info indexer.BatchProgressInfo) {
			p.Send(watchUIEmbedMsg{
				completed: info.CompletedChunks,
				total:     info.TotalChunks,
			})
			if info.TotalChunks > 0 && info.CompletedChunks < info.TotalChunks {
				p.Send(watchUIPhaseMsg{current: 2}) // Embedding
			}
		}),
		withWatchSupervisorRPGObserver(func(step string, current, total int) {
			p.Send(watchUIRPGMsg{
				step:    step,
				current: current,
				total:   total,
			})
		}),
		withWatchSupervisorActivityObserver(func(state, file string) {
			p.Send(watchUIActivityMsg{
				state: state,
				file:  file,
			})
		}),
		withWatchSupervisorStatsObserver(func(projectRoot string, delta watchStatsDelta) {
			p.Send(watchUIStatsMsg{
				projectRoot: projectRoot,
				delta:       delta,
			})
		}),
	)
}

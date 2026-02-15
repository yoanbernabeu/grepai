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
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
)

const watchLedgerLimit = 300

type watchUILedgerEntry struct {
	at    time.Time
	level string
	text  string
}

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

type watchUISummaryMsg struct {
	filesIndexed  int
	filesSkipped  int
	chunksCreated int
	filesRemoved  int
}

type watchUISymbolMsg struct {
	count int
}

type watchUILedgerMsg struct {
	level string
	text  string
}

type watchUIHealthMsg struct {
	totalEvents int
	lastSuccess time.Time
}

type watchUIErrorMsg struct {
	err error
}

type watchUIDoneMsg struct{}

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

	scanCurrent int
	scanTotal   int
	scanFile    string

	embedCompleted int
	embedTotal     int
	embedRetryInfo string

	filesIndexed  int
	filesSkipped  int
	chunksCreated int
	filesRemoved  int
	symbolCount   int

	totalEvents int
	lastSuccess time.Time

	events []watchUILedgerEntry

	paused   bool
	showHelp bool
	stopping bool
	done     bool
	err      error
	started  time.Time
}

func newWatchUIModel(cancel context.CancelFunc) watchUIModel {
	return watchUIModel{
		theme:       newTUITheme(),
		cancel:      cancel,
		phases:      []string{"Preflight", "Scan", "Embed", "Symbol", "Steady"},
		currentStep: 0,
		events:      make([]watchUILedgerEntry, 0, watchLedgerLimit),
		started:     time.Now(),
	}
}

func (m watchUIModel) Init() tea.Cmd {
	return nil
}

func (m watchUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancel != nil && !m.stopping {
				m.stopping = true
				m.cancel()
				m.events = append(m.events, watchUILedgerEntry{
					at:    time.Now(),
					level: "warn",
					text:  "Stopping watcher and persisting index...",
				})
			}
		case "p":
			m.paused = !m.paused
		case "?":
			m.showHelp = !m.showHelp
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
		m.scanCurrent = msg.current
		m.scanTotal = msg.total
		m.scanFile = msg.file
		if msg.total > 0 {
			m.currentStep = 1
		}
	case watchUIEmbedMsg:
		m.embedCompleted = msg.completed
		m.embedTotal = msg.total
		if msg.total > 0 {
			m.currentStep = 2
		}
		if msg.retrying {
			m.embedRetryInfo = fmt.Sprintf("retry batch attempt %d (%s)", msg.attempt, describeRetryReason(msg.status))
		} else {
			m.embedRetryInfo = ""
		}
	case watchUISummaryMsg:
		m.filesIndexed = msg.filesIndexed
		m.filesSkipped = msg.filesSkipped
		m.chunksCreated = msg.chunksCreated
		m.filesRemoved = msg.filesRemoved
	case watchUISymbolMsg:
		m.symbolCount = msg.count
		m.currentStep = 3
	case watchUILedgerMsg:
		if m.paused {
			return m, nil
		}
		m.events = append(m.events, watchUILedgerEntry{
			at:    time.Now(),
			level: msg.level,
			text:  msg.text,
		})
		if len(m.events) > watchLedgerLimit {
			m.events = m.events[len(m.events)-watchLedgerLimit:]
		}
	case watchUIHealthMsg:
		m.totalEvents = msg.totalEvents
		m.lastSuccess = msg.lastSuccess
	case watchUIErrorMsg:
		m.err = msg.err
		m.events = append(m.events, watchUILedgerEntry{
			at:    time.Now(),
			level: "error",
			text:  msg.err.Error(),
		})
	case watchUIDoneMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
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
		helpCard := renderActionCard(
			m.theme,
			"Controls",
			"Interactive watch controls",
			"q: graceful stop | p: pause ledger | ?: toggle help",
			m.width-2,
		)
		return m.theme.canvas.Render(lipgloss.JoinVertical(lipgloss.Left, top, rail, content, helpCard, help))
	}

	return m.theme.canvas.Render(lipgloss.JoinVertical(lipgloss.Left, top, rail, content, help))
}

func (m watchUIModel) renderHeader() string {
	uptime := time.Since(m.started).Round(time.Second)
	title := m.theme.title.Render("grepai watch")
	meta := m.theme.muted.Render(fmt.Sprintf("project=%s", m.projectRoot))
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

	if leftW <= 0 {
		return rightCol
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
}

func (m watchUIModel) renderProgressPanel(width, height int) string {
	lines := []string{
		m.theme.subtitle.Render("Lifecycle Snapshot"),
		m.theme.text.Render(fmt.Sprintf("Scan:   %d/%d", m.scanCurrent, m.scanTotal)),
		m.theme.text.Render(fmt.Sprintf("Embed:  %d/%d", m.embedCompleted, m.embedTotal)),
		m.theme.text.Render(fmt.Sprintf("Symbol: %d extracted", m.symbolCount)),
		m.theme.text.Render(fmt.Sprintf("Stats: indexed=%d chunks=%d removed=%d skipped=%d",
			m.filesIndexed, m.chunksCreated, m.filesRemoved, m.filesSkipped)),
	}
	if m.scanFile != "" {
		lines = append(lines, m.theme.muted.Render("Current file: "+truncateRunes(m.scanFile, width-6)))
	}
	if m.embedRetryInfo != "" {
		lines = append(lines, m.theme.warn.Render("Embed "+m.embedRetryInfo))
	}
	return m.theme.panel.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m watchUIModel) renderLedgerPanel(width, height int) string {
	lines := []string{m.theme.subtitle.Render("Event Ledger")}
	if m.paused {
		lines = append(lines, m.theme.warn.Render("[paused]"))
	}

	entries := m.events
	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	if len(entries) > maxRows {
		entries = entries[len(entries)-maxRows:]
	}

	if len(entries) == 0 {
		lines = append(lines, m.theme.muted.Render("No events yet"))
	} else {
		for _, ev := range entries {
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
			lines = append(lines, fmt.Sprintf("%s %s %s",
				m.theme.muted.Render(ts),
				levelStyle.Render(strings.ToUpper(ev.level)),
				truncateRunes(ev.text, width-22)))
		}
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
	if m.paused {
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
	p.Send(watchUILedgerMsg{level: "info", text: "Starting watcher runtime"})

	emb, err := initializeEmbedder(ctx, cfg)
	if err != nil {
		return err
	}
	defer emb.Close()

	st, err := initializeStore(ctx, cfg, projectRoot)
	if err != nil {
		return err
	}
	defer st.Close()

	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, cfg.Ignore, cfg.ExternalGitignore)
	if err != nil {
		return fmt.Errorf("failed to initialize ignore matcher: %w", err)
	}
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)
	chunker := indexer.NewChunker(cfg.Chunking.Size, cfg.Chunking.Overlap)
	idx := indexer.NewIndexer(projectRoot, st, emb, chunker, scanner, cfg.Watch.LastIndexTime)

	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		log.Printf("Warning: failed to load symbol index for %s: %v", projectRoot, err)
	}
	defer symbolStore.Close()

	extractor := trace.NewRegexExtractor()
	tracedLanguages := cfg.Trace.EnabledLanguages
	if len(tracedLanguages) == 0 {
		tracedLanguages = []string{".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".php", ".java", ".cs"}
	}

	p.Send(watchUIPhaseMsg{current: 1})
	p.Send(watchUILedgerMsg{level: "info", text: "Running initial scan"})

	stats, err := idx.IndexAllWithBatchProgress(ctx,
		func(info indexer.ProgressInfo) {
			p.Send(watchUIScanMsg{
				current: info.Current,
				total:   info.Total,
				file:    info.CurrentFile,
			})
		},
		func(info indexer.BatchProgressInfo) {
			p.Send(watchUIEmbedMsg{
				completed: info.CompletedChunks,
				total:     info.TotalChunks,
				retrying:  info.Retrying,
				attempt:   info.Attempt,
				status:    info.StatusCode,
			})
			if info.Retrying {
				p.Send(watchUILedgerMsg{
					level: "warn",
					text:  fmt.Sprintf("Embedding retry: batch=%d attempt=%d status=%d", info.BatchIndex+1, info.Attempt, info.StatusCode),
				})
			}
		},
	)
	if err != nil {
		return fmt.Errorf("initial indexing failed: %w", err)
	}
	p.Send(watchUISummaryMsg{
		filesIndexed:  stats.FilesIndexed,
		filesSkipped:  stats.FilesSkipped,
		chunksCreated: stats.ChunksCreated,
		filesRemoved:  stats.FilesRemoved,
	})
	p.Send(watchUILedgerMsg{
		level: "ok",
		text: fmt.Sprintf("Initial scan complete: indexed=%d chunks=%d removed=%d skipped=%d",
			stats.FilesIndexed, stats.ChunksCreated, stats.FilesRemoved, stats.FilesSkipped),
	})

	if stats.FilesIndexed > 0 || stats.ChunksCreated > 0 {
		cfg.Watch.LastIndexTime = time.Now()
		if err := cfg.Save(projectRoot); err != nil {
			log.Printf("Warning: failed to save config: %v", err)
			p.Send(watchUILedgerMsg{level: "warn", text: "Failed to save updated watch timestamp"})
		}
	}

	p.Send(watchUIPhaseMsg{current: 3})
	p.Send(watchUILedgerMsg{level: "info", text: "Building symbol index"})
	symbolCount := 0
	files, _, scanErr := scanner.ScanMetadata()
	if scanErr != nil {
		log.Printf("Warning: failed to scan files for symbol index: %v", scanErr)
		p.Send(watchUILedgerMsg{level: "warn", text: fmt.Sprintf("Symbol scan warning: %v", scanErr)})
	} else {
		for _, file := range files {
			ext := strings.ToLower(filepath.Ext(file.Path))
			if !isTracedLanguage(ext, tracedLanguages) {
				continue
			}
			if !cfg.Watch.LastIndexTime.IsZero() {
				fileModTime := time.Unix(file.ModTime, 0)
				if (fileModTime.Before(cfg.Watch.LastIndexTime) || fileModTime.Equal(cfg.Watch.LastIndexTime)) && symbolStore.IsFileIndexed(file.Path) {
					continue
				}
			}

			fileInfo, scanFileErr := scanner.ScanFile(file.Path)
			if scanFileErr != nil {
				log.Printf("Warning: failed to scan %s for symbols: %v", file.Path, scanFileErr)
				continue
			}
			if fileInfo == nil {
				continue
			}

			if existingHash, ok := symbolStore.GetFileContentHash(fileInfo.Path); ok && existingHash == fileInfo.Hash {
				continue
			}

			symbols, refs, extractErr := extractor.ExtractAll(ctx, fileInfo.Path, fileInfo.Content)
			if extractErr != nil {
				log.Printf("Warning: failed to extract symbols from %s: %v", fileInfo.Path, extractErr)
				continue
			}
			if saveErr := symbolStore.SaveFileWithContentHash(ctx, fileInfo.Path, fileInfo.Hash, symbols, refs); saveErr != nil {
				log.Printf("Warning: failed to save symbols for %s: %v", fileInfo.Path, saveErr)
				continue
			}
			symbolCount += len(symbols)
		}
	}
	if err := symbolStore.Persist(ctx); err != nil {
		log.Printf("Warning: failed to persist symbol index: %v", err)
	}
	p.Send(watchUISymbolMsg{count: symbolCount})
	p.Send(watchUILedgerMsg{level: "ok", text: fmt.Sprintf("Symbol index built: %d symbols", symbolCount)})

	var rpgIndexer *rpg.RPGIndexer
	var rpgStore rpg.RPGStore
	var rpgManager *rpgRealtimeManager

	if cfg.RPG.Enabled {
		rpgStore = rpg.NewGOBRPGStore(config.GetRPGIndexPath(projectRoot))
		if loadErr := rpgStore.Load(ctx); loadErr != nil {
			log.Printf("Warning: failed to load RPG index for %s: %v", projectRoot, loadErr)
		}

		var featureExtractor rpg.FeatureExtractor
		switch cfg.RPG.FeatureMode {
		case "llm", "hybrid":
			if cfg.RPG.LLMEndpoint == "" || cfg.RPG.LLMModel == "" {
				featureExtractor = rpg.NewLocalExtractor()
				p.Send(watchUILedgerMsg{level: "warn", text: "RPG LLM settings missing, fallback to local extractor"})
			} else {
				featureExtractor = rpg.NewLLMExtractor(rpg.LLMExtractorConfig{
					Provider: cfg.RPG.LLMProvider,
					Model:    cfg.RPG.LLMModel,
					Endpoint: cfg.RPG.LLMEndpoint,
					APIKey:   cfg.RPG.LLMAPIKey,
					Timeout:  time.Duration(cfg.RPG.LLMTimeoutMs) * time.Millisecond,
				})
			}
		default:
			featureExtractor = rpg.NewLocalExtractor()
		}

		rpgIndexer = rpg.NewRPGIndexer(rpgStore, featureExtractor, projectRoot, rpg.RPGIndexerConfig{
			DriftThreshold:       cfg.RPG.DriftThreshold,
			MaxTraversalDepth:    cfg.RPG.MaxTraversalDepth,
			FeatureGroupStrategy: cfg.RPG.FeatureGroupStrategy,
		})
		if buildErr := rpgIndexer.BuildFull(ctx, symbolStore, st); buildErr != nil {
			log.Printf("Warning: failed to build RPG graph for %s: %v", projectRoot, buildErr)
			p.Send(watchUILedgerMsg{level: "warn", text: fmt.Sprintf("RPG build warning: %v", buildErr)})
		} else {
			stats := rpgStore.GetGraph().Stats()
			p.Send(watchUILedgerMsg{
				level: "ok",
				text:  fmt.Sprintf("RPG built: nodes=%d edges=%d", stats.TotalNodes, stats.TotalEdges),
			})
		}
		if persistErr := rpgStore.Persist(ctx); persistErr != nil {
			log.Printf("Warning: failed to persist RPG graph: %v", persistErr)
		}

		rpgManager = newRPGRealtimeManager(cfg.Watch.RPGMaxDirtyFilesPerBatch)
		startRPGRealtimeWorkers(ctx, projectRoot, symbolStore, rpgIndexer, rpgStore, cfg.Watch, rpgManager)
		defer rpgStore.Close()
	}

	if err := st.Persist(ctx); err != nil {
		log.Printf("Warning: failed to persist index: %v", err)
	}

	w, err := watcher.NewWatcher(projectRoot, ignoreMatcher, cfg.Watch.DebounceMs)
	if err != nil {
		return fmt.Errorf("failed to initialize watcher for %s: %w", projectRoot, err)
	}
	defer w.Close()

	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("failed to start watcher for %s: %w", projectRoot, err)
	}

	p.Send(watchUIPhaseMsg{current: 4})
	p.Send(watchUILedgerMsg{level: "ok", text: "Watching for changes"})

	persistTicker := time.NewTicker(30 * time.Second)
	defer persistTicker.Stop()

	var lastConfigWrite time.Time
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

	for {
		select {
		case <-ctx.Done():
			if err := st.Persist(context.Background()); err != nil {
				log.Printf("Warning: failed to persist index on shutdown: %v", err)
			}
			if err := symbolStore.Persist(context.Background()); err != nil {
				log.Printf("Warning: failed to persist symbol index on shutdown: %v", err)
			}
			if rpgStore != nil {
				if err := rpgStore.Persist(context.Background()); err != nil {
					log.Printf("Warning: failed to persist RPG graph on shutdown: %v", err)
				}
			}
			p.Send(watchUILedgerMsg{level: "warn", text: "Watcher stopped"})
			return nil
		case <-persistTicker.C:
			if err := st.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist index: %v", err)
			}
			if err := symbolStore.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist symbol index: %v", err)
			}
			if rpgStore != nil {
				if err := rpgStore.Persist(ctx); err != nil {
					log.Printf("Warning: failed to persist RPG graph: %v", err)
				}
			}
		case event := <-w.Events():
			totalEvents++
			p.Send(watchUILedgerMsg{
				level: "info",
				text:  fmt.Sprintf("[%s] %s", event.Type.String(), event.Path),
			})

			handleFileEvent(
				ctx,
				idx,
				scanner,
				extractor,
				symbolStore,
				rpgIndexer,
				st,
				tracedLanguages,
				projectRoot,
				cfg,
				&lastConfigWrite,
				rpgManager,
				event,
			)
			lastSuccess = time.Now()
			emitHealth()
		}
	}
}

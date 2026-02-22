package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/git"
	"github.com/yoanbernabeu/grepai/indexer"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
	"github.com/yoanbernabeu/grepai/watcher"
	"golang.org/x/sync/errgroup"
)

var (
	watchBackground bool
	watchLogDir     string
	watchStatus     bool
	watchStop       bool
	watchWorkspace  string
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Start the real-time file watcher daemon",
	Long: `Start a background process that monitors file changes and maintains the index.

The watcher will:
- Perform an initial scan comparing disk state with existing index
- Skip unchanged files by comparing modification times (ModTime) for faster subsequent launches
- Index modified and new files
- Monitor filesystem events (create, modify, delete, rename)
- Apply debouncing (500ms) to batch rapid changes
- Handle atomic updates to avoid duplicate vectors

Background mode:
  grepai watch --background              Run in background with default log directory
  grepai watch --background --log-dir /custom/path  Run with custom log directory
  grepai watch --status                  Check if background watcher is running
  grepai watch --stop                    Stop the background watcher

Default log directories:
  Linux:   ~/.local/state/grepai/logs/grepai-watch.log (or $XDG_STATE_HOME)
  macOS:   ~/Library/Logs/grepai/grepai-watch.log
  Windows: %LOCALAPPDATA%\grepai\logs\grepai-watch.log

Log management:
  Logs are not rotated automatically. To prevent disk usage growth, periodically
  truncate or archive the log file:
    echo "" > ~/Library/Logs/grepai/grepai-watch.log  (macOS example)
  Or set up logrotate (Linux) / Newsyslog (macOS) for automatic rotation.`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().BoolVar(&watchBackground, "background", false, "Run in background mode")
	watchCmd.Flags().StringVar(&watchLogDir, "log-dir", "", "Directory for log files (default: OS-specific)")
	watchCmd.Flags().BoolVar(&watchStatus, "status", false, "Show background watcher status")
	watchCmd.Flags().BoolVar(&watchStop, "stop", false, "Stop the background watcher")
	watchCmd.Flags().StringVar(&watchWorkspace, "workspace", "", "Workspace name for multi-project mode")
}

func runWatch(cmd *cobra.Command, args []string) error {
	// Validate mutually exclusive flags
	activeFlags := 0
	if watchBackground {
		activeFlags++
	}
	if watchStatus {
		activeFlags++
	}
	if watchStop {
		activeFlags++
	}
	if activeFlags > 1 {
		return fmt.Errorf("flags --background, --status, and --stop are mutually exclusive")
	}

	// Determine log directory
	logDir := watchLogDir
	if logDir == "" {
		var err error
		logDir, err = daemon.GetDefaultLogDir()
		if err != nil {
			return fmt.Errorf("failed to get default log directory: %w", err)
		}
	}

	// Workspace mode
	if watchWorkspace != "" {
		return runWorkspaceWatch(logDir)
	}

	// Detect worktree
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	gitInfo, gitErr := git.Detect(cwd)
	var worktreeID string
	if gitErr == nil && gitInfo.WorktreeID != "" {
		worktreeID = gitInfo.WorktreeID
	}

	// Handle --status flag
	if watchStatus {
		return showWatchStatus(logDir, worktreeID)
	}

	// Handle --stop flag
	if watchStop {
		return stopWatchDaemon(logDir, worktreeID)
	}

	// Handle --background flag
	if watchBackground {
		return startBackgroundWatch(logDir, worktreeID)
	}

	// Check if already running in background (automatically cleans up stale PIDs)
	var pid int
	if worktreeID != "" {
		pid, err = daemon.GetRunningWorktreePID(logDir, worktreeID)
		if pid == 0 && err == nil {
			// Fallback: check regular PID file for backward compatibility
			pid, err = daemon.GetRunningPID(logDir)
		}
	} else {
		pid, err = daemon.GetRunningPID(logDir)
	}
	if err != nil {
		return fmt.Errorf("failed to check running status: %w", err)
	}
	if pid > 0 {
		return fmt.Errorf("watcher is already running in background (PID %d)\nUse 'grepai watch --stop' to stop it", pid)
	}

	// Run in foreground mode
	return runWatchForeground()
}

func showWatchStatus(logDir, worktreeID string) error {
	// Get running PID (automatically cleans up stale PIDs)
	var pid int
	var err error
	var logFile string

	if worktreeID != "" {
		pid, err = daemon.GetRunningWorktreePID(logDir, worktreeID)
		logFile = daemon.GetWorktreeLogFile(logDir, worktreeID)
	} else {
		pid, err = daemon.GetRunningPID(logDir)
		logFile = filepath.Join(logDir, "grepai-watch.log")
	}

	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	if pid == 0 {
		fmt.Println("Status: not running")
		fmt.Printf("Log directory: %s\n", logDir)
		if worktreeID != "" {
			fmt.Printf("Worktree ID: %s\n", worktreeID)
		}
		return nil
	}

	fmt.Println("Status: running")
	fmt.Printf("PID: %d\n", pid)
	fmt.Printf("Log directory: %s\n", logDir)
	fmt.Printf("Log file: %s\n", logFile)
	if worktreeID != "" {
		fmt.Printf("Worktree ID: %s\n", worktreeID)
	}

	return nil
}

func stopWatchDaemon(logDir, worktreeID string) error {
	// Get running PID (automatically cleans up stale PIDs)
	var pid int
	var err error
	var logFile string

	if worktreeID != "" {
		pid, err = daemon.GetRunningWorktreePID(logDir, worktreeID)
		logFile = daemon.GetWorktreeLogFile(logDir, worktreeID)
	} else {
		pid, err = daemon.GetRunningPID(logDir)
		logFile = filepath.Join(logDir, "grepai-watch.log")
	}

	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	if pid == 0 {
		fmt.Println("No background watcher is running")
		return nil
	}

	fmt.Printf("Stopping background watcher (PID %d)...\n", pid)
	if err := daemon.StopProcess(pid); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Wait for process to stop with timeout
	const shutdownTimeout = 30 * time.Second
	const shutdownPollInterval = 500 * time.Millisecond
	deadline := time.Now().Add(shutdownTimeout)
	lastProgress := time.Now()

	for time.Now().Before(deadline) {
		if !daemon.IsProcessRunning(pid) {
			break
		}

		// Show progress message every 5 seconds
		if time.Since(lastProgress) >= 5*time.Second {
			fmt.Println("Waiting for graceful shutdown...")
			lastProgress = time.Now()
		}

		time.Sleep(shutdownPollInterval)
	}

	// Verify the process actually stopped
	if daemon.IsProcessRunning(pid) {
		return fmt.Errorf("process did not stop within %v\nStill running? Try: kill -9 %d\nOr check logs at: %s",
			shutdownTimeout, pid, logFile)
	}

	// Clean up PID file
	if worktreeID != "" {
		if err := daemon.RemoveWorktreePIDFile(logDir, worktreeID); err != nil {
			return fmt.Errorf("failed to remove PID file: %w", err)
		}
	} else {
		if err := daemon.RemovePIDFile(logDir); err != nil {
			return fmt.Errorf("failed to remove PID file: %w", err)
		}
	}

	fmt.Println("Background watcher stopped")
	return nil
}

func startBackgroundWatch(logDir, worktreeID string) error {
	// Check if already running (automatically cleans up stale PIDs)
	var pid int
	var err error
	var logFile string

	if worktreeID != "" {
		pid, err = daemon.GetRunningWorktreePID(logDir, worktreeID)
		logFile = daemon.GetWorktreeLogFile(logDir, worktreeID)
	} else {
		pid, err = daemon.GetRunningPID(logDir)
		logFile = filepath.Join(logDir, "grepai-watch.log")
	}

	if err != nil {
		return fmt.Errorf("failed to check running status: %w", err)
	}
	if pid > 0 {
		return fmt.Errorf("watcher is already running (PID %d)", pid)
	}

	// Build args for background process (exclude --background flag)
	args := []string{"watch"}
	if watchLogDir != "" {
		args = append(args, "--log-dir", watchLogDir)
	}

	// Spawn background process
	var childPID int
	var exitCh <-chan struct{}
	if worktreeID != "" {
		childPID, exitCh, err = daemon.SpawnWorktreeBackground(logDir, worktreeID, args)
	} else {
		childPID, exitCh, err = daemon.SpawnBackground(logDir, args)
	}
	if err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	// Wait for process to become ready or fail
	// Poll for ready file with timeout, also checking for early child exit
	const startupTimeout = 30 * time.Second
	const pollInterval = 250 * time.Millisecond
	deadline := time.Now().Add(startupTimeout)

	for time.Now().Before(deadline) {
		// Check if ready file exists (initialization succeeded)
		var isReady bool
		if worktreeID != "" {
			isReady = daemon.IsWorktreeReady(logDir, worktreeID)
		} else {
			isReady = daemon.IsReady(logDir)
		}

		if isReady {
			fmt.Printf("Background watcher started (PID %d)\n", childPID)
			fmt.Printf("Logs: %s\n", logFile)
			if worktreeID != "" {
				fmt.Printf("Worktree ID: %s\n", worktreeID)
			}
			fmt.Printf("\nUse 'grepai watch --status' to check status\n")
			fmt.Printf("Use 'grepai watch --stop' to stop the watcher\n")
			return nil
		}

		// Check if child process exited early (detects failures immediately,
		// unlike kill(0) which reports zombies as alive)
		select {
		case <-exitCh:
			return fmt.Errorf("background process failed to start (check logs at %s)", logFile)
		default:
		}

		time.Sleep(pollInterval)
	}

	// Timeout - process is still running but hasn't become ready
	return fmt.Errorf("timeout waiting for process to become ready after %v (check logs at %s)", startupTimeout, logFile)
}

func initializeEmbedder(ctx context.Context, cfg *config.Config) (embedder.Embedder, error) {
	emb, err := embedder.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	type pinger interface {
		Ping(ctx context.Context) error
	}

	switch cfg.Embedder.Provider {
	case "ollama":
		if p, ok := emb.(pinger); ok {
			if err := p.Ping(ctx); err != nil {
				return nil, fmt.Errorf("cannot connect to Ollama: %w\nMake sure Ollama is running and has the %s model", err, cfg.Embedder.Model)
			}
		}
	case "lmstudio":
		if p, ok := emb.(pinger); ok {
			if err := p.Ping(ctx); err != nil {
				return nil, fmt.Errorf("cannot connect to LM Studio: %w\nMake sure LM Studio is running with the %s model loaded", err, cfg.Embedder.Model)
			}
		}
	}

	return emb, nil
}

func initializeStore(ctx context.Context, cfg *config.Config, projectRoot string) (store.VectorStore, error) {
	switch cfg.Store.Backend {
	case "gob":
		indexPath := config.GetIndexPath(projectRoot)
		gobStore := store.NewGOBStore(indexPath)
		if err := gobStore.Load(ctx); err != nil {
			return nil, fmt.Errorf("failed to load index: %w", err)
		}
		return gobStore, nil
	case "postgres":
		return store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.GetDimensions())
	case "qdrant":
		collectionName := cfg.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = store.SanitizeCollectionName(projectRoot)
		}
		return store.NewQdrantStore(ctx, cfg.Store.Qdrant.Endpoint, cfg.Store.Qdrant.Port, cfg.Store.Qdrant.UseTLS, collectionName, cfg.Store.Qdrant.APIKey, cfg.Embedder.GetDimensions())
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", cfg.Store.Backend)
	}
}

const configWriteThrottle = 30 * time.Second
const rpgDerivedFailureThreshold = 3

type rpgRealtimeManager struct {
	mu               sync.Mutex
	dirtyFiles       map[string]struct{}
	dirtyDerived     bool
	dirtyPersist     bool
	forceFull        bool
	lastDerivedRun   time.Time
	lastPersistRun   time.Time
	derivedFailures  int
	maxDirtyFileSize int
}

func newRPGRealtimeManager(maxDirtyFiles int) *rpgRealtimeManager {
	if maxDirtyFiles < 1 {
		maxDirtyFiles = 1
	}
	return &rpgRealtimeManager{
		dirtyFiles:       make(map[string]struct{}),
		maxDirtyFileSize: maxDirtyFiles,
	}
}

func (m *rpgRealtimeManager) MarkFileDirty(filePath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dirtyDerived = true
	m.dirtyPersist = true
	if filePath != "" {
		m.dirtyFiles[filePath] = struct{}{}
	}
	if len(m.dirtyFiles) > m.maxDirtyFileSize {
		m.forceFull = true
		m.dirtyFiles = make(map[string]struct{})
	}
}

func (m *rpgRealtimeManager) MarkPersistDirty() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirtyPersist = true
}

func (m *rpgRealtimeManager) ShouldPersist() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dirtyPersist
}

func (m *rpgRealtimeManager) MarkPersisted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirtyPersist = false
	m.lastPersistRun = time.Now()
}

func (m *rpgRealtimeManager) ScheduleFullReconcile() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forceFull = true
	m.dirtyDerived = true
}

func (m *rpgRealtimeManager) NextDerivedBatch(maxBatch int) ([]string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if maxBatch < 1 {
		maxBatch = 1
	}

	if !m.dirtyDerived && !m.forceFull {
		return nil, false
	}

	if m.forceFull {
		files := make([]string, 0, len(m.dirtyFiles))
		for f := range m.dirtyFiles {
			files = append(files, f)
		}
		m.forceFull = false
		m.dirtyDerived = false
		m.dirtyFiles = make(map[string]struct{})
		return files, true
	}

	files := make([]string, 0, maxBatch)
	for f := range m.dirtyFiles {
		files = append(files, f)
		delete(m.dirtyFiles, f)
		if len(files) >= maxBatch {
			break
		}
	}
	m.dirtyDerived = len(m.dirtyFiles) > 0
	return files, false
}

func (m *rpgRealtimeManager) MarkDerivedSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.derivedFailures = 0
	m.lastDerivedRun = time.Now()
}

func (m *rpgRealtimeManager) MarkDerivedFailure(files []string, full bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.dirtyDerived = true
	if full {
		m.forceFull = true
	}
	for _, f := range files {
		if f != "" {
			m.dirtyFiles[f] = struct{}{}
		}
	}
	m.derivedFailures++
	return m.derivedFailures
}

func (m *rpgRealtimeManager) Snapshot() (dirtyFiles int, dirtyPersist bool, lastDerivedRun, lastPersistRun time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.dirtyFiles), m.dirtyPersist, m.lastDerivedRun, m.lastPersistRun
}

func startRPGRealtimeWorkers(ctx context.Context, projectLabel string, symbolStore trace.SymbolStore, rpgIndexer *rpg.RPGIndexer, rpgStore rpg.RPGStore, watchCfg config.WatchConfig, manager *rpgRealtimeManager) {
	if manager == nil || rpgIndexer == nil || rpgStore == nil || symbolStore == nil {
		return
	}

	go func() {
		derivedTicker := time.NewTicker(time.Duration(watchCfg.RPGDerivedDebounceMs) * time.Millisecond)
		persistTicker := time.NewTicker(time.Duration(watchCfg.RPGPersistIntervalMs) * time.Millisecond)
		reconcileTicker := time.NewTicker(time.Duration(watchCfg.RPGFullReconcileIntervalSec) * time.Second)
		defer derivedTicker.Stop()
		defer persistTicker.Stop()
		defer reconcileTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-reconcileTicker.C:
				manager.ScheduleFullReconcile()
				log.Printf("rpg_full_reconcile_triggered=true project=%s reason=periodic", projectLabel)

			case <-derivedTicker.C:
				changedFiles, full := manager.NextDerivedBatch(watchCfg.RPGMaxDirtyFilesPerBatch)
				if !full && len(changedFiles) == 0 {
					continue
				}

				start := time.Now()
				var err error
				mode := "incremental"
				if full {
					mode = "full"
					err = rpgIndexer.RefreshDerivedEdgesFull(ctx, symbolStore)
				} else {
					err = rpgIndexer.RefreshDerivedEdgesIncremental(ctx, symbolStore, changedFiles)
				}

				if err != nil {
					failures := manager.MarkDerivedFailure(changedFiles, full)
					if failures >= rpgDerivedFailureThreshold {
						manager.ScheduleFullReconcile()
						log.Printf("rpg_full_reconcile_triggered=true project=%s reason=retry_threshold failures=%d", projectLabel, failures)
					}
					dirtyCount, _, _, _ := manager.Snapshot()
					log.Printf("Warning: rpg_derived_refresh_ms=%d project=%s mode=%s changed_files=%d rpg_dirty_files_count=%d err=%v",
						time.Since(start).Milliseconds(),
						projectLabel,
						mode,
						len(changedFiles),
						dirtyCount,
						err,
					)
					continue
				}

				manager.MarkDerivedSuccess()
				manager.MarkPersistDirty()
				dirtyCount, _, _, _ := manager.Snapshot()
				log.Printf("rpg_derived_refresh_ms=%d project=%s mode=%s changed_files=%d rpg_dirty_files_count=%d",
					time.Since(start).Milliseconds(),
					projectLabel,
					mode,
					len(changedFiles),
					dirtyCount,
				)

			case <-persistTicker.C:
				if !manager.ShouldPersist() {
					continue
				}

				start := time.Now()
				if err := rpgStore.Persist(ctx); err != nil {
					log.Printf("Warning: rpg_persist_ms=%d project=%s err=%v", time.Since(start).Milliseconds(), projectLabel, err)
					continue
				}
				manager.MarkPersisted()

				_, _, lastDerived, _ := manager.Snapshot()
				lagMs := int64(0)
				if !lastDerived.IsZero() {
					lagMs = time.Since(lastDerived).Milliseconds()
				}
				log.Printf("rpg_persist_ms=%d project=%s persist_lag_ms=%d", time.Since(start).Milliseconds(), projectLabel, lagMs)
			}
		}
	}()
}

//nolint:unused // Retained for upcoming watch-loop refactor across fg/bg modes.
func runWatchLoop(ctx context.Context, st store.VectorStore, symbolStore *trace.GOBSymbolStore, w *watcher.Watcher, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, tracedLanguages []string, projectRoot string, cfg *config.Config, isBackgroundChild bool) error {
	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	loopStopCh := daemon.StopChannel()

	if !isBackgroundChild {
		fmt.Println("\nWatching for changes... (Press Ctrl+C to stop)")
	} else {
		log.Println("Watching for changes...")
	}

	// Periodic persist ticker
	persistTicker := time.NewTicker(30 * time.Second)
	defer persistTicker.Stop()

	// Config write throttling - only write every 30 seconds at most
	var lastConfigWrite time.Time

	// persistAndExit persists all stores before returning from the event loop.
	persistAndExit := func() error {
		if err := st.Persist(ctx); err != nil {
			log.Printf("Warning: failed to persist index on shutdown: %v", err)
		}
		if err := symbolStore.Persist(ctx); err != nil {
			log.Printf("Warning: failed to persist symbol index on shutdown: %v", err)
		}
		return nil
	}

	// Event loop
	for {
		select {
		case <-sigChan:
			if !isBackgroundChild {
				fmt.Println("\nShutting down...")
			} else {
				log.Println("Shutting down...")
			}
			return persistAndExit()

		case <-loopStopCh:
			log.Println("Stop file detected, shutting down...")
			return persistAndExit()

		case <-persistTicker.C:
			if err := st.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist index: %v", err)
			}
			if err := symbolStore.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist symbol index: %v", err)
			}

		case event := <-w.Events():
			handleFileEvent(ctx, idx, scanner, extractor, symbolStore, nil, nil, tracedLanguages, projectRoot, cfg, &lastConfigWrite, nil, event)
		}
	}
}

func runInitialScan(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, symbolStore *trace.GOBSymbolStore, tracedLanguages []string, lastIndexTime time.Time, isBackgroundChild bool) (*indexer.IndexStats, error) {
	// Initial scan with progress
	if !isBackgroundChild {
		fmt.Println("\nPerforming initial scan...")
	} else {
		log.Println("Performing initial scan...")
	}

	var stats *indexer.IndexStats
	var err error
	if !isBackgroundChild {
		stats, err = idx.IndexAllWithBatchProgress(ctx,
			func(info indexer.ProgressInfo) {
				printProgress(info.Current, info.Total, info.CurrentFile)
			},
			func(info indexer.BatchProgressInfo) {
				printBatchProgress(info)
			},
		)
		// Clear progress line
		fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")
	} else {
		stats, err = idx.IndexAllWithBatchProgress(ctx, nil, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("initial indexing failed: %w", err)
	}

	if !isBackgroundChild {
		fmt.Printf("Initial scan complete: %d files indexed, %d chunks created, %d files removed, %d skipped (took %s)\n",
			stats.FilesIndexed, stats.ChunksCreated, stats.FilesRemoved, stats.FilesSkipped, stats.Duration.Round(time.Millisecond))
	} else {
		log.Printf("Initial scan complete: %d files indexed, %d chunks created, %d files removed, %d skipped (took %s)",
			stats.FilesIndexed, stats.ChunksCreated, stats.FilesRemoved, stats.FilesSkipped, stats.Duration.Round(time.Millisecond))
	}

	// Index symbols for traced languages
	if !isBackgroundChild {
		fmt.Println("Building symbol index...")
	} else {
		log.Println("Building symbol index...")
	}
	symbolCount := 0
	files, _, err := scanner.ScanMetadata()
	if err != nil {
		log.Printf("Warning: failed to scan files for symbol index: %v", err)
		return stats, nil
	}

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Path))
		if !isTracedLanguage(ext, tracedLanguages) {
			continue
		}

		// Skip files that are unchanged since the last index run and already tracked.
		if !lastIndexTime.IsZero() {
			fileModTime := time.Unix(file.ModTime, 0)
			if (fileModTime.Before(lastIndexTime) || fileModTime.Equal(lastIndexTime)) && symbolStore.IsFileIndexed(file.Path) {
				continue
			}
		}

		fileInfo, err := scanner.ScanFile(file.Path)
		if err != nil {
			log.Printf("Warning: failed to scan %s for symbols: %v", file.Path, err)
			continue
		}
		if fileInfo == nil {
			continue
		}

		// Skip extraction when content hash matches what we already persisted.
		if existingHash, ok := symbolStore.GetFileContentHash(fileInfo.Path); ok && existingHash == fileInfo.Hash {
			continue
		}

		symbols, refs, err := extractor.ExtractAll(ctx, fileInfo.Path, fileInfo.Content)
		if err != nil {
			log.Printf("Warning: failed to extract symbols from %s: %v", fileInfo.Path, err)
			continue
		}
		if err := symbolStore.SaveFileWithContentHash(ctx, fileInfo.Path, fileInfo.Hash, symbols, refs); err != nil {
			log.Printf("Warning: failed to save symbols for %s: %v", fileInfo.Path, err)
		}
		symbolCount += len(symbols)
	}
	if err := symbolStore.Persist(ctx); err != nil {
		log.Printf("Warning: failed to persist symbol index: %v", err)
	}
	if !isBackgroundChild {
		fmt.Printf("Symbol index built: %d symbols extracted\n", symbolCount)
	} else {
		log.Printf("Symbol index built: %d symbols extracted", symbolCount)
	}

	return stats, nil
}

// discoverWorktreesForWatch discovers linked worktrees and auto-initializes them.
// Only discovers from the main worktree; returns nil for linked worktrees.
func discoverWorktreesForWatch(projectRoot string) []string {
	projectRootCanonical := canonicalPath(projectRoot)

	gitInfo, err := git.Detect(projectRoot)
	if err != nil {
		return nil
	}

	// Only the main worktree discovers linked worktrees
	if gitInfo.IsWorktree {
		return nil
	}

	// Run git worktree list
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", projectRoot, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var worktrees []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			wtPath := strings.TrimPrefix(line, "worktree ")
			wtPathCanonical := canonicalPath(wtPath)
			// Skip the main worktree itself
			if wtPathCanonical == projectRootCanonical {
				continue
			}
			// Guard against duplicated aliases pointing to the same path.
			if seen[wtPathCanonical] {
				continue
			}
			seen[wtPathCanonical] = true
			// Auto-init .grepai/ if needed (FindProjectRoot does this when called
			// from within the worktree, but we're not in it, so init manually)
			localGrepai := filepath.Join(wtPathCanonical, ".grepai")
			if _, statErr := os.Stat(localGrepai); os.IsNotExist(statErr) {
				// Auto-init from main
				if initErr := config.AutoInitWorktree(wtPathCanonical, projectRootCanonical); initErr != nil {
					log.Printf("Warning: failed to auto-init worktree %s: %v", wtPathCanonical, initErr)
					continue
				}
				log.Printf("Auto-initialized worktree: %s", wtPathCanonical)
			}
			worktrees = append(worktrees, wtPathCanonical)
		}
	}
	return worktrees
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

// watchProject runs the full watch lifecycle for a single project.
// The embedder is shared across all projects to avoid duplicate connections.
// If onReady is non-nil, it is called once after initial indexing and watcher start.
func watchProject(ctx context.Context, projectRoot string, emb embedder.Embedder, isBackgroundChild bool, onReady func()) error {
	// Load configuration
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load config for %s: %w", projectRoot, err)
	}

	log.Printf("Watching project: %s (backend: %s)", projectRoot, cfg.Store.Backend)

	// Initialize store
	st, err := initializeStore(ctx, cfg, projectRoot)
	if err != nil {
		return err
	}
	defer st.Close()

	// Initialize ignore matcher
	ignoreMatcher, err := indexer.NewIgnoreMatcher(projectRoot, cfg.Ignore, cfg.ExternalGitignore)
	if err != nil {
		return fmt.Errorf("failed to initialize ignore matcher: %w", err)
	}

	// Initialize scanner
	scanner := indexer.NewScanner(projectRoot, ignoreMatcher)

	// Initialize chunker
	chunker := indexer.NewChunker(cfg.Chunking.Size, cfg.Chunking.Overlap)

	// Initialize indexer
	idx := indexer.NewIndexer(projectRoot, st, emb, chunker, scanner, cfg.Watch.LastIndexTime)

	// Initialize symbol store and extractor
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(projectRoot))
	if err := symbolStore.Load(ctx); err != nil {
		log.Printf("Warning: failed to load symbol index for %s: %v", projectRoot, err)
	}
	defer symbolStore.Close()

	extractor := trace.NewRegexExtractor()

	// Initialize RPG if enabled.
	var rpgIndexer *rpg.RPGIndexer
	var rpgStore rpg.RPGStore
	if cfg.RPG.Enabled {
		rpgStore = rpg.NewGOBRPGStore(config.GetRPGIndexPath(projectRoot))
		if err := rpgStore.Load(ctx); err != nil {
			log.Printf("Warning: failed to load RPG index for %s: %v", projectRoot, err)
		}

		var featureExtractor rpg.FeatureExtractor
		switch cfg.RPG.FeatureMode {
		case "llm", "hybrid":
			if cfg.RPG.LLMEndpoint == "" || cfg.RPG.LLMModel == "" {
				log.Printf("Warning: RPG feature_mode=%q but llm_endpoint or llm_model is empty, falling back to local extractor", cfg.RPG.FeatureMode)
				featureExtractor = rpg.NewLocalExtractor()
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
	}

	if rpgStore != nil {
		defer rpgStore.Close()
	}

	tracedLanguages := cfg.Trace.EnabledLanguages
	if len(tracedLanguages) == 0 {
		tracedLanguages = []string{".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".php", ".java", ".cs", ".fs", ".fsx", ".fsi"}
	}
	// In multi-worktree mode callers pass isBackgroundChild=true for non-interactive output.
	stats, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, tracedLanguages, cfg.Watch.LastIndexTime, isBackgroundChild)
	if err != nil {
		return err
	}

	if stats.FilesIndexed > 0 || stats.ChunksCreated > 0 {
		cfg.Watch.LastIndexTime = time.Now()
		if err := cfg.Save(projectRoot); err != nil {
			log.Printf("Warning: failed to save config: %v", err)
		}
	}

	if rpgIndexer != nil {
		if err := rpgIndexer.BuildFull(ctx, symbolStore, st); err != nil {
			log.Printf("Warning: failed to build RPG graph for %s: %v", projectRoot, err)
		} else {
			rpgStats := rpgStore.GetGraph().Stats()
			log.Printf("RPG graph built for %s: %d nodes, %d edges", projectRoot, rpgStats.TotalNodes, rpgStats.TotalEdges)
		}
	}

	if err := st.Persist(ctx); err != nil {
		log.Printf("Warning: failed to persist index: %v", err)
	}
	if rpgStore != nil {
		if err := rpgStore.Persist(ctx); err != nil {
			log.Printf("Warning: failed to persist RPG graph: %v", err)
		}
	}

	// Initialize watcher
	w, err := watcher.NewWatcher(projectRoot, ignoreMatcher, cfg.Watch.DebounceMs)
	if err != nil {
		return fmt.Errorf("failed to initialize watcher for %s: %w", projectRoot, err)
	}
	defer w.Close()

	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("failed to start watcher for %s: %w", projectRoot, err)
	}

	if onReady != nil {
		onReady()
	}

	// Run watch loop (responds to ctx.Done() for graceful shutdown)
	return runProjectWatchLoop(ctx, st, symbolStore, w, idx, scanner, extractor, rpgIndexer, rpgStore, tracedLanguages, projectRoot, cfg)
}

func runProjectWatchLoop(ctx context.Context, st store.VectorStore, symbolStore *trace.GOBSymbolStore, w *watcher.Watcher, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, rpgIndexer *rpg.RPGIndexer, rpgStore rpg.RPGStore, tracedLanguages []string, projectRoot string, cfg *config.Config) error {
	persistTicker := time.NewTicker(30 * time.Second)
	defer persistTicker.Stop()

	var lastConfigWrite time.Time
	var rpgManager *rpgRealtimeManager
	if rpgIndexer != nil && rpgStore != nil {
		rpgManager = newRPGRealtimeManager(cfg.Watch.RPGMaxDirtyFilesPerBatch)
		startRPGRealtimeWorkers(ctx, projectRoot, symbolStore, rpgIndexer, rpgStore, cfg.Watch, rpgManager)
	}

	for {
		select {
		case <-ctx.Done():
			if err := st.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist index on shutdown for %s: %v", projectRoot, err)
			}
			if err := symbolStore.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist symbol index on shutdown for %s: %v", projectRoot, err)
			}
			if rpgStore != nil {
				if err := rpgStore.Persist(ctx); err != nil {
					log.Printf("Warning: failed to persist RPG graph on shutdown for %s: %v", projectRoot, err)
				}
			}
			return nil

		case <-persistTicker.C:
			if err := st.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist index for %s: %v", projectRoot, err)
			}
			if err := symbolStore.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist symbol index for %s: %v", projectRoot, err)
			}
			if rpgStore != nil {
				if err := rpgStore.Persist(ctx); err != nil {
					log.Printf("Warning: failed to persist RPG graph for %s: %v", projectRoot, err)
				}
			}

		case event := <-w.Events():
			handleFileEvent(ctx, idx, scanner, extractor, symbolStore, rpgIndexer, st, tracedLanguages, projectRoot, cfg, &lastConfigWrite, rpgManager, event)
		}
	}
}

type watchProjectRunner func(ctx context.Context, projectRoot string, emb embedder.Embedder, isBackgroundChild bool, onReady func()) error

func startProjectWatch(g *errgroup.Group, gCtx context.Context, projectRoot string, emb embedder.Embedder, makeOnReady func() func(), startFn watchProjectRunner) {
	g.Go(func() error {
		onReady := makeOnReady()
		return startFn(gCtx, projectRoot, emb, true, onReady)
	})
}

func waitForProjectsReady(ctx context.Context, total int, readyCh <-chan struct{}) error {
	ready := 0
	for ready < total {
		select {
		case <-readyCh:
			ready++
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func runWatchForeground() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Detect if running as background child process
	isBackgroundChild := os.Getenv("GREPAI_BACKGROUND") == "1"

	// If running in background, determine log directory and write PID file
	var logDir string
	var worktreeID string
	if isBackgroundChild {
		// Configure structured logging with timestamps for daemon mode
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		log.SetPrefix("[grepai-watch] ")

		var err error
		logDir = watchLogDir
		if logDir == "" {
			logDir, err = daemon.GetDefaultLogDir()
			if err != nil {
				return fmt.Errorf("failed to get default log directory: %w", err)
			}
		}

		// Detect worktree
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		gitInfo, gitErr := git.Detect(cwd)
		if gitErr == nil && gitInfo.WorktreeID != "" {
			worktreeID = gitInfo.WorktreeID
		}

		// Write PID file
		if worktreeID != "" {
			if err := daemon.WriteWorktreePIDFile(logDir, worktreeID); err != nil {
				return fmt.Errorf("failed to write PID file: %w", err)
			}
		} else {
			if err := daemon.WritePIDFile(logDir); err != nil {
				return fmt.Errorf("failed to write PID file: %w", err)
			}
		}

		// Ensure PID file is removed on exit
		defer func() {
			if worktreeID != "" {
				if err := daemon.RemoveWorktreePIDFile(logDir, worktreeID); err != nil {
					log.Printf("Warning: failed to remove PID file on exit: %v", err)
				}
			} else {
				if err := daemon.RemovePIDFile(logDir); err != nil {
					log.Printf("Warning: failed to remove PID file on exit: %v", err)
				}
			}
		}()
	}

	// Find project root
	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if !isBackgroundChild {
		fmt.Printf("Starting grepai watch in %s\n", projectRoot)
		fmt.Printf("Provider: %s (%s)\n", cfg.Embedder.Provider, cfg.Embedder.Model)
		fmt.Printf("Backend: %s\n", cfg.Store.Backend)
		if cfg.RPG.Enabled {
			fmt.Printf("RPG: enabled (feature_mode: %s, llm: %s/%s)\n", cfg.RPG.FeatureMode, cfg.RPG.LLMProvider, cfg.RPG.LLMModel)
		} else {
			fmt.Println("RPG: disabled")
		}
	} else {
		log.Printf("Starting grepai watch in %s", projectRoot)
		log.Printf("Provider: %s (%s)", cfg.Embedder.Provider, cfg.Embedder.Model)
		log.Printf("Backend: %s", cfg.Store.Backend)
		if cfg.RPG.Enabled {
			log.Printf("RPG: enabled (feature_mode: %s, llm: %s/%s)", cfg.RPG.FeatureMode, cfg.RPG.LLMProvider, cfg.RPG.LLMModel)
		} else {
			log.Printf("RPG: disabled")
		}
	}

	// Initialize shared embedder (reused across all worktrees)
	emb, err := initializeEmbedder(ctx, cfg)
	if err != nil {
		return err
	}
	defer emb.Close()

	// Discover linked worktrees (only from main worktree)
	linkedWorktrees := discoverWorktreesForWatch(projectRoot)
	if len(linkedWorktrees) > 0 {
		if !isBackgroundChild {
			fmt.Printf("Detected %d linked worktree(s), watching all:\n", len(linkedWorktrees))
			for _, wt := range linkedWorktrees {
				fmt.Printf("  - %s\n", wt)
			}
		} else {
			log.Printf("Detected %d linked worktree(s), watching all", len(linkedWorktrees))
			for _, wt := range linkedWorktrees {
				log.Printf("  - %s", wt)
			}
		}
	}

	// Handle signals at top level
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create cancellable context for all watchers
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()

	stopCh := daemon.StopChannel()
	go func() {
		select {
		case <-sigChan:
			if !isBackgroundChild {
				fmt.Println("\nShutting down...")
			} else {
				log.Println("Shutting down...")
			}
			watchCancel()
		case <-stopCh:
			log.Println("Stop file detected, shutting down...")
			watchCancel()
		case <-watchCtx.Done():
		}
	}()

	// If no linked worktrees, run the main project directly using watchProject
	// (BUG FIX: deduplicated - previously this duplicated watchProject logic inline)
	if len(linkedWorktrees) == 0 {
		onReady := func() {
			if isBackgroundChild {
				if worktreeID != "" {
					if err := daemon.WriteWorktreeReadyFile(logDir, worktreeID); err != nil {
						log.Printf("Warning: failed to write ready file: %v", err)
					}
				} else {
					if err := daemon.WriteReadyFile(logDir); err != nil {
						log.Printf("Warning: failed to write ready file: %v", err)
					}
				}
			} else {
				fmt.Println("\nWatching for changes... (Press Ctrl+C to stop)")
			}
		}

		if isBackgroundChild {
			defer func() {
				if worktreeID != "" {
					if err := daemon.RemoveWorktreeReadyFile(logDir, worktreeID); err != nil {
						log.Printf("Warning: failed to remove ready file on exit: %v", err)
					}
				} else {
					if err := daemon.RemoveReadyFile(logDir); err != nil {
						log.Printf("Warning: failed to remove ready file on exit: %v", err)
					}
				}
			}()
		}

		return watchProject(watchCtx, projectRoot, emb, isBackgroundChild, onReady)
	}

	// Multi-worktree mode: run all projects in parallel using errgroup
	g, gCtx := errgroup.WithContext(watchCtx)

	totalProjects := 1 + len(linkedWorktrees)
	readyCh := make(chan struct{}, totalProjects)

	makeOnReady := func() func() {
		var once sync.Once
		return func() {
			once.Do(func() {
				readyCh <- struct{}{}
			})
		}
	}

	// Main project
	startProjectWatch(g, gCtx, projectRoot, emb, makeOnReady, watchProject)

	// Linked worktrees
	for _, wt := range linkedWorktrees {
		startProjectWatch(g, gCtx, wt, emb, makeOnReady, watchProject)
	}

	// Write ready file after all watchers have actually started (or failed).
	// Uses select to also unblock on context cancellation.
	g.Go(func() error {
		if err := waitForProjectsReady(gCtx, totalProjects, readyCh); err != nil {
			return err
		}
		if isBackgroundChild {
			if worktreeID != "" {
				return daemon.WriteWorktreeReadyFile(logDir, worktreeID)
			}
			return daemon.WriteReadyFile(logDir)
		}
		fmt.Printf("\nWatching %d projects for changes... (Press Ctrl+C to stop)\n", totalProjects)
		return nil
	})

	if isBackgroundChild {
		defer func() {
			if worktreeID != "" {
				if err := daemon.RemoveWorktreeReadyFile(logDir, worktreeID); err != nil {
					log.Printf("Warning: failed to remove ready file on exit: %v", err)
				}
			} else {
				if err := daemon.RemoveReadyFile(logDir); err != nil {
					log.Printf("Warning: failed to remove ready file on exit: %v", err)
				}
			}
		}()
	}

	return g.Wait()
}

func handleFileEvent(ctx context.Context, idx *indexer.Indexer, scanner *indexer.Scanner, extractor *trace.RegexExtractor, symbolStore *trace.GOBSymbolStore, rpgIndexer *rpg.RPGIndexer, vectorStore store.VectorStore, enabledLanguages []string, projectRoot string, cfg *config.Config, lastConfigWrite *time.Time, rpgManager *rpgRealtimeManager, event watcher.FileEvent) {
	log.Printf("[%s] %s", event.Type, event.Path)

	switch event.Type {
	case watcher.EventCreate, watcher.EventModify:
		start := time.Now()
		fileInfo, err := scanner.ScanFile(event.Path)
		if err != nil {
			log.Printf("Failed to scan %s: %v", event.Path, err)
			return
		}
		if fileInfo == nil {
			return // File was skipped (binary, too large, etc.)
		}

		needsReindex, err := idx.NeedsReindex(ctx, fileInfo.Path, fileInfo.Hash)
		if err != nil {
			log.Printf("Failed to check reindex status for %s: %v", event.Path, err)
			return
		}
		if !needsReindex {
			log.Printf("Skipped unchanged %s", event.Path)
			return
		}

		chunks, err := idx.IndexFile(ctx, *fileInfo)
		if err != nil {
			log.Printf("Failed to index %s: %v", event.Path, err)
			return
		}
		log.Printf("Indexed %s (%d chunks)", event.Path, chunks)

		// Update last_index_time with throttling (only write if 30 seconds have passed)
		now := time.Now()
		if now.Sub(*lastConfigWrite) >= configWriteThrottle {
			cfg.Watch.LastIndexTime = now
			if err := cfg.Save(projectRoot); err != nil {
				log.Printf("Warning: failed to save config: %v", err)
			}
			*lastConfigWrite = now
		}

		// Extract symbols if language is supported
		ext := strings.ToLower(filepath.Ext(event.Path))
		if isTracedLanguage(ext, enabledLanguages) {
			symbols, refs, err := extractor.ExtractAll(ctx, fileInfo.Path, fileInfo.Content)
			if err != nil {
				log.Printf("Failed to extract symbols from %s: %v", event.Path, err)
			} else if err := symbolStore.SaveFileWithContentHash(ctx, fileInfo.Path, fileInfo.Hash, symbols, refs); err != nil {
				log.Printf("Failed to save symbols for %s: %v", event.Path, err)
			} else {
				log.Printf("Extracted %d symbols from %s", len(symbols), event.Path)

				// Update RPG graph.
				if rpgIndexer != nil {
					eventType := "create"
					if event.Type == watcher.EventModify {
						eventType = "modify"
					}
					if err := rpgIndexer.HandleFileEvent(ctx, eventType, fileInfo.Path, symbols); err != nil {
						log.Printf("Warning: failed to update RPG for %s: %v", event.Path, err)
					}
					if vectorStore != nil {
						if chunks, err := vectorStore.GetChunksForFile(ctx, fileInfo.Path); err == nil {
							if err := rpgIndexer.LinkChunksForFile(ctx, fileInfo.Path, chunks); err != nil {
								log.Printf("Warning: failed to link RPG chunks for %s: %v", event.Path, err)
							}
						}
					}
					if rpgManager != nil {
						rpgManager.MarkFileDirty(fileInfo.Path)
						dirtyCount, _, _, _ := rpgManager.Snapshot()
						log.Printf("rpg_event_applied_ms=%d file=%s event=%s rpg_dirty_files_count=%d",
							time.Since(start).Milliseconds(),
							fileInfo.Path,
							event.Type.String(),
							dirtyCount,
						)
					}
				}
			}
		}

	case watcher.EventDelete, watcher.EventRename:
		start := time.Now()
		if err := idx.RemoveFile(ctx, event.Path); err != nil {
			log.Printf("Failed to remove %s from index: %v", event.Path, err)
			return
		}
		// Also remove from symbol index
		if err := symbolStore.DeleteFile(ctx, event.Path); err != nil {
			log.Printf("Failed to remove symbols for %s: %v", event.Path, err)
		}
		if rpgIndexer != nil {
			if err := rpgIndexer.HandleFileEvent(ctx, "delete", event.Path, nil); err != nil {
				log.Printf("Warning: failed to update RPG for deleted %s: %v", event.Path, err)
			} else if rpgManager != nil {
				rpgManager.MarkFileDirty(event.Path)
				dirtyCount, _, _, _ := rpgManager.Snapshot()
				log.Printf("rpg_event_applied_ms=%d file=%s event=%s rpg_dirty_files_count=%d",
					time.Since(start).Milliseconds(),
					event.Path,
					event.Type.String(),
					dirtyCount,
				)
			}
		}
		log.Printf("Removed %s from index", event.Path)
	}
}

// isTracedLanguage checks if a file extension is in the enabled languages list.
func isTracedLanguage(ext string, enabledLanguages []string) bool {
	for _, lang := range enabledLanguages {
		if ext == lang {
			return true
		}
	}
	return false
}

// printProgress displays a progress bar for indexing
func printProgress(current, total int, filePath string) {
	if total == 0 {
		return
	}

	// Calculate percentage
	percent := float64(current) / float64(total) * 100

	// Build progress bar (20 chars width)
	barWidth := 20
	filled := int(float64(barWidth) * float64(current) / float64(total))
	bar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", barWidth-filled)

	// Truncate file path if too long
	maxPathLen := 35
	displayPath := filePath
	if len(filePath) > maxPathLen {
		displayPath = "..." + filePath[len(filePath)-maxPathLen+3:]
	}

	// Print with carriage return to overwrite previous line
	fmt.Printf("\rIndexing [%s] %3.0f%% (%d/%d) %s", bar, percent, current, total, displayPath)
}

func printBatchProgress(info indexer.BatchProgressInfo) {
	if info.Retrying {
		fmt.Printf("\r%s\r", strings.Repeat(" ", 80))
		reason := describeRetryReason(info.StatusCode)
		fmt.Printf("%s - Retrying batch %d (attempt %d/5)...\n", reason, info.BatchIndex+1, info.Attempt)
	} else if info.TotalChunks > 0 {
		percentage := float64(info.CompletedChunks) / float64(info.TotalChunks) * 100
		barWidth := 20
		filled := int(float64(barWidth) * float64(info.CompletedChunks) / float64(info.TotalChunks))
		bar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", barWidth-filled)
		fmt.Printf("\rEmbedding [%s] %3.0f%% (%d/%d)", bar, percentage, info.CompletedChunks, info.TotalChunks)
	}
}

// describeRetryReason returns a human-readable description of why a retry is happening
func describeRetryReason(statusCode int) string {
	switch {
	case statusCode == 429:
		return "Rate limited (429)"
	case statusCode >= 500 && statusCode < 600:
		return fmt.Sprintf("Server error (%d)", statusCode)
	case statusCode > 0:
		return fmt.Sprintf("HTTP error (%d)", statusCode)
	default:
		return "Error"
	}
}

// runWorkspaceWatch handles workspace-level watch operations
func runWorkspaceWatch(logDir string) error {
	// Load workspace config
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		return fmt.Errorf("no workspaces configured; create one with: grepai workspace create <name>")
	}

	ws, err := wsCfg.GetWorkspace(watchWorkspace)
	if err != nil {
		return err
	}

	// Validate backend (GOB not supported for workspaces)
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	// Handle --status flag
	if watchStatus {
		return showWorkspaceWatchStatus(logDir, ws)
	}

	// Handle --stop flag
	if watchStop {
		return stopWorkspaceWatchDaemon(logDir, watchWorkspace)
	}

	// Handle --background flag
	if watchBackground {
		return startBackgroundWorkspaceWatch(logDir, ws)
	}

	// Check if already running
	pid, err := daemon.GetRunningWorkspacePID(logDir, watchWorkspace)
	if err != nil {
		return fmt.Errorf("failed to check running status: %w", err)
	}
	if pid > 0 {
		return fmt.Errorf("workspace watcher is already running in background (PID %d)\nUse 'grepai watch --workspace %s --stop' to stop it", pid, watchWorkspace)
	}

	// Run in foreground mode
	return runWorkspaceWatchForeground(logDir, ws)
}

func showWorkspaceWatchStatus(logDir string, ws *config.Workspace) error {
	pid, err := daemon.GetRunningWorkspacePID(logDir, ws.Name)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	if pid == 0 {
		fmt.Printf("Workspace %s: not running\n", ws.Name)
		fmt.Printf("Log directory: %s\n", logDir)
		return nil
	}

	fmt.Printf("Workspace %s: running\n", ws.Name)
	fmt.Printf("PID: %d\n", pid)
	fmt.Printf("Log directory: %s\n", logDir)
	fmt.Printf("Log file: %s\n", daemon.GetWorkspaceLogFile(logDir, ws.Name))
	fmt.Printf("Projects: %d\n", len(ws.Projects))
	for _, p := range ws.Projects {
		fmt.Printf("  - %s: %s\n", p.Name, p.Path)
	}

	return nil
}

func stopWorkspaceWatchDaemon(logDir, workspaceName string) error {
	pid, err := daemon.GetRunningWorkspacePID(logDir, workspaceName)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	if pid == 0 {
		fmt.Printf("No background watcher is running for workspace %s\n", workspaceName)
		return nil
	}

	fmt.Printf("Stopping workspace watcher %s (PID %d)...\n", workspaceName, pid)
	if err := daemon.StopProcess(pid); err != nil {
		return fmt.Errorf("failed to stop process: %w", err)
	}

	// Wait for process to stop
	const shutdownTimeout = 30 * time.Second
	const pollInterval = 500 * time.Millisecond
	deadline := time.Now().Add(shutdownTimeout)

	for time.Now().Before(deadline) {
		if !daemon.IsProcessRunning(pid) {
			break
		}
		time.Sleep(pollInterval)
	}

	if daemon.IsProcessRunning(pid) {
		return fmt.Errorf("process did not stop within %v", shutdownTimeout)
	}

	if err := daemon.RemoveWorkspacePIDFile(logDir, workspaceName); err != nil {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	fmt.Printf("Workspace watcher %s stopped\n", workspaceName)
	return nil
}

func startBackgroundWorkspaceWatch(logDir string, ws *config.Workspace) error {
	// Check if already running
	pid, err := daemon.GetRunningWorkspacePID(logDir, ws.Name)
	if err != nil {
		return fmt.Errorf("failed to check running status: %w", err)
	}
	if pid > 0 {
		return fmt.Errorf("workspace watcher %s is already running (PID %d)", ws.Name, pid)
	}

	// Build extra args
	var extraArgs []string
	if watchLogDir != "" {
		extraArgs = append(extraArgs, "--log-dir", watchLogDir)
	}

	// Spawn background process
	childPID, exitCh, err := daemon.SpawnWorkspaceBackground(logDir, ws.Name, extraArgs)
	if err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	// Wait for process to become ready
	const startupTimeout = 60 * time.Second
	const pollInterval = 250 * time.Millisecond
	deadline := time.Now().Add(startupTimeout)

	wsLogFile := daemon.GetWorkspaceLogFile(logDir, ws.Name)

	for time.Now().Before(deadline) {
		if daemon.IsWorkspaceReady(logDir, ws.Name) {
			fmt.Printf("Workspace watcher %s started (PID %d)\n", ws.Name, childPID)
			fmt.Printf("Logs: %s\n", wsLogFile)
			fmt.Printf("\nUse 'grepai watch --workspace %s --status' to check status\n", ws.Name)
			fmt.Printf("Use 'grepai watch --workspace %s --stop' to stop the watcher\n", ws.Name)
			return nil
		}

		select {
		case <-exitCh:
			return fmt.Errorf("background process failed to start (check logs at %s)", wsLogFile)
		default:
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for process to become ready (check logs at %s)", wsLogFile)
}

func runWorkspaceWatchForeground(logDir string, ws *config.Workspace) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	isBackgroundChild := os.Getenv("GREPAI_BACKGROUND") == "1"

	// If running in background, write PID file
	if isBackgroundChild {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		log.SetPrefix(fmt.Sprintf("[grepai-workspace-%s] ", ws.Name))

		if err := daemon.WriteWorkspacePIDFile(logDir, ws.Name); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		defer func() {
			if err := daemon.RemoveWorkspacePIDFile(logDir, ws.Name); err != nil {
				log.Printf("Warning: failed to remove PID file on exit: %v", err)
			}
		}()
	}

	if !isBackgroundChild {
		fmt.Printf("Starting workspace watcher: %s\n", ws.Name)
		fmt.Printf("Backend: %s\n", ws.Store.Backend)
		fmt.Printf("Embedder: %s (%s)\n", ws.Embedder.Provider, ws.Embedder.Model)
		fmt.Printf("Projects: %d\n", len(ws.Projects))
	} else {
		log.Printf("Starting workspace watcher: %s", ws.Name)
		log.Printf("Backend: %s", ws.Store.Backend)
		log.Printf("Embedder: %s (%s)", ws.Embedder.Provider, ws.Embedder.Model)
		log.Printf("Projects: %d", len(ws.Projects))
	}

	// Check all project paths exist
	for _, p := range ws.Projects {
		if _, err := os.Stat(p.Path); os.IsNotExist(err) {
			return fmt.Errorf("project path does not exist: %s (%s)", p.Name, p.Path)
		}
	}

	// Initialize shared embedder
	embCfg := &config.Config{Embedder: ws.Embedder}
	emb, err := initializeEmbedder(ctx, embCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize embedder: %w", err)
	}
	defer emb.Close()

	// Initialize shared store with workspace-specific project ID
	st, err := initializeWorkspaceStore(ctx, ws)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	defer st.Close()

	runtimes := make(map[string]*workspaceProjectRuntime, len(ws.Projects))
	watchers := make([]*watcher.Watcher, 0, len(ws.Projects))

	for _, project := range ws.Projects {
		if !isBackgroundChild {
			fmt.Printf("\nIndexing project: %s (%s)\n", project.Name, project.Path)
		} else {
			log.Printf("Indexing project: %s (%s)", project.Name, project.Path)
		}

		runtime, w, rtErr := initializeWorkspaceRuntime(ctx, ws, project, emb, st, isBackgroundChild)
		if rtErr != nil {
			log.Printf("Warning: failed to initialize runtime for %s: %v", project.Name, rtErr)
			continue
		}

		projectKey := canonicalPath(project.Path)
		runtimes[projectKey] = runtime
		watchers = append(watchers, w)
	}

	defer func() {
		for _, w := range watchers {
			w.Close()
		}
		for _, runtime := range runtimes {
			if runtime.symbolStore != nil {
				if err := runtime.symbolStore.Close(); err != nil {
					log.Printf("Warning: failed to close symbol store for %s: %v", runtime.project.Path, err)
				}
			}
			if runtime.rpgStore != nil {
				if err := runtime.rpgStore.Close(); err != nil {
					log.Printf("Warning: failed to close RPG store for %s: %v", runtime.project.Path, err)
				}
			}
		}
	}()

	// Write ready file
	if isBackgroundChild {
		if err := daemon.WriteWorkspaceReadyFile(logDir, ws.Name); err != nil {
			return fmt.Errorf("failed to write ready file: %w", err)
		}
		defer func() {
			if err := daemon.RemoveWorkspaceReadyFile(logDir, ws.Name); err != nil {
				log.Printf("Warning: failed to remove ready file on exit: %v", err)
			}
		}()
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	wsStopCh := daemon.StopChannel()

	if !isBackgroundChild {
		fmt.Printf("\nWatching %d projects for changes... (Press Ctrl+C to stop)\n", len(runtimes))
	} else {
		log.Printf("Watching %d projects for changes...", len(runtimes))
	}

	// Collect events from all watchers
	eventChan := make(chan workspaceWatchEvent, 100)
	for _, runtime := range runtimes {
		runtime := runtime
		if runtime.watcher == nil {
			continue
		}
		go func() {
			for event := range runtime.watcher.Events() {
				eventChan <- workspaceWatchEvent{
					projectPath: runtime.project.Path,
					event:       event,
				}
			}
		}()
	}

	// persistAndShutdown persists all stores before returning from the event loop.
	persistAndShutdown := func() {
		if err := st.Persist(ctx); err != nil {
			log.Printf("Warning: failed to persist index on shutdown: %v", err)
		}
		for _, runtime := range runtimes {
			if err := runtime.symbolStore.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist symbol index on shutdown for %s: %v", runtime.project.Name, err)
			}
			if runtime.rpgStore != nil {
				if err := runtime.rpgStore.Persist(ctx); err != nil {
					log.Printf("Warning: failed to persist RPG graph on shutdown for %s: %v", runtime.project.Name, err)
				}
			}
		}
	}

	// Event loop
	persistTicker := time.NewTicker(30 * time.Second)
	defer persistTicker.Stop()

	for {
		select {
		case <-sigChan:
			if !isBackgroundChild {
				fmt.Println("\nShutting down...")
			} else {
				log.Println("Shutting down...")
			}
			persistAndShutdown()
			return nil

		case <-wsStopCh:
			log.Println("Stop file detected, shutting down...")
			persistAndShutdown()
			return nil

		case <-persistTicker.C:
			if err := st.Persist(ctx); err != nil {
				log.Printf("Warning: failed to persist index: %v", err)
			}
			for _, runtime := range runtimes {
				if err := runtime.symbolStore.Persist(ctx); err != nil {
					log.Printf("Warning: failed to persist symbol index for %s: %v", runtime.project.Name, err)
				}
				if runtime.rpgStore != nil {
					if err := runtime.rpgStore.Persist(ctx); err != nil {
						log.Printf("Warning: failed to persist RPG graph for %s: %v", runtime.project.Name, err)
					}
				}
			}

		case event := <-eventChan:
			projectKey := canonicalPath(event.projectPath)
			runtime := runtimes[projectKey]
			if runtime == nil {
				log.Printf("Warning: received event for unknown runtime: %s", event.projectPath)
				continue
			}
			handleFileEvent(
				ctx,
				runtime.idx,
				runtime.scanner,
				runtime.extractor,
				runtime.symbolStore,
				runtime.rpgIndexer,
				runtime.vectorStore,
				runtime.tracedLanguages,
				runtime.project.Path,
				runtime.cfg,
				&runtime.lastConfigWrite,
				runtime.manager,
				event.event,
			)
		}
	}
}

type workspaceWatchEvent struct {
	projectPath string
	event       watcher.FileEvent
}

type workspaceProjectRuntime struct {
	project         config.ProjectEntry
	cfg             *config.Config
	idx             *indexer.Indexer
	scanner         *indexer.Scanner
	extractor       *trace.RegexExtractor
	symbolStore     *trace.GOBSymbolStore
	rpgIndexer      *rpg.RPGIndexer
	rpgStore        rpg.RPGStore
	vectorStore     store.VectorStore
	tracedLanguages []string
	lastConfigWrite time.Time
	manager         *rpgRealtimeManager
	watcher         *watcher.Watcher
}

func initializeWorkspaceRuntime(ctx context.Context, ws *config.Workspace, project config.ProjectEntry, emb embedder.Embedder, sharedStore store.VectorStore, isBackgroundChild bool) (*workspaceProjectRuntime, *watcher.Watcher, error) {
	projectCfg := config.DefaultConfig()
	if config.Exists(project.Path) {
		loadedCfg, err := config.Load(project.Path)
		if err != nil {
			log.Printf("Warning: failed to load config for %s, using defaults: %v", project.Name, err)
		} else {
			projectCfg = loadedCfg
		}
	}

	ignoreMatcher, err := indexer.NewIgnoreMatcher(project.Path, projectCfg.Ignore, projectCfg.ExternalGitignore)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize ignore matcher: %w", err)
	}

	scanner := indexer.NewScanner(project.Path, ignoreMatcher)
	chunker := indexer.NewChunker(projectCfg.Chunking.Size, projectCfg.Chunking.Overlap)
	vectorStore := &projectPrefixStore{
		store:         sharedStore,
		workspaceName: ws.Name,
		projectName:   project.Name,
		projectPath:   project.Path,
	}
	idx := indexer.NewIndexer(project.Path, vectorStore, emb, chunker, scanner, projectCfg.Watch.LastIndexTime)
	extractor := trace.NewRegexExtractor()
	symbolStore := trace.NewGOBSymbolStore(config.GetSymbolIndexPath(project.Path))
	if err := symbolStore.Load(ctx); err != nil {
		log.Printf("Warning: failed to load symbol index for %s: %v", project.Path, err)
	}

	tracedLanguages := projectCfg.Trace.EnabledLanguages
	if len(tracedLanguages) == 0 {
		tracedLanguages = []string{".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".php", ".java", ".cs", ".fs", ".fsx", ".fsi"}
	}

	stats, err := runInitialScan(ctx, idx, scanner, extractor, symbolStore, tracedLanguages, projectCfg.Watch.LastIndexTime, isBackgroundChild)
	if err != nil {
		_ = symbolStore.Close()
		return nil, nil, err
	}
	if stats.FilesIndexed > 0 || stats.ChunksCreated > 0 {
		projectCfg.Watch.LastIndexTime = time.Now()
		if err := projectCfg.Save(project.Path); err != nil {
			log.Printf("Warning: failed to save config for %s: %v", project.Name, err)
		}
	}

	var rpgStore rpg.RPGStore
	var rpgIndexer *rpg.RPGIndexer
	var manager *rpgRealtimeManager
	if projectCfg.RPG.Enabled {
		rpgStore = rpg.NewGOBRPGStore(config.GetRPGIndexPath(project.Path))
		if err := rpgStore.Load(ctx); err != nil {
			log.Printf("Warning: failed to load RPG index for %s: %v", project.Path, err)
		}

		var featureExtractor rpg.FeatureExtractor
		switch projectCfg.RPG.FeatureMode {
		case "llm", "hybrid":
			if projectCfg.RPG.LLMEndpoint == "" || projectCfg.RPG.LLMModel == "" {
				log.Printf("Warning: RPG feature_mode=%q but llm_endpoint or llm_model is empty for %s, falling back to local extractor", projectCfg.RPG.FeatureMode, project.Path)
				featureExtractor = rpg.NewLocalExtractor()
			} else {
				featureExtractor = rpg.NewLLMExtractor(rpg.LLMExtractorConfig{
					Provider: projectCfg.RPG.LLMProvider,
					Model:    projectCfg.RPG.LLMModel,
					Endpoint: projectCfg.RPG.LLMEndpoint,
					APIKey:   projectCfg.RPG.LLMAPIKey,
					Timeout:  time.Duration(projectCfg.RPG.LLMTimeoutMs) * time.Millisecond,
				})
			}
		default:
			featureExtractor = rpg.NewLocalExtractor()
		}

		rpgIndexer = rpg.NewRPGIndexer(rpgStore, featureExtractor, project.Path, rpg.RPGIndexerConfig{
			DriftThreshold:       projectCfg.RPG.DriftThreshold,
			MaxTraversalDepth:    projectCfg.RPG.MaxTraversalDepth,
			FeatureGroupStrategy: projectCfg.RPG.FeatureGroupStrategy,
		})
		if err := rpgIndexer.BuildFull(ctx, symbolStore, vectorStore); err != nil {
			log.Printf("Warning: failed to build RPG graph for %s: %v", project.Path, err)
		}
		if err := rpgStore.Persist(ctx); err != nil {
			log.Printf("Warning: failed to persist RPG graph for %s: %v", project.Path, err)
		}

		manager = newRPGRealtimeManager(projectCfg.Watch.RPGMaxDirtyFilesPerBatch)
		startRPGRealtimeWorkers(ctx, fmt.Sprintf("workspace:%s/%s", ws.Name, project.Name), symbolStore, rpgIndexer, rpgStore, projectCfg.Watch, manager)
	}

	w, err := watcher.NewWatcher(project.Path, ignoreMatcher, projectCfg.Watch.DebounceMs)
	if err != nil {
		if rpgStore != nil {
			_ = rpgStore.Close()
		}
		_ = symbolStore.Close()
		return nil, nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	if err := w.Start(ctx); err != nil {
		w.Close()
		if rpgStore != nil {
			_ = rpgStore.Close()
		}
		_ = symbolStore.Close()
		return nil, nil, fmt.Errorf("failed to start watcher: %w", err)
	}

	runtime := &workspaceProjectRuntime{
		project:         project,
		cfg:             projectCfg,
		idx:             idx,
		scanner:         scanner,
		extractor:       extractor,
		symbolStore:     symbolStore,
		rpgIndexer:      rpgIndexer,
		rpgStore:        rpgStore,
		vectorStore:     vectorStore,
		tracedLanguages: tracedLanguages,
		manager:         manager,
		watcher:         w,
	}
	return runtime, w, nil
}

func initializeWorkspaceStore(ctx context.Context, ws *config.Workspace) (store.VectorStore, error) {
	// Use workspace name as project ID for shared store
	projectID := "workspace:" + ws.Name

	switch ws.Store.Backend {
	case "postgres":
		return store.NewPostgresStore(ctx, ws.Store.Postgres.DSN, projectID, ws.Embedder.GetDimensions())
	case "qdrant":
		collectionName := ws.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = "workspace_" + ws.Name
		}
		return store.NewQdrantStore(ctx, ws.Store.Qdrant.Endpoint, ws.Store.Qdrant.Port, ws.Store.Qdrant.UseTLS, collectionName, ws.Store.Qdrant.APIKey, ws.Embedder.GetDimensions())
	default:
		return nil, fmt.Errorf("unsupported backend for workspace: %s", ws.Store.Backend)
	}
}

// projectPrefixStore wraps a VectorStore to prefix file paths with workspace and project name
type projectPrefixStore struct {
	store         store.VectorStore
	workspaceName string
	projectName   string
	projectPath   string
}

// getPrefix returns the full prefix for this project (workspace/project)
func (p *projectPrefixStore) getPrefix() string {
	return p.workspaceName + "/" + p.projectName
}

// toRelSlash computes a forward-slash relative path from the project root.
// Vector store paths must always use forward slashes regardless of OS.
func (p *projectPrefixStore) toRelSlash(absPath string) string {
	if !filepath.IsAbs(absPath) {
		return filepath.ToSlash(absPath)
	}
	rel, err := filepath.Rel(p.projectPath, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

func (p *projectPrefixStore) SaveChunks(ctx context.Context, chunks []store.Chunk) error {
	prefixedChunks := make([]store.Chunk, len(chunks))
	for i, c := range chunks {
		prefixedChunks[i] = c
		relPath := p.toRelSlash(c.FilePath)
		prefixedPath := p.getPrefix() + "/" + relPath
		prefixedChunks[i].FilePath = prefixedPath
		// Also update the chunk ID to include project prefix
		// Original ID format is "filePath_index", we need to replace the filePath part
		if idx := strings.LastIndex(c.ID, "_"); idx >= 0 {
			prefixedChunks[i].ID = prefixedPath + c.ID[idx:]
		}
	}
	return p.store.SaveChunks(ctx, prefixedChunks)
}

func (p *projectPrefixStore) DeleteByFile(ctx context.Context, filePath string) error {
	prefixedPath := p.getPrefix() + "/" + p.toRelSlash(filePath)
	return p.store.DeleteByFile(ctx, prefixedPath)
}

func (p *projectPrefixStore) Search(ctx context.Context, queryVector []float32, limit int, opts store.SearchOptions) ([]store.SearchResult, error) {
	return p.store.Search(ctx, queryVector, limit, opts)
}

func (p *projectPrefixStore) GetDocument(ctx context.Context, filePath string) (*store.Document, error) {
	prefixedPath := p.getPrefix() + "/" + p.toRelSlash(filePath)
	return p.store.GetDocument(ctx, prefixedPath)
}

func (p *projectPrefixStore) SaveDocument(ctx context.Context, doc store.Document) error {
	doc.Path = p.getPrefix() + "/" + p.toRelSlash(doc.Path)
	return p.store.SaveDocument(ctx, doc)
}

func (p *projectPrefixStore) DeleteDocument(ctx context.Context, filePath string) error {
	prefixedPath := p.getPrefix() + "/" + p.toRelSlash(filePath)
	return p.store.DeleteDocument(ctx, prefixedPath)
}

func (p *projectPrefixStore) ListDocuments(ctx context.Context) ([]string, error) {
	return p.store.ListDocuments(ctx)
}

func (p *projectPrefixStore) Load(ctx context.Context) error {
	return p.store.Load(ctx)
}

func (p *projectPrefixStore) Persist(ctx context.Context) error {
	return p.store.Persist(ctx)
}

func (p *projectPrefixStore) Close() error {
	return p.store.Close()
}

func (p *projectPrefixStore) GetStats(ctx context.Context) (*store.IndexStats, error) {
	return p.store.GetStats(ctx)
}

func (p *projectPrefixStore) ListFilesWithStats(ctx context.Context) ([]store.FileStats, error) {
	return p.store.ListFilesWithStats(ctx)
}

func (p *projectPrefixStore) GetChunksForFile(ctx context.Context, filePath string) ([]store.Chunk, error) {
	relPath, err := filepath.Rel(p.projectPath, filePath)
	if err == nil {
		filePath = p.getPrefix() + "/" + filepath.ToSlash(relPath)
	}
	return p.store.GetChunksForFile(ctx, filePath)
}

func (p *projectPrefixStore) GetAllChunks(ctx context.Context) ([]store.Chunk, error) {
	return p.store.GetAllChunks(ctx)
}

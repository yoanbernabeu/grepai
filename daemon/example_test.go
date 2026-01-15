package daemon_test

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/yoanbernabeu/grepai/daemon"
)

// ExampleGetDefaultLogDir demonstrates how to get the OS-specific default log directory.
func ExampleGetDefaultLogDir() {
	logDir, err := daemon.GetDefaultLogDir()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Log directory determined")
	// On macOS: ~/Library/Logs/grepai
	// On Linux: ~/.local/state/grepai/logs (or $XDG_STATE_HOME/grepai/logs)
	// On Windows: %LOCALAPPDATA%\grepai\logs
	_ = logDir
}

// ExampleGetRunningPID demonstrates how to check if a watcher is running
// and automatically clean up stale PID files.
func ExampleGetRunningPID() {
	logDir := "/tmp/grepai-logs"

	// Check if a watcher is running
	pid, err := daemon.GetRunningPID(logDir)
	if err != nil {
		log.Fatal(err)
	}

	if pid == 0 {
		fmt.Println("No watcher running")
	} else {
		fmt.Printf("Watcher running with PID %d\n", pid)
	}
}

// ExampleReadPIDFile demonstrates how to read the PID file directly.
// For most use cases, prefer GetRunningPID which also handles stale PIDs.
func ExampleReadPIDFile() {
	logDir := "/tmp/grepai-logs"

	// Read the PID file
	pid, err := daemon.ReadPIDFile(logDir)
	if err != nil {
		log.Fatal(err)
	}

	if pid == 0 {
		fmt.Println("No PID file found")
	} else {
		// Check if the process is actually running
		if daemon.IsProcessRunning(pid) {
			fmt.Printf("Process %d is running\n", pid)
		} else {
			fmt.Printf("Process %d is not running (stale PID)\n", pid)
		}
	}
}

// ExampleSpawnBackground demonstrates how to start a background process.
// This is a simplified example; real usage should include error handling
// and startup verification.
func ExampleSpawnBackground() {
	logDir := "/tmp/grepai-logs"

	// Spawn background process
	pid, err := daemon.SpawnBackground(logDir, []string{"watch"})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Started background watcher with PID %d\n", pid)
	fmt.Printf("Logs: %s\n", filepath.Join(logDir, "grepai-watch.log"))

	// In real usage, poll for IsReady() to verify successful startup
}

// ExampleWritePIDFile demonstrates how to write the current process PID to a file.
// This is typically called after daemonizing to record the background process PID.
func ExampleWritePIDFile() {
	logDir := "/tmp/grepai-logs"

	// Write PID file for current process
	if err := daemon.WritePIDFile(logDir); err != nil {
		log.Fatal(err)
	}

	fmt.Println("PID file written")

	// Clean up when done
	defer daemon.RemovePIDFile(logDir)
}

// ExampleStopProcess demonstrates how to gracefully stop a background process.
func ExampleStopProcess() {
	logDir := "/tmp/grepai-logs"

	// Get the running PID
	pid, err := daemon.GetRunningPID(logDir)
	if err != nil {
		log.Fatal(err)
	}

	if pid == 0 {
		fmt.Println("No process to stop")
		return
	}

	// Send interrupt signal
	if err := daemon.StopProcess(pid); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Sent interrupt signal to PID %d\n", pid)

	// In real usage, poll IsProcessRunning to wait for shutdown
	// Then call RemovePIDFile to clean up
}

package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/daemon"
)

const watchLogDirHintFileName = "watch-log-dir"

func watchLogDirHintPath(projectRoot string) string {
	return filepath.Join(config.GetConfigDir(projectRoot), watchLogDirHintFileName)
}

func readWatchLogDirHint(projectRoot string) (string, error) {
	data, err := os.ReadFile(watchLogDirHintPath(projectRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func clearWatchLogDirHint(projectRoot string) error {
	err := os.Remove(watchLogDirHintPath(projectRoot))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveWatchLogDirHint(projectRoot, logDir string) error {
	if projectRoot == "" {
		return nil
	}
	rawLogDir := strings.TrimSpace(logDir)
	if rawLogDir == "" {
		return clearWatchLogDirHint(projectRoot)
	}
	cleanLogDir := filepath.Clean(rawLogDir)
	if !filepath.IsAbs(cleanLogDir) {
		if absLogDir, err := filepath.Abs(cleanLogDir); err == nil {
			cleanLogDir = absLogDir
		}
	}

	defaultLogDir, err := daemon.GetDefaultLogDir()
	if err == nil && filepath.Clean(defaultLogDir) == cleanLogDir {
		return clearWatchLogDirHint(projectRoot)
	}

	cfgDir := config.GetConfigDir(projectRoot)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(watchLogDirHintPath(projectRoot), []byte(cleanLogDir+"\n"), 0600)
}

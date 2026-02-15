package cli

import (
	"os"
	"strings"
)

func isTerminalFD(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func isInteractiveTerminal() bool {
	if !isTerminalFD(os.Stdin) || !isTerminalFD(os.Stdout) {
		return false
	}
	term := strings.TrimSpace(strings.ToLower(os.Getenv("TERM")))
	return term != "" && term != "dumb"
}

func shouldUseWatchUI(isTTY, noUI, background, status, stop bool, workspace string) bool {
	if !isTTY || noUI {
		return false
	}
	if background || status || stop {
		return false
	}
	return workspace == ""
}

func shouldUseStatusUI(isTTY, noUI bool) bool {
	return isTTY && !noUI
}

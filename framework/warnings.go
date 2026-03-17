package framework

import (
	"log"
	"sync"
)

var loggedFrameworkWarnings sync.Map

// LogWarningsOnce suppresses identical framework warnings after the first log.
func LogWarningsOnce(warnings []string) {
	for _, warning := range warnings {
		if warning == "" {
			continue
		}
		if _, loaded := loggedFrameworkWarnings.LoadOrStore(warning, struct{}{}); loaded {
			continue
		}
		log.Printf("Warning: %s", warning)
	}
}

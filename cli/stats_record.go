package cli

import (
	"context"
	"time"

	"github.com/yoanbernabeu/grepai/stats"
)

// statsWriteTimeout bounds how long a CLI command waits for its stats entry
// to be written before exiting.
const statsWriteTimeout = 500 * time.Millisecond

// recordStatsEntry writes a stats entry and waits for the write to finish,
// up to statsWriteTimeout. A CLI process exits right after printing its
// output, so a fire-and-forget goroutine would be killed before writing.
// The wait is bounded so a held stats lock can never stall the command.
func recordStatsEntry(projectRoot string, entry stats.Entry) {
	ctx, cancel := context.WithTimeout(context.Background(), statsWriteTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = stats.NewRecorder(projectRoot).Record(ctx, entry)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

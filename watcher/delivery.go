package watcher

import (
	"context"
	"time"
)

func (w *Watcher) processDelivery(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case <-w.flushReady:
			w.flushWithContext(ctx)
		}
	}
}

func (w *Watcher) debounceEvent(event FileEvent) {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()
	if w.stopped() {
		return
	}
	if event.Type == EventReconcile {
		existing := w.reconcilePending[event.Path]
		event.IsDir = event.IsDir || existing.IsDir
		w.reconcilePending[event.Path] = event
		w.resetDebounceTimerLocked()
		return
	}

	// Directory identity is sticky because a replacement file can arrive before
	// the old directory cleanup event is delivered.
	existing, exists := w.pending[event.Path]
	event.IsDir = event.IsDir || existing.IsDir
	if exists && existing.Type == EventDelete && event.Type != EventDelete {
		// Keep delete if the path was deleted then recreated quickly.
		existing.IsDir = event.IsDir
		w.pending[event.Path] = existing
	} else {
		w.pending[event.Path] = event
	}

	w.resetDebounceTimerLocked()
}

func (w *Watcher) resetDebounceTimerLocked() {
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(time.Duration(w.debounceMs)*time.Millisecond, func() {
		select {
		case w.flushReady <- struct{}{}:
		case <-w.done:
		default:
		}
	})
}

func (w *Watcher) flush() {
	w.flushWithContext(context.Background())
}

func (w *Watcher) flushWithContext(ctx context.Context) {
	w.eventSenders.Add(1)
	defer w.eventSenders.Done()
	w.pendingMu.Lock()
	events := make([]FileEvent, 0, len(w.pending))
	for _, event := range w.pending {
		events = append(events, event)
	}
	for _, event := range w.reconcilePending {
		events = append(events, event)
	}
	w.pending = make(map[string]FileEvent)
	w.reconcilePending = make(map[string]FileEvent)
	w.pendingMu.Unlock()

	for _, event := range events {
		select {
		case w.events <- event:
		case <-ctx.Done():
			return
		case <-w.done:
			return
		}
	}
}

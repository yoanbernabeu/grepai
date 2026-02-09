package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/yoanbernabeu/grepai/embedder"
	"golang.org/x/sync/errgroup"
)

func TestWaitForProjectsReady_CanceledWhenAnyWatcherFailsBeforeReady(t *testing.T) {
	g, gCtx := errgroup.WithContext(context.Background())

	readyCh := make(chan struct{}, 2)

	makeOnReady := func() func() {
		signaled := false
		return func() {
			if signaled {
				return
			}
			signaled = true
			readyCh <- struct{}{}
		}
	}

	start := func(ctx context.Context, projectRoot string, _ embedder.Embedder, _ bool, onReady func()) error {
		if projectRoot == "ok" {
			onReady()
			<-ctx.Done()
			return ctx.Err()
		}
		return errors.New("boom")
	}

	startProjectWatch(g, gCtx, "ok", nil, makeOnReady, start)
	startProjectWatch(g, gCtx, "fail", nil, makeOnReady, start)

	if err := waitForProjectsReady(gCtx, 2, readyCh); err == nil {
		t.Fatal("waitForProjectsReady() expected cancellation when one watcher fails before ready")
	}

	if err := g.Wait(); err == nil {
		t.Fatal("g.Wait() expected failure from watcher")
	}
}

func TestWaitForProjectsReady_SucceedsWhenAllWatchersSignalReady(t *testing.T) {
	g, gCtx := errgroup.WithContext(context.Background())

	readyCh := make(chan struct{}, 2)

	makeOnReady := func() func() {
		signaled := false
		return func() {
			if signaled {
				return
			}
			signaled = true
			readyCh <- struct{}{}
		}
	}

	start := func(ctx context.Context, projectRoot string, _ embedder.Embedder, _ bool, onReady func()) error {
		_ = ctx
		_ = projectRoot
		onReady()
		return nil
	}

	startProjectWatch(g, gCtx, "a", nil, makeOnReady, start)
	startProjectWatch(g, gCtx, "b", nil, makeOnReady, start)

	if err := waitForProjectsReady(gCtx, 2, readyCh); err != nil {
		t.Fatalf("waitForProjectsReady() unexpected error: %v", err)
	}

	if err := g.Wait(); err != nil {
		t.Fatalf("g.Wait() unexpected error: %v", err)
	}
}

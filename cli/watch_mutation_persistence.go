package cli

import (
	"context"
	"log"

	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

func persistProjectPeriodically(ctx context.Context, fence *watchMutationFence, st store.VectorStore, symbolStore trace.SymbolStore, rpgStore rpg.RPGStore, projectRoot string) error {
	return fence.handle(ctx, func(persistCtx context.Context) {
		if err := st.Persist(persistCtx); err != nil {
			log.Printf("Warning: failed to persist index for %s: %v", projectRoot, err)
		}
		if err := symbolStore.Persist(persistCtx); err != nil {
			log.Printf("Warning: failed to persist symbol index for %s: %v", projectRoot, err)
		}
		if rpgStore != nil {
			if err := rpgStore.Persist(persistCtx); err != nil {
				log.Printf("Warning: failed to persist RPG graph for %s: %v", projectRoot, err)
			}
		}
	})
}

func persistWorkspacePeriodically(ctx context.Context, fence *watchMutationFence, st store.VectorStore, runtimes map[string]*workspaceProjectRuntime) error {
	return fence.handle(ctx, func(persistCtx context.Context) {
		if err := st.Persist(persistCtx); err != nil {
			log.Printf("Warning: failed to persist index: %v", err)
		}
		for _, runtime := range runtimes {
			if err := runtime.symbolStore.Persist(persistCtx); err != nil {
				log.Printf("Warning: failed to persist symbol index for %s: %v", runtime.project.Name, err)
			}
			if runtime.rpgStore != nil {
				if err := runtime.rpgStore.Persist(persistCtx); err != nil {
					log.Printf("Warning: failed to persist RPG graph for %s: %v", runtime.project.Name, err)
				}
			}
		}
	})
}

func persistWorkspaceOnShutdown(ctx context.Context, st store.VectorStore, runtimes map[string]*workspaceProjectRuntime) {
	if err := persistWorkspaceStores(ctx, st, runtimes); err != nil {
		log.Printf("Warning: failed to persist workspace indexes on shutdown: %v", err)
	}
	// Flush per-project watch.last_index_time surgically (stamp only when a
	// config file already exists) so restarts can use the mtime gate.
	for _, runtime := range runtimes {
		if runtime == nil || runtime.cfg == nil {
			continue
		}
		persistLastIndexTime(runtime.project.Path, runtime.cfg, runtime.cfg.Watch.LastIndexTime)
	}
}

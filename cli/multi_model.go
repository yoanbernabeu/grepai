package cli

import (
	"context"

	"github.com/yoanbernabeu/grepai/store"
)

// countUntaggedChunks returns the number of chunks with an empty EmbedModel field.
func countUntaggedChunks(ctx context.Context, st store.VectorStore) (int, error) {
	allChunks, err := st.GetAllChunks(ctx)
	if err != nil {
		return 0, err
	}

	var count int
	for _, c := range allChunks {
		if c.EmbedModel == "" {
			count++
		}
	}
	return count, nil
}

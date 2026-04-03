package search

import (
	"context"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

type Searcher struct {
	store     store.VectorStore
	embedder  embedder.Embedder
	boostCfg  config.BoostConfig
	hybridCfg config.HybridConfig
	dedupCfg  config.DedupConfig
}

func NewSearcher(st store.VectorStore, emb embedder.Embedder, searchCfg config.SearchConfig) *Searcher {
	return &Searcher{
		store:     st,
		embedder:  emb,
		boostCfg:  searchCfg.Boost,
		hybridCfg: searchCfg.Hybrid,
		dedupCfg:  searchCfg.Dedup,
	}
}

func (s *Searcher) Search(ctx context.Context, query string, limit int, pathPrefix string) ([]store.SearchResult, error) {
	queryVector, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	fetchMultiplier := 2
	if s.dedupCfg.Enabled {
		fetchMultiplier = 4
	}
	fetchLimit := limit * fetchMultiplier

	var results []store.SearchResult

	if s.hybridCfg.Enabled {
		results, err = s.hybridSearch(ctx, query, queryVector, fetchLimit, pathPrefix)
	} else {
		results, err = s.store.Search(ctx, queryVector, fetchLimit, store.SearchOptions{PathPrefix: pathPrefix})
	}

	if err != nil {
		return nil, err
	}

	results = ApplyBoost(results, s.boostCfg)

	if s.dedupCfg.Enabled {
		results = DeduplicateByFile(results)
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// hybridSearch combines vector search and text search using RRF.
func (s *Searcher) hybridSearch(ctx context.Context, query string, queryVector []float32, limit int, pathPrefix string) ([]store.SearchResult, error) {
	vectorResults, err := s.store.Search(ctx, queryVector, limit, store.SearchOptions{PathPrefix: pathPrefix})
	if err != nil {
		return nil, err
	}

	allChunks, err := s.store.GetAllChunks(ctx)
	if err != nil {
		return nil, err
	}

	textResults := TextSearch(ctx, allChunks, query, limit, pathPrefix)

	k := s.hybridCfg.K
	if k <= 0 {
		k = 60
	}

	return ReciprocalRankFusion(k, limit, vectorResults, textResults), nil
}

package search

import (
	"context"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

type Searcher struct {
	store          store.VectorStore
	embedder       embedder.Embedder
	boostCfg       config.BoostConfig
	hybridCfg      config.HybridConfig
	embedModelTag  string // When non-empty, filter search results by this model tag
}

func NewSearcher(st store.VectorStore, emb embedder.Embedder, searchCfg config.SearchConfig) *Searcher {
	return &Searcher{
		store:     st,
		embedder:  emb,
		boostCfg:  searchCfg.Boost,
		hybridCfg: searchCfg.Hybrid,
	}
}

// SetEmbedModelFilter sets the model tag used to filter search results.
// When non-empty, only chunks with this exact EmbedModel are returned.
func (s *Searcher) SetEmbedModelFilter(tag string) {
	s.embedModelTag = tag
}

func (s *Searcher) Search(ctx context.Context, query string, limit int, pathPrefix string) ([]store.SearchResult, error) {
	// Embed the query
	queryVector, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	// Fetch more results to allow re-ranking
	fetchLimit := limit * 2

	var results []store.SearchResult

	opts := store.SearchOptions{
		PathPrefix: pathPrefix,
		EmbedModel: s.embedModelTag,
	}

	if s.hybridCfg.Enabled {
		// Hybrid search: combine vector + text search with RRF
		results, err = s.hybridSearch(ctx, query, queryVector, fetchLimit, opts)
	} else {
		// Vector-only search
		results, err = s.store.Search(ctx, queryVector, fetchLimit, opts)
	}

	if err != nil {
		return nil, err
	}

	// Apply structural boosting
	results = ApplyBoost(results, s.boostCfg)

	// Trim to requested limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// hybridSearch combines vector search and text search using RRF.
func (s *Searcher) hybridSearch(ctx context.Context, query string, queryVector []float32, limit int, opts store.SearchOptions) ([]store.SearchResult, error) {
	// Vector search
	vectorResults, err := s.store.Search(ctx, queryVector, limit, opts)
	if err != nil {
		return nil, err
	}

	// Text search (get all chunks first, then filter by model tag)
	allChunks, err := s.store.GetAllChunks(ctx)
	if err != nil {
		return nil, err
	}

	// Filter allChunks by embed model tag when set
	if s.embedModelTag != "" {
		filtered := make([]store.Chunk, 0, len(allChunks))
		for _, c := range allChunks {
			if c.EmbedModel == s.embedModelTag {
				filtered = append(filtered, c)
			}
		}
		allChunks = filtered
	}

	textResults := TextSearch(ctx, allChunks, query, limit, opts.PathPrefix)

	// Combine with RRF
	k := s.hybridCfg.K
	if k <= 0 {
		k = 60 // default
	}

	return ReciprocalRankFusion(k, limit, vectorResults, textResults), nil
}

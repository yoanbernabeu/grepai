package search

import (
	"testing"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/store"
)

func TestApplyBoost_Disabled(t *testing.T) {
	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "test_foo.go"}, Score: 0.9},
		{Chunk: store.Chunk{FilePath: "main.go"}, Score: 0.8},
	}

	boostCfg := config.BoostConfig{Enabled: false}
	boosted := ApplyBoost(results, boostCfg)

	// Should not change order when disabled
	if boosted[0].Chunk.FilePath != "test_foo.go" {
		t.Errorf("expected first result to be test_foo.go, got %s", boosted[0].Chunk.FilePath)
	}
}

func TestApplyBoost_Penalties(t *testing.T) {
	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "foo_test.go"}, Score: 0.9},
		{Chunk: store.Chunk{FilePath: "main.go"}, Score: 0.8},
	}

	boostCfg := config.BoostConfig{
		Enabled: true,
		Penalties: []config.BoostRule{
			{Pattern: "_test.go", Factor: 0.5},
		},
	}

	boosted := ApplyBoost(results, boostCfg)

	// main.go should now be first (0.8 > 0.9*0.5=0.45)
	if boosted[0].Chunk.FilePath != "main.go" {
		t.Errorf("expected main.go first after penalty, got %s", boosted[0].Chunk.FilePath)
	}

	// Verify scores
	if boosted[0].Score != 0.8 {
		t.Errorf("expected main.go score 0.8, got %f", boosted[0].Score)
	}
	if boosted[1].Score != 0.45 {
		t.Errorf("expected foo_test.go score 0.45, got %f", boosted[1].Score)
	}
}

func TestApplyBoost_Bonuses(t *testing.T) {
	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "utils/helper.go"}, Score: 0.9},
		{Chunk: store.Chunk{FilePath: "cmd/main.go"}, Score: 0.8},
	}

	boostCfg := config.BoostConfig{
		Enabled: true,
		Bonuses: []config.BoostRule{
			{Pattern: "cmd/", Factor: 1.3},
		},
	}

	boosted := ApplyBoost(results, boostCfg)

	// cmd/main.go should now be first (0.8*1.3=1.04 > 0.9)
	if boosted[0].Chunk.FilePath != "cmd/main.go" {
		t.Errorf("expected cmd/main.go first after bonus, got %s", boosted[0].Chunk.FilePath)
	}
}

func TestApplyBoost_Combined(t *testing.T) {
	results := []store.SearchResult{
		{Chunk: store.Chunk{FilePath: "cmd/main_test.go"}, Score: 1.0},
		{Chunk: store.Chunk{FilePath: "internal/handler.go"}, Score: 0.7},
	}

	boostCfg := config.BoostConfig{
		Enabled: true,
		Penalties: []config.BoostRule{
			{Pattern: "_test.go", Factor: 0.5},
		},
		Bonuses: []config.BoostRule{
			{Pattern: "cmd/", Factor: 1.3},
			{Pattern: "internal/", Factor: 1.1},
		},
	}

	boosted := ApplyBoost(results, boostCfg)

	// cmd/main_test.go: 1.0 * 0.5 * 1.3 = 0.65
	// internal/handler.go: 0.7 * 1.1 = 0.77
	if boosted[0].Chunk.FilePath != "internal/handler.go" {
		t.Errorf("expected internal/handler.go first, got %s", boosted[0].Chunk.FilePath)
	}
}

func TestApplyBoost_EmptyResults(t *testing.T) {
	results := []store.SearchResult{}
	boostCfg := config.BoostConfig{Enabled: true}

	boosted := ApplyBoost(results, boostCfg)

	if len(boosted) != 0 {
		t.Errorf("expected empty results, got %d", len(boosted))
	}
}

func TestComputeBoostFactor(t *testing.T) {
	boostCfg := config.BoostConfig{
		Enabled: true,
		Penalties: []config.BoostRule{
			{Pattern: "_test.", Factor: 0.5},
			{Pattern: "/tests/", Factor: 0.5},
		},
		Bonuses: []config.BoostRule{
			{Pattern: "/src/", Factor: 1.1},
		},
	}

	tests := []struct {
		path     string
		expected float32
	}{
		{"main.go", 1.0},
		{"foo_test.go", 0.5},
		{"project/src/main.go", 1.1},        // matches /src/
		{"project/tests/foo_test.go", 0.25}, // matches /tests/ and _test.
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			factor := computeBoostFactor(tt.path, boostCfg)
			if factor != tt.expected {
				t.Errorf("computeBoostFactor(%s) = %f, want %f", tt.path, factor, tt.expected)
			}
		})
	}
}

// Boost factors collapse distinct scores onto equal ones, so boosting can
// create ties of its own. The result order must not depend on where the tied
// results happened to sit before the sort.
func TestApplyBoost_TiedScoresOrderByChunkID(t *testing.T) {
	results := []store.SearchResult{
		{Chunk: store.Chunk{ID: "d", FilePath: "src/d.go"}, Score: 0.5},
		{Chunk: store.Chunk{ID: "b", FilePath: "b_test.go"}, Score: 1.0},
		{Chunk: store.Chunk{ID: "c", FilePath: "src/c.go"}, Score: 0.5},
		{Chunk: store.Chunk{ID: "a", FilePath: "a_test.go"}, Score: 1.0},
	}

	boostCfg := config.BoostConfig{
		Enabled:   true,
		Penalties: []config.BoostRule{{Pattern: "_test.", Factor: 0.5}},
	}

	// Every result lands on 0.5 after boosting.
	boosted := ApplyBoost(results, boostCfg)

	want := []string{"a", "b", "c", "d"}
	got := make([]string, 0, len(boosted))
	for _, r := range boosted {
		got = append(got, r.Chunk.ID)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ApplyBoost() = %v, want %v", got, want)
		}
	}
}

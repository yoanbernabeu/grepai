package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestQdrantStore_SharedCollectionIsolatedByProjectNamespace(t *testing.T) {
	endpoint := os.Getenv("GREPAI_QDRANT_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set GREPAI_QDRANT_TEST_ENDPOINT to run Qdrant integration tests")
	}

	ctx := context.Background()
	collection := "grepai_test_namespace_" + strings.ReplaceAll(time.Now().Format("20060102150405.000000000"), ".", "_")

	victim, err := NewQdrantStoreWithNamespace(ctx, endpoint, 6334, false, collection, "/repos/victim", "", 3)
	if err != nil {
		t.Fatalf("NewQdrantStore victim: %v", err)
	}
	defer victim.Close()

	attacker, err := NewQdrantStoreWithNamespace(ctx, endpoint, 6334, false, collection, "/repos/attacker", "", 3)
	if err != nil {
		t.Fatalf("NewQdrantStore attacker: %v", err)
	}
	defer attacker.Close()

	now := time.Now()
	victimContent := "File: README.md\n\nVictim private deployment runbook"
	attackerContent := "File: README.md\n\nAttacker decoy repository content"

	if err := victim.SaveChunks(ctx, []Chunk{{
		ID:        "README.md_0",
		FilePath:  "README.md",
		StartLine: 1,
		EndLine:   1,
		Content:   victimContent,
		Vector:    []float32{1, 0, 0},
		Hash:      "victim-hash",
		UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("victim SaveChunks: %v", err)
	}

	if err := attacker.SaveChunks(ctx, []Chunk{{
		ID:        "README.md_0",
		FilePath:  "README.md",
		StartLine: 1,
		EndLine:   1,
		Content:   attackerContent,
		Vector:    []float32{1, 0, 0},
		Hash:      "attacker-hash",
		UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("attacker SaveChunks: %v", err)
	}

	victimChunks, err := victim.GetChunksForFile(ctx, "README.md")
	if err != nil {
		t.Fatalf("victim GetChunksForFile: %v", err)
	}
	if len(victimChunks) != 1 {
		t.Fatalf("victim chunks = %d, want 1", len(victimChunks))
	}
	if victimChunks[0].Content != victimContent {
		t.Fatalf("victim content = %q, want %q", victimChunks[0].Content, victimContent)
	}
	victimResults, err := victim.Search(ctx, []float32{1, 0, 0}, 10, SearchOptions{})
	if err != nil {
		t.Fatalf("victim Search: %v", err)
	}
	if len(victimResults) != 1 {
		t.Fatalf("victim search results = %d, want 1", len(victimResults))
	}
	if victimResults[0].Chunk.Content != victimContent {
		t.Fatalf("victim search content = %q, want %q", victimResults[0].Chunk.Content, victimContent)
	}

	attackerChunks, err := attacker.GetChunksForFile(ctx, "README.md")
	if err != nil {
		t.Fatalf("attacker GetChunksForFile: %v", err)
	}
	if len(attackerChunks) != 1 {
		t.Fatalf("attacker chunks = %d, want 1", len(attackerChunks))
	}
	if attackerChunks[0].Content != attackerContent {
		t.Fatalf("attacker content = %q, want %q", attackerChunks[0].Content, attackerContent)
	}
	attackerResults, err := attacker.Search(ctx, []float32{1, 0, 0}, 10, SearchOptions{})
	if err != nil {
		t.Fatalf("attacker Search: %v", err)
	}
	if len(attackerResults) != 1 {
		t.Fatalf("attacker search results = %d, want 1", len(attackerResults))
	}
	if attackerResults[0].Chunk.Content != attackerContent {
		t.Fatalf("attacker search content = %q, want %q", attackerResults[0].Chunk.Content, attackerContent)
	}
}

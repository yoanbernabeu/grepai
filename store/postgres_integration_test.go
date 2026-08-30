package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreLookupByContentHashScopesCacheIdentity(t *testing.T) {
	dsn := os.Getenv("GREPAI_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("GREPAI_POSTGRES_TEST_DSN is not set")
	}

	ctx := context.Background()
	victim, err := NewPostgresStore(ctx, dsn, "proj_victim", 3, "model=text-embedding-3-large")
	if err != nil {
		t.Fatalf("failed to create victim store: %v", err)
	}
	defer victim.Close()

	attackerProject, err := NewPostgresStore(ctx, dsn, "proj_attacker", 3, "model=text-embedding-3-large")
	if err != nil {
		t.Fatalf("failed to create attacker project store: %v", err)
	}
	defer attackerProject.Close()

	attackerModel, err := NewPostgresStore(ctx, dsn, "proj_victim", 3, "model=text-embedding-3-small")
	if err != nil {
		t.Fatalf("failed to create attacker model store: %v", err)
	}
	defer attackerModel.Close()

	if _, err := victim.pool.Exec(ctx, `TRUNCATE TABLE chunks, documents`); err != nil {
		t.Fatalf("failed to clean tables: %v", err)
	}

	contentHash := "sha256:shared-content"
	victimVector := []float32{9.9, 8.8, 7.7}
	if err := victim.SaveChunks(ctx, []Chunk{{
		ID:          "README.md_0",
		FilePath:    "README.md",
		StartLine:   1,
		EndLine:     1,
		Content:     "File: README.md\n\nShared content",
		Vector:      victimVector,
		Hash:        "victim-row",
		ContentHash: contentHash,
		UpdatedAt:   time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("failed to save victim chunk: %v", err)
	}

	if _, found, err := attackerProject.LookupByContentHash(ctx, contentHash); err != nil {
		t.Fatalf("cross-project lookup failed: %v", err)
	} else if found {
		t.Fatal("different project should not reuse victim project's cached vector")
	}

	if _, found, err := attackerModel.LookupByContentHash(ctx, contentHash); err != nil {
		t.Fatalf("cross-model lookup failed: %v", err)
	} else if found {
		t.Fatal("different embedding cache namespace should not reuse victim model's cached vector")
	}

	got, found, err := victim.LookupByContentHash(ctx, contentHash)
	if err != nil {
		t.Fatalf("victim lookup failed: %v", err)
	}
	if !found {
		t.Fatal("victim project should find its own cached vector")
	}
	if len(got) != len(victimVector) {
		t.Fatalf("victim vector length mismatch: got %d, want %d", len(got), len(victimVector))
	}
	for i := range got {
		if got[i] != victimVector[i] {
			t.Fatalf("victim vector mismatch at %d: got %v, want %v", i, got, victimVector)
		}
	}
}

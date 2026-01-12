package store

import (
	"testing"
)

// TestPostgresStore_DimensionsField verifies the dimensions field is properly set.
// Note: These tests don't require a real database connection.
func TestPostgresStore_DimensionsField(t *testing.T) {
	// Test that PostgresStore struct has dimensions field
	store := &PostgresStore{
		projectID:  "test-project",
		dimensions: 768,
	}

	if store.dimensions != 768 {
		t.Errorf("expected dimensions 768, got %d", store.dimensions)
	}

	// Test different dimension values
	tests := []struct {
		name       string
		dimensions int
	}{
		{"nomic-embed-text", 768},
		{"text-embedding-3-small", 1536},
		{"text-embedding-3-large", 3072},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PostgresStore{
				projectID:  "test",
				dimensions: tt.dimensions,
			}
			if s.dimensions != tt.dimensions {
				t.Errorf("expected dimensions %d, got %d", tt.dimensions, s.dimensions)
			}
		})
	}
}

// TestPostgresStore_ProjectID verifies project ID is properly set
func TestPostgresStore_ProjectID(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
	}{
		{"simple", "my-project"},
		{"with path", "/Users/test/project"},
		{"with special chars", "project-123_test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PostgresStore{
				projectID:  tt.projectID,
				dimensions: 768,
			}
			if s.projectID != tt.projectID {
				t.Errorf("expected projectID %s, got %s", tt.projectID, s.projectID)
			}
		})
	}
}

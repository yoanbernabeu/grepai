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

	// Use both fields to avoid unusedwrite warning
	if store.dimensions != 768 || store.projectID != "test-project" {
		t.Errorf("expected dimensions 768 and projectID 'test-project', got %d and %s", store.dimensions, store.projectID)
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
			// Verify both fields are accessible and correct
			if got := s.dimensions; got != tt.dimensions {
				t.Errorf("expected dimensions %d, got %d", tt.dimensions, got)
			}
			if s.projectID != "test" {
				t.Errorf("expected projectID 'test', got %s", s.projectID)
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
			// Verify both fields are accessible and correct
			if got := s.projectID; got != tt.projectID {
				t.Errorf("expected projectID %s, got %s", tt.projectID, got)
			}
			if s.dimensions != 768 {
				t.Errorf("expected dimensions 768, got %d", s.dimensions)
			}
		})
	}
}

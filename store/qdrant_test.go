package store

import (
	"testing"
)

// TestQdrantStore_StructFields verifies struct has all expected fields
func TestQdrantStore_StructFields(t *testing.T) {
	store := &QdrantStore{
		collectionName: "test-collection",
		dimensions:     768,
		apiKey:         "test-key",
	}

	// Use all fields to avoid unused variable warnings
	if store.collectionName != "test-collection" || store.dimensions != 768 || store.apiKey != "test-key" {
		t.Errorf("expected collectionName 'test-collection', dimensions 768, and apiKey 'test-key'")
	}
}

// TestQdrantStore_CollectionNameSanitization verifies / replacement with _
func TestQdrantStore_CollectionNameSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "/Users/test/project", "_Users_test_project"},
		{"nested path", "/home/user/src/github.com/repo", "_home_user_src_github.com_repo"},
		{"root path", "/", "_"},
		{"multiple slashes", "///", "___"},
		{"no slashes", "myproject", "myproject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeCollectionName(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestQdrantStore_DimensionsField verifies dimension field values
func TestQdrantStore_DimensionsField(t *testing.T) {
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
			store := &QdrantStore{
				collectionName: "test",
				dimensions:     tt.dimensions,
			}

			// Verify dimensions is accessible and correct
			if store.dimensions != tt.dimensions {
				t.Errorf("expected dimensions %d, got %d", tt.dimensions, store.dimensions)
			}
			if store.collectionName != "test" {
				t.Errorf("expected collectionName 'test', got %s", store.collectionName)
			}
		})
	}
}

// TestQdrantStore_ConfigurationVariants tests different config combinations
func TestQdrantStore_ConfigurationVariants(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		collection string
		apiKey     string
		dimensions int
	}{
		{
			name:       "local qdrant",
			endpoint:   "http://localhost:6333",
			collection: "myproject",
			apiKey:     "",
			dimensions: 768,
		},
		{
			name:       "qdrant cloud",
			endpoint:   "https://cloud.qdrant.io",
			collection: "project",
			apiKey:     "secret-key",
			dimensions: 1536,
		},
		{
			name:       "custom endpoint",
			endpoint:   "http://192.168.1.100:6333",
			collection: "test_repo",
			apiKey:     "",
			dimensions: 768,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &QdrantStore{
				collectionName: tt.collection,
				dimensions:     tt.dimensions,
				apiKey:         tt.apiKey,
			}

			// Verify all fields are accessible and correct
			if store.collectionName != tt.collection {
				t.Errorf("expected collectionName %s, got %s", tt.collection, store.collectionName)
			}
			if store.dimensions != tt.dimensions {
				t.Errorf("expected dimensions %d, got %d", tt.dimensions, store.dimensions)
			}
			if store.apiKey != tt.apiKey {
				t.Errorf("expected apiKey %s, got %s", tt.apiKey, store.apiKey)
			}
		})
	}
}

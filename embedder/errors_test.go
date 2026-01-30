package embedder

import (
	"errors"
	"fmt"
	"testing"
)

func TestContextLengthError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ContextLengthError
		contains []string
	}{
		{
			name: "with max tokens",
			err: &ContextLengthError{
				ChunkIndex:      0,
				EstimatedTokens: 10000,
				MaxTokens:       8192,
				Message:         "input exceeds context length",
			},
			contains: []string{"chunk 0", "~10000 tokens", "max 8192", "input exceeds context length"},
		},
		{
			name: "without max tokens",
			err: &ContextLengthError{
				ChunkIndex:      2,
				EstimatedTokens: 5000,
				MaxTokens:       0,
				Message:         "context limit exceeded",
			},
			contains: []string{"chunk 2", "~5000 tokens", "context limit exceeded"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			for _, substr := range tt.contains {
				if !containsString(msg, substr) {
					t.Errorf("error message %q should contain %q", msg, substr)
				}
			}
		})
	}
}

func TestNewContextLengthError(t *testing.T) {
	err := NewContextLengthError(1, 9000, 8192, "too long")

	if err.ChunkIndex != 1 {
		t.Errorf("ChunkIndex = %d, expected 1", err.ChunkIndex)
	}
	if err.EstimatedTokens != 9000 {
		t.Errorf("EstimatedTokens = %d, expected 9000", err.EstimatedTokens)
	}
	if err.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, expected 8192", err.MaxTokens)
	}
	if err.Message != "too long" {
		t.Errorf("Message = %q, expected %q", err.Message, "too long")
	}
}

func TestIsContextLengthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ContextLengthError",
			err:      &ContextLengthError{Message: "test"},
			expected: true,
		},
		{
			name:     "wrapped ContextLengthError",
			err:      fmt.Errorf("outer error: %w", &ContextLengthError{Message: "test"}),
			expected: true,
		},
		{
			name:     "double wrapped ContextLengthError",
			err:      fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &ContextLengthError{Message: "test"})),
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsContextLengthError(tt.err)
			if result != tt.expected {
				t.Errorf("IsContextLengthError() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestAsContextLengthError(t *testing.T) {
	origErr := &ContextLengthError{ChunkIndex: 5, Message: "original"}

	tests := []struct {
		name          string
		err           error
		expectNil     bool
		expectedIndex int
	}{
		{
			name:          "direct error",
			err:           origErr,
			expectNil:     false,
			expectedIndex: 5,
		},
		{
			name:          "wrapped error",
			err:           fmt.Errorf("wrapped: %w", origErr),
			expectNil:     false,
			expectedIndex: 5,
		},
		{
			name:      "non-ContextLengthError",
			err:       errors.New("other error"),
			expectNil: true,
		},
		{
			name:      "nil error",
			err:       nil,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AsContextLengthError(tt.err)
			if tt.expectNil {
				if result != nil {
					t.Error("expected nil, got non-nil")
				}
			} else {
				if result == nil {
					t.Fatal("expected non-nil, got nil")
				}
				if result.ChunkIndex != tt.expectedIndex {
					t.Errorf("ChunkIndex = %d, expected %d", result.ChunkIndex, tt.expectedIndex)
				}
			}
		})
	}
}

// containsString is a helper to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

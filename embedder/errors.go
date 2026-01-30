package embedder

import (
	"errors"
	"fmt"
)

// ContextLengthError indicates that the input text exceeds the model's context limit.
// This error is retryable by re-chunking the input into smaller pieces.
type ContextLengthError struct {
	ChunkIndex      int    // Index of the chunk that exceeded the limit
	EstimatedTokens int    // Estimated number of tokens in the chunk
	MaxTokens       int    // Maximum tokens allowed by the model (if known)
	Message         string // Original error message from the provider
}

func (e *ContextLengthError) Error() string {
	if e.MaxTokens > 0 {
		return fmt.Sprintf("chunk %d exceeds context limit: ~%d tokens (max %d): %s",
			e.ChunkIndex, e.EstimatedTokens, e.MaxTokens, e.Message)
	}
	return fmt.Sprintf("chunk %d exceeds context limit: ~%d tokens: %s",
		e.ChunkIndex, e.EstimatedTokens, e.Message)
}

// NewContextLengthError creates a new ContextLengthError.
func NewContextLengthError(chunkIndex, estimatedTokens, maxTokens int, message string) *ContextLengthError {
	return &ContextLengthError{
		ChunkIndex:      chunkIndex,
		EstimatedTokens: estimatedTokens,
		MaxTokens:       maxTokens,
		Message:         message,
	}
}

// IsContextLengthError checks if an error is or wraps a ContextLengthError.
func IsContextLengthError(err error) bool {
	var ctxErr *ContextLengthError
	return errors.As(err, &ctxErr)
}

// AsContextLengthError extracts a ContextLengthError from an error chain.
// Returns nil if the error is not a ContextLengthError.
func AsContextLengthError(err error) *ContextLengthError {
	var ctxErr *ContextLengthError
	if errors.As(err, &ctxErr) {
		return ctxErr
	}
	return nil
}

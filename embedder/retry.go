package embedder

import (
	"math"
	"math/rand/v2"
	"net/http"
	"time"
)

// RetryPolicy configures exponential backoff retry behavior for API calls.
type RetryPolicy struct {
	// BaseDelay is the initial delay before the first retry (default: 1s)
	BaseDelay time.Duration
	// Multiplier is the factor to multiply delay by on each retry (default: 2.0)
	Multiplier float64
	// MaxDelay is the maximum delay cap (default: 32s)
	MaxDelay time.Duration
	// MaxAttempts is the maximum number of retry attempts (default: 5)
	MaxAttempts int
}

// DefaultRetryPolicy returns a RetryPolicy with sensible defaults for OpenAI API.
// BaseDelay: 1s, Multiplier: 2x, MaxDelay: 32s, MaxAttempts: 5
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		BaseDelay:   1 * time.Second,
		Multiplier:  2.0,
		MaxDelay:    32 * time.Second,
		MaxAttempts: 5,
	}
}

// Calculate returns the delay for the given attempt number (0-indexed) with jitter.
// Jitter adds random variance (0-100% of calculated delay) to prevent thundering herd.
func (p RetryPolicy) Calculate(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Calculate exponential delay: base * multiplier^attempt
	delay := float64(p.BaseDelay) * math.Pow(p.Multiplier, float64(attempt))

	// Cap at MaxDelay
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	// Add jitter: random 0-100% of calculated delay
	// Cryptographic randomness not required for retry jitter - just prevents thundering herd
	jitter := rand.Float64() * delay //nolint:gosec // G404: non-security use case
	delay += jitter

	return time.Duration(delay)
}

// ShouldRetry returns true if more attempts are available.
// attempt is 0-indexed (0 = first attempt, 1 = first retry, etc.)
func (p RetryPolicy) ShouldRetry(attempt int) bool {
	return attempt < p.MaxAttempts
}

// IsRetryable returns true if the HTTP status code indicates a retryable error.
// Retryable: 429 (rate limit), 5xx (server errors)
// Non-retryable: 4xx client errors (except 429)
func IsRetryable(statusCode int) bool {
	// 429 Too Many Requests is retryable
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	// 5xx server errors are retryable
	if statusCode >= 500 && statusCode < 600 {
		return true
	}
	// All other errors (including 4xx client errors) are not retryable
	return false
}

// RetryableError wraps an error with its HTTP status code for retry decisions.
type RetryableError struct {
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *RetryableError) Error() string {
	return e.Message
}

// NewRetryableError creates a RetryableError from an HTTP status code and message.
func NewRetryableError(statusCode int, message string) *RetryableError {
	return &RetryableError{
		StatusCode: statusCode,
		Message:    message,
		Retryable:  IsRetryable(statusCode),
	}
}

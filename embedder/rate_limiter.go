package embedder

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimitHeaders contains parsed rate limit information from HTTP response headers.
type RateLimitHeaders struct {
	// RetryAfter is the duration to wait before retrying (from Retry-After header).
	// Zero if header not present.
	RetryAfter time.Duration

	// RemainingTokens from x-ratelimit-remaining-tokens header.
	// -1 if header not present.
	RemainingTokens int

	// RemainingRequests from x-ratelimit-remaining-requests header.
	// -1 if header not present.
	RemainingRequests int

	// ResetTokens duration from x-ratelimit-reset-tokens header.
	// Zero if header not present.
	ResetTokens time.Duration
}

// parseRetryAfter parses the Retry-After header value.
// It supports both integer seconds format (e.g., "30") and HTTP-date format per RFC 7231.
// Returns zero duration if the header is empty, invalid, or unparseable.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}

	// Try integer seconds first (most common from OpenAI)
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	// Try HTTP-date format (RFC 7231)
	// Format: "Mon, 02 Jan 2006 15:04:05 GMT"
	if t, err := time.Parse(http.TimeFormat, value); err == nil {
		delay := time.Until(t)
		if delay < 0 {
			return 0
		}
		return delay
	}

	return 0
}

// parseRateLimitHeaders extracts rate limit information from HTTP response headers.
func parseRateLimitHeaders(header http.Header) RateLimitHeaders {
	h := RateLimitHeaders{
		RemainingTokens:   -1,
		RemainingRequests: -1,
	}

	// Parse Retry-After header
	h.RetryAfter = parseRetryAfter(header.Get("Retry-After"))

	// Parse x-ratelimit-remaining-tokens
	if v := header.Get("x-ratelimit-remaining-tokens"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			h.RemainingTokens = n
		}
	}

	// Parse x-ratelimit-remaining-requests
	if v := header.Get("x-ratelimit-remaining-requests"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			h.RemainingRequests = n
		}
	}

	// Parse x-ratelimit-reset-tokens (format: "1s", "500ms", "1m30s")
	if v := header.Get("x-ratelimit-reset-tokens"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			h.ResetTokens = d
		}
	}

	return h
}

// AdaptiveRateLimiter coordinates adaptive parallelism adjustment based on rate limit feedback.
// It reduces parallelism when rate limits are detected and gradually restores it after successful requests.
type AdaptiveRateLimiter struct {
	// currentWorkers is the current parallelism level (1 <= value <= maxWorkers)
	currentWorkers atomic.Int32

	// maxWorkers is the configured maximum parallelism
	maxWorkers int

	// rateLimitHits counts consecutive 429 responses (reset on success)
	rateLimitHits atomic.Int32

	// successStreak counts consecutive successful requests (reset on rate limit)
	successStreak atomic.Int32

	// reductionThreshold is the number of consecutive 429s before reducing parallelism
	reductionThreshold int

	// restorationThreshold is the number of consecutive successes before increasing parallelism
	restorationThreshold int

	// mu protects coordinated operations
	mu sync.RWMutex

	// lastReductionTime tracks when parallelism was last reduced to avoid rapid reductions
	lastReductionTime time.Time
}

// NewAdaptiveRateLimiter creates a new AdaptiveRateLimiter with the given maximum parallelism.
// Default thresholds: reductionThreshold=3, restorationThreshold=10
func NewAdaptiveRateLimiter(maxWorkers int) *AdaptiveRateLimiter {
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	arl := &AdaptiveRateLimiter{
		maxWorkers:           maxWorkers,
		reductionThreshold:   3,
		restorationThreshold: 10,
	}
	arl.currentWorkers.Store(int32(maxWorkers)) //nolint:gosec // maxWorkers validated to be small positive int

	return arl
}

// CurrentWorkers returns the current parallelism level.
func (arl *AdaptiveRateLimiter) CurrentWorkers() int {
	return int(arl.currentWorkers.Load())
}

// MaxWorkers returns the configured maximum parallelism.
func (arl *AdaptiveRateLimiter) MaxWorkers() int {
	return arl.maxWorkers
}

// OnRateLimitHit should be called when a 429 response is received.
// It increments the rate limit hit counter and may reduce parallelism.
// Returns true if parallelism was reduced.
func (arl *AdaptiveRateLimiter) OnRateLimitHit() bool {
	// Reset success streak on rate limit
	arl.successStreak.Store(0)

	// Increment rate limit hits
	hits := arl.rateLimitHits.Add(1)

	// Check if we should reduce parallelism
	if int(hits) >= arl.reductionThreshold {
		return arl.reduceParallelism()
	}

	return false
}

// reduceParallelism halves the current parallelism level, never going below 1.
// Returns true if parallelism was actually reduced.
func (arl *AdaptiveRateLimiter) reduceParallelism() bool {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	current := int(arl.currentWorkers.Load())
	if current <= 1 {
		// Already at minimum
		arl.rateLimitHits.Store(0)
		return false
	}

	// Halve parallelism
	newLevel := current / 2
	if newLevel < 1 {
		newLevel = 1
	}

	arl.currentWorkers.Store(int32(newLevel)) //nolint:gosec // newLevel is always small positive (>=1)
	arl.rateLimitHits.Store(0)
	arl.lastReductionTime = time.Now()

	log.Printf("Rate limit: reducing parallelism from %d to %d due to consecutive 429 responses", current, newLevel)

	return true
}

// OnSuccess should be called when a request succeeds.
// It increments the success streak counter and may restore parallelism.
// Returns true if parallelism was restored.
func (arl *AdaptiveRateLimiter) OnSuccess() bool {
	// Reset rate limit hits on success
	arl.rateLimitHits.Store(0)

	// Increment success streak
	streak := arl.successStreak.Add(1)

	// Check if we should restore parallelism
	if int(streak) >= arl.restorationThreshold {
		return arl.restoreParallelism()
	}

	return false
}

// restoreParallelism increments the current parallelism level by 1, up to maxWorkers.
// Returns true if parallelism was actually increased.
func (arl *AdaptiveRateLimiter) restoreParallelism() bool {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	current := int(arl.currentWorkers.Load())
	if current >= arl.maxWorkers {
		// Already at maximum
		arl.successStreak.Store(0)
		return false
	}

	// Increment by 1 for gradual restoration
	newLevel := current + 1
	if newLevel > arl.maxWorkers {
		newLevel = arl.maxWorkers
	}

	arl.currentWorkers.Store(int32(newLevel)) //nolint:gosec // newLevel is always small positive (<=maxWorkers)
	arl.successStreak.Store(0)

	log.Printf("Rate limit: restoring parallelism from %d to %d after successful requests", current, newLevel)

	return true
}

// TokenBucket tracks token usage for proactive rate limiting using a sliding window.
type TokenBucket struct {
	// tokensUsed in the current minute window
	tokensUsed atomic.Int64

	// windowStart is the Unix timestamp (seconds) of the current window start
	windowStart atomic.Int64

	// tokensPerMinute is the configured TPM limit
	tokensPerMinute int64

	// mu protects window reset operations
	mu sync.Mutex
}

// NewTokenBucket creates a new TokenBucket with the given tokens-per-minute limit.
// Default for OpenAI Tier 1 embeddings is 1,000,000 TPM.
func NewTokenBucket(tokensPerMinute int64) *TokenBucket {
	if tokensPerMinute <= 0 {
		tokensPerMinute = 1_000_000 // Default to Tier 1 limit
	}

	tb := &TokenBucket{
		tokensPerMinute: tokensPerMinute,
	}
	tb.windowStart.Store(time.Now().Unix())

	return tb
}

// TokensAvailable returns the number of tokens available in the current window.
// Resets the window if a minute has passed.
func (tb *TokenBucket) TokensAvailable() int64 {
	tb.maybeResetWindow()
	used := tb.tokensUsed.Load()
	available := tb.tokensPerMinute - used
	if available < 0 {
		return 0
	}
	return available
}

// AddTokens records token usage. Returns true if tokens were successfully added.
func (tb *TokenBucket) AddTokens(tokens int64) bool {
	tb.maybeResetWindow()
	tb.tokensUsed.Add(tokens)
	return true
}

// WaitForTokens checks if the requested tokens are available.
// Returns the duration to wait if tokens are not available, or 0 if they are.
func (tb *TokenBucket) WaitForTokens(tokens int64) time.Duration {
	tb.maybeResetWindow()

	used := tb.tokensUsed.Load()
	if used+tokens <= tb.tokensPerMinute {
		return 0
	}

	// Calculate time until window reset
	windowStart := tb.windowStart.Load()
	elapsed := time.Since(time.Unix(windowStart, 0))
	remaining := time.Minute - elapsed

	if remaining < 0 {
		return 0
	}

	return remaining
}

// maybeResetWindow resets the token counter if a minute has passed.
func (tb *TokenBucket) maybeResetWindow() {
	now := time.Now().Unix()
	windowStart := tb.windowStart.Load()

	// Check if at least 60 seconds have passed
	if now-windowStart >= 60 {
		tb.mu.Lock()
		defer tb.mu.Unlock()

		// Double-check after acquiring lock
		if now-tb.windowStart.Load() >= 60 {
			tb.windowStart.Store(now)
			tb.tokensUsed.Store(0)
		}
	}
}

// ParseRateLimitHeadersForTest exports parseRateLimitHeaders for testing.
func ParseRateLimitHeadersForTest(header http.Header) RateLimitHeaders {
	return parseRateLimitHeaders(header)
}

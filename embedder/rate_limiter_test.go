package embedder_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
)

func TestParseRetryAfter_IntegerSeconds(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected time.Duration
	}{
		{"empty string", "", 0},
		{"zero", "0", 0},
		{"positive seconds", "30", 30 * time.Second},
		{"large value", "120", 120 * time.Second},
		{"negative value", "-5", 0},
		{"invalid format", "abc", 0},
		{"float format", "30.5", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.value != "" {
				header.Set("Retry-After", tt.value)
			}
			headers := embedder.ParseRateLimitHeadersForTest(header)
			if headers.RetryAfter != tt.expected {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.value, headers.RetryAfter, tt.expected)
			}
		})
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	// Test HTTP-date format
	futureTime := time.Now().Add(60 * time.Second).UTC()
	httpDate := futureTime.Format(http.TimeFormat)

	header := http.Header{}
	header.Set("Retry-After", httpDate)
	headers := embedder.ParseRateLimitHeadersForTest(header)

	// Allow 2 second tolerance for test execution time
	if headers.RetryAfter < 58*time.Second || headers.RetryAfter > 62*time.Second {
		t.Errorf("ParseRetryAfter(HTTP-date) = %v, expected ~60s", headers.RetryAfter)
	}
}

func TestParseRetryAfter_PastHTTPDate(t *testing.T) {
	// Test HTTP-date in the past
	pastTime := time.Now().Add(-60 * time.Second).UTC()
	httpDate := pastTime.Format(http.TimeFormat)

	header := http.Header{}
	header.Set("Retry-After", httpDate)
	headers := embedder.ParseRateLimitHeadersForTest(header)

	if headers.RetryAfter != 0 {
		t.Errorf("ParseRetryAfter(past HTTP-date) = %v, want 0", headers.RetryAfter)
	}
}

func TestParseRateLimitHeaders(t *testing.T) {
	header := http.Header{}
	header.Set("Retry-After", "30")
	header.Set("x-ratelimit-remaining-tokens", "50000")
	header.Set("x-ratelimit-remaining-requests", "100")
	header.Set("x-ratelimit-reset-tokens", "5s")

	headers := embedder.ParseRateLimitHeadersForTest(header)

	if headers.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want 30s", headers.RetryAfter)
	}
	if headers.RemainingTokens != 50000 {
		t.Errorf("RemainingTokens = %d, want 50000", headers.RemainingTokens)
	}
	if headers.RemainingRequests != 100 {
		t.Errorf("RemainingRequests = %d, want 100", headers.RemainingRequests)
	}
	if headers.ResetTokens != 5*time.Second {
		t.Errorf("ResetTokens = %v, want 5s", headers.ResetTokens)
	}
}

func TestParseRateLimitHeaders_MissingHeaders(t *testing.T) {
	header := http.Header{}

	headers := embedder.ParseRateLimitHeadersForTest(header)

	if headers.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0", headers.RetryAfter)
	}
	if headers.RemainingTokens != -1 {
		t.Errorf("RemainingTokens = %d, want -1", headers.RemainingTokens)
	}
	if headers.RemainingRequests != -1 {
		t.Errorf("RemainingRequests = %d, want -1", headers.RemainingRequests)
	}
	if headers.ResetTokens != 0 {
		t.Errorf("ResetTokens = %v, want 0", headers.ResetTokens)
	}
}

func TestParseRateLimitHeaders_InvalidValues(t *testing.T) {
	header := http.Header{}
	header.Set("x-ratelimit-remaining-tokens", "invalid")
	header.Set("x-ratelimit-remaining-requests", "not-a-number")
	header.Set("x-ratelimit-reset-tokens", "bad-duration")

	headers := embedder.ParseRateLimitHeadersForTest(header)

	if headers.RemainingTokens != -1 {
		t.Errorf("RemainingTokens = %d, want -1 for invalid", headers.RemainingTokens)
	}
	if headers.RemainingRequests != -1 {
		t.Errorf("RemainingRequests = %d, want -1 for invalid", headers.RemainingRequests)
	}
	if headers.ResetTokens != 0 {
		t.Errorf("ResetTokens = %v, want 0 for invalid", headers.ResetTokens)
	}
}

func TestAdaptiveRateLimiter_NewWithDefaults(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(4)

	if arl.CurrentWorkers() != 4 {
		t.Errorf("CurrentWorkers() = %d, want 4", arl.CurrentWorkers())
	}
	if arl.MaxWorkers() != 4 {
		t.Errorf("MaxWorkers() = %d, want 4", arl.MaxWorkers())
	}
}

func TestAdaptiveRateLimiter_NewWithInvalidMaxWorkers(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(0)

	if arl.CurrentWorkers() != 1 {
		t.Errorf("CurrentWorkers() = %d, want 1 for invalid input", arl.CurrentWorkers())
	}

	arl2 := embedder.NewAdaptiveRateLimiter(-5)
	if arl2.CurrentWorkers() != 1 {
		t.Errorf("CurrentWorkers() = %d, want 1 for negative input", arl2.CurrentWorkers())
	}
}

func TestAdaptiveRateLimiter_ReductionAfterThreshold(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(4)

	// First two hits should not reduce
	for i := 0; i < 2; i++ {
		reduced := arl.OnRateLimitHit()
		if reduced {
			t.Errorf("OnRateLimitHit() returned true on hit %d, want false", i+1)
		}
		if arl.CurrentWorkers() != 4 {
			t.Errorf("CurrentWorkers() = %d after hit %d, want 4", arl.CurrentWorkers(), i+1)
		}
	}

	// Third hit should trigger reduction (default threshold is 3)
	reduced := arl.OnRateLimitHit()
	if !reduced {
		t.Error("OnRateLimitHit() returned false on hit 3, want true")
	}
	if arl.CurrentWorkers() != 2 {
		t.Errorf("CurrentWorkers() = %d after reduction, want 2", arl.CurrentWorkers())
	}
}

func TestAdaptiveRateLimiter_NeverBelowOne(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(2)

	// Reduce from 2 to 1
	for i := 0; i < 3; i++ {
		arl.OnRateLimitHit()
	}
	if arl.CurrentWorkers() != 1 {
		t.Errorf("CurrentWorkers() = %d, want 1", arl.CurrentWorkers())
	}

	// Try to reduce further
	for i := 0; i < 10; i++ {
		arl.OnRateLimitHit()
	}
	if arl.CurrentWorkers() != 1 {
		t.Errorf("CurrentWorkers() = %d after multiple hits at min, want 1", arl.CurrentWorkers())
	}
}

func TestAdaptiveRateLimiter_RestorationAfterThreshold(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(4)

	// First reduce
	for i := 0; i < 3; i++ {
		arl.OnRateLimitHit()
	}
	if arl.CurrentWorkers() != 2 {
		t.Fatalf("Setup failed: CurrentWorkers() = %d, want 2", arl.CurrentWorkers())
	}

	// 9 successes should not restore yet (default threshold is 10)
	for i := 0; i < 9; i++ {
		restored := arl.OnSuccess()
		if restored {
			t.Errorf("OnSuccess() returned true on success %d, want false", i+1)
		}
	}
	if arl.CurrentWorkers() != 2 {
		t.Errorf("CurrentWorkers() = %d after 9 successes, want 2", arl.CurrentWorkers())
	}

	// 10th success should trigger restoration
	restored := arl.OnSuccess()
	if !restored {
		t.Error("OnSuccess() returned false on success 10, want true")
	}
	if arl.CurrentWorkers() != 3 {
		t.Errorf("CurrentWorkers() = %d after restoration, want 3", arl.CurrentWorkers())
	}
}

func TestAdaptiveRateLimiter_NeverExceedsMax(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(4)

	// Try to restore when already at max
	for i := 0; i < 20; i++ {
		arl.OnSuccess()
	}
	if arl.CurrentWorkers() != 4 {
		t.Errorf("CurrentWorkers() = %d, want 4 (max)", arl.CurrentWorkers())
	}
}

func TestAdaptiveRateLimiter_CountersReset(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(4)

	// Two rate limit hits
	arl.OnRateLimitHit()
	arl.OnRateLimitHit()

	// Success should reset rate limit hits counter
	arl.OnSuccess()

	// Now we need 3 more hits for reduction (counter was reset)
	arl.OnRateLimitHit()
	arl.OnRateLimitHit()
	if arl.CurrentWorkers() != 4 {
		t.Errorf("CurrentWorkers() = %d, want 4 (counter should have reset)", arl.CurrentWorkers())
	}

	// Third hit should trigger reduction
	arl.OnRateLimitHit()
	if arl.CurrentWorkers() != 2 {
		t.Errorf("CurrentWorkers() = %d, want 2", arl.CurrentWorkers())
	}
}

func TestAdaptiveRateLimiter_SuccessStreakResetOnRateLimit(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(4)

	// Reduce first
	for i := 0; i < 3; i++ {
		arl.OnRateLimitHit()
	}

	// 5 successes
	for i := 0; i < 5; i++ {
		arl.OnSuccess()
	}

	// Rate limit should reset success streak
	arl.OnRateLimitHit()

	// Now we need 10 more successes for restoration
	for i := 0; i < 9; i++ {
		arl.OnSuccess()
	}
	if arl.CurrentWorkers() != 2 {
		t.Errorf("CurrentWorkers() = %d, want 2 (streak should have reset)", arl.CurrentWorkers())
	}

	// 10th success should restore
	arl.OnSuccess()
	if arl.CurrentWorkers() != 3 {
		t.Errorf("CurrentWorkers() = %d, want 3", arl.CurrentWorkers())
	}
}

func TestAdaptiveRateLimiter_ConcurrentAccess(t *testing.T) {
	arl := embedder.NewAdaptiveRateLimiter(8)

	var wg sync.WaitGroup
	numGoroutines := 100
	iterationsPerGoroutine := 100

	// Half the goroutines call OnRateLimitHit, half call OnSuccess
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				arl.OnRateLimitHit()
			}
		}()
	}

	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				arl.OnSuccess()
			}
		}()
	}

	wg.Wait()

	// Just check that we didn't crash and state is valid
	workers := arl.CurrentWorkers()
	if workers < 1 || workers > 8 {
		t.Errorf("CurrentWorkers() = %d, want 1-8", workers)
	}
}

func TestTokenBucket_Basic(t *testing.T) {
	tb := embedder.NewTokenBucket(1000)

	available := tb.TokensAvailable()
	if available != 1000 {
		t.Errorf("TokensAvailable() = %d, want 1000", available)
	}

	tb.AddTokens(300)
	available = tb.TokensAvailable()
	if available != 700 {
		t.Errorf("TokensAvailable() = %d after adding 300, want 700", available)
	}
}

func TestTokenBucket_DefaultLimit(t *testing.T) {
	tb := embedder.NewTokenBucket(0)
	available := tb.TokensAvailable()
	if available != 1_000_000 {
		t.Errorf("TokensAvailable() = %d, want 1000000 (default)", available)
	}

	tb2 := embedder.NewTokenBucket(-100)
	available2 := tb2.TokensAvailable()
	if available2 != 1_000_000 {
		t.Errorf("TokensAvailable() = %d for negative input, want 1000000", available2)
	}
}

func TestTokenBucket_WaitForTokens(t *testing.T) {
	tb := embedder.NewTokenBucket(100)

	// Should not need to wait when under limit
	wait := tb.WaitForTokens(50)
	if wait != 0 {
		t.Errorf("WaitForTokens(50) = %v, want 0", wait)
	}

	// Add tokens up to limit
	tb.AddTokens(100)

	// Now should need to wait
	wait = tb.WaitForTokens(10)
	if wait <= 0 {
		t.Errorf("WaitForTokens(10) = %v after exhaustion, want > 0", wait)
	}
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	tb := embedder.NewTokenBucket(1_000_000)

	var wg sync.WaitGroup
	numGoroutines := 100
	tokensPerGoroutine := int64(1000)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tb.AddTokens(tokensPerGoroutine)
		}()
	}

	wg.Wait()

	// All tokens should be accounted for
	available := tb.TokensAvailable()
	expected := int64(1_000_000 - numGoroutines*int(tokensPerGoroutine))
	if available != expected {
		t.Errorf("TokensAvailable() = %d, want %d", available, expected)
	}
}

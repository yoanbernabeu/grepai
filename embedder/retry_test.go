package embedder

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.BaseDelay != 1*time.Second {
		t.Errorf("expected BaseDelay 1s, got %v", policy.BaseDelay)
	}
	if policy.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %v", policy.Multiplier)
	}
	if policy.MaxDelay != 32*time.Second {
		t.Errorf("expected MaxDelay 32s, got %v", policy.MaxDelay)
	}
	if policy.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts 5, got %v", policy.MaxAttempts)
	}
}

func TestRetryPolicyCalculate_ExponentialBackoff(t *testing.T) {
	policy := RetryPolicy{
		BaseDelay:   1 * time.Second,
		Multiplier:  2.0,
		MaxDelay:    32 * time.Second,
		MaxAttempts: 5,
	}

	// Test exponential progression without jitter (check base values)
	// Since jitter adds 0-100% of delay, we check that delays are in expected ranges
	tests := []struct {
		attempt int
		minBase time.Duration // base delay before jitter
		maxBase time.Duration // base delay after 100% jitter
	}{
		{0, 1 * time.Second, 2 * time.Second},   // 1s + (0-1s jitter)
		{1, 2 * time.Second, 4 * time.Second},   // 2s + (0-2s jitter)
		{2, 4 * time.Second, 8 * time.Second},   // 4s + (0-4s jitter)
		{3, 8 * time.Second, 16 * time.Second},  // 8s + (0-8s jitter)
		{4, 16 * time.Second, 32 * time.Second}, // 16s + (0-16s jitter)
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			// Run multiple times to check jitter is applied
			for i := 0; i < 10; i++ {
				delay := policy.Calculate(tt.attempt)
				if delay < tt.minBase {
					t.Errorf("attempt %d: delay %v < min %v", tt.attempt, delay, tt.minBase)
				}
				if delay > tt.maxBase {
					t.Errorf("attempt %d: delay %v > max %v", tt.attempt, delay, tt.maxBase)
				}
			}
		})
	}
}

func TestRetryPolicyCalculate_MaxDelayCap(t *testing.T) {
	policy := RetryPolicy{
		BaseDelay:   1 * time.Second,
		Multiplier:  2.0,
		MaxDelay:    32 * time.Second,
		MaxAttempts: 10,
	}

	// At attempt 5: base would be 32s, at attempt 6: 64s, but should be capped
	for attempt := 5; attempt < 10; attempt++ {
		for i := 0; i < 10; i++ {
			delay := policy.Calculate(attempt)
			// With jitter, max possible is 32s + 32s = 64s
			if delay > 64*time.Second {
				t.Errorf("attempt %d: delay %v exceeded 64s cap (32s + 100%% jitter)", attempt, delay)
			}
			// Minimum should be 32s (base at cap)
			if delay < 32*time.Second {
				t.Errorf("attempt %d: delay %v less than 32s base", attempt, delay)
			}
		}
	}
}

func TestRetryPolicyCalculate_NegativeAttempt(t *testing.T) {
	policy := DefaultRetryPolicy()

	// Negative attempts should be treated as 0
	delay := policy.Calculate(-1)
	if delay < 1*time.Second || delay > 2*time.Second {
		t.Errorf("negative attempt delay %v not in expected range [1s, 2s]", delay)
	}
}

func TestRetryPolicyCalculate_JitterVariance(t *testing.T) {
	policy := RetryPolicy{
		BaseDelay:   1 * time.Second,
		Multiplier:  2.0,
		MaxDelay:    32 * time.Second,
		MaxAttempts: 5,
	}

	// Run multiple calculations and verify we get some variance (jitter)
	delays := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		delay := policy.Calculate(0)
		delays[delay] = true
	}

	// With jitter, we should have multiple different delay values
	if len(delays) < 2 {
		t.Error("expected jitter to produce variance in delays, but all delays were identical")
	}
}

func TestRetryPolicyShouldRetry(t *testing.T) {
	policy := RetryPolicy{
		BaseDelay:   1 * time.Second,
		Multiplier:  2.0,
		MaxDelay:    32 * time.Second,
		MaxAttempts: 5,
	}

	tests := []struct {
		attempt  int
		expected bool
	}{
		{0, true},  // first attempt
		{1, true},  // first retry
		{2, true},  // second retry
		{3, true},  // third retry
		{4, true},  // fourth retry
		{5, false}, // fifth retry (at limit)
		{6, false}, // beyond limit
		{10, false},
	}

	for _, tt := range tests {
		result := policy.ShouldRetry(tt.attempt)
		if result != tt.expected {
			t.Errorf("ShouldRetry(%d) = %v, expected %v", tt.attempt, result, tt.expected)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
		desc       string
	}{
		// Retryable errors
		{http.StatusTooManyRequests, true, "429 rate limit"},
		{http.StatusInternalServerError, true, "500 server error"},
		{http.StatusBadGateway, true, "502 bad gateway"},
		{http.StatusServiceUnavailable, true, "503 service unavailable"},
		{http.StatusGatewayTimeout, true, "504 gateway timeout"},
		{599, true, "599 (edge case 5xx)"},

		// Non-retryable errors
		{http.StatusBadRequest, false, "400 bad request"},
		{http.StatusUnauthorized, false, "401 unauthorized"},
		{http.StatusForbidden, false, "403 forbidden"},
		{http.StatusNotFound, false, "404 not found"},
		{http.StatusMethodNotAllowed, false, "405 method not allowed"},
		{http.StatusConflict, false, "409 conflict"},
		{http.StatusGone, false, "410 gone"},
		{http.StatusUnprocessableEntity, false, "422 unprocessable entity"},

		// Success codes (should not be retryable)
		{http.StatusOK, false, "200 OK"},
		{http.StatusCreated, false, "201 created"},
		{http.StatusNoContent, false, "204 no content"},

		// Redirect codes (should not be retryable)
		{http.StatusMovedPermanently, false, "301 moved permanently"},
		{http.StatusFound, false, "302 found"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := IsRetryable(tt.statusCode)
			if result != tt.expected {
				t.Errorf("IsRetryable(%d) = %v, expected %v", tt.statusCode, result, tt.expected)
			}
		})
	}
}

func TestNewRetryableError(t *testing.T) {
	tests := []struct {
		statusCode int
		message    string
		retryable  bool
	}{
		{429, "rate limited", true},
		{500, "server error", true},
		{400, "bad request", false},
		{401, "unauthorized", false},
	}

	for _, tt := range tests {
		err := NewRetryableError(tt.statusCode, tt.message)

		if err.StatusCode != tt.statusCode {
			t.Errorf("expected StatusCode %d, got %d", tt.statusCode, err.StatusCode)
		}
		if err.Message != tt.message {
			t.Errorf("expected Message %q, got %q", tt.message, err.Message)
		}
		if err.Retryable != tt.retryable {
			t.Errorf("expected Retryable %v for status %d, got %v", tt.retryable, tt.statusCode, err.Retryable)
		}
		if err.Error() != tt.message {
			t.Errorf("expected Error() %q, got %q", tt.message, err.Error())
		}
	}
}

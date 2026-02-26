package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestShowTraceActionCardUIError_PreservesOriginalError(t *testing.T) {
	originalRunner := runTraceActionCardUIRunner
	defer func() { runTraceActionCardUIRunner = originalRunner }()

	runTraceActionCardUIRunner = func(title, why, action string) error {
		return nil
	}

	baseErr := errors.New("base trace failure")
	err := showTraceActionCardUIError(baseErr, "t", "w", "a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "base trace failure") {
		t.Fatalf("expected original error message, got %q", err.Error())
	}
}

func TestShowTraceActionCardUIError_IncludesCardRenderFailure(t *testing.T) {
	originalRunner := runTraceActionCardUIRunner
	defer func() { runTraceActionCardUIRunner = originalRunner }()

	runTraceActionCardUIRunner = func(title, why, action string) error {
		return errors.New("ui render failed")
	}

	baseErr := errors.New("base trace failure")
	err := showTraceActionCardUIError(baseErr, "t", "w", "a")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "base trace failure") {
		t.Fatalf("missing original error in %q", err.Error())
	}
	if !strings.Contains(err.Error(), "ui render failed") {
		t.Fatalf("missing UI render error in %q", err.Error())
	}
}

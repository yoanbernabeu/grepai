package cli

import (
	"strings"
	"testing"
)

func TestTraceUIFlagsMutuallyExclusiveWithJSON(t *testing.T) {
	root := GetRootCmd()
	root.SetArgs([]string{"trace", "callers", "Login", "--ui", "--json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected mutually exclusive flags error, got nil")
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "none of the others can be") && !strings.Contains(lower, "group") {
		t.Fatalf("unexpected error: %v", err)
	}
}

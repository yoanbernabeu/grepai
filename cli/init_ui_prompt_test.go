package cli

import "testing"

func TestShouldPromptInheritChoice(t *testing.T) {
	if shouldPromptInheritChoice(false, false, true) {
		t.Fatal("UI mode should skip legacy inherit prompt")
	}
	if shouldPromptInheritChoice(false, true, false) {
		t.Fatal("non-interactive mode should skip inherit prompt")
	}
	if shouldPromptInheritChoice(true, false, false) {
		t.Fatal("already-selected inherit should skip prompt")
	}
	if !shouldPromptInheritChoice(false, false, false) {
		t.Fatal("interactive non-UI mode should prompt for inherit choice")
	}
}

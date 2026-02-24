package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/trace"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func hasLineContainingBoth(s, a, b string) bool {
	for _, line := range strings.Split(stripANSI(s), "\n") {
		if strings.Contains(line, a) && strings.Contains(line, b) {
			return true
		}
	}
	return false
}

func maxLineLen(s string) int {
	maxLen := 0
	for _, line := range strings.Split(stripANSI(s), "\n") {
		if width := lipgloss.Width(line); width > maxLen {
			maxLen = width
		}
	}
	return maxLen
}

func TestTraceUIFallsBackToSingleColumnOnNarrowWidth(t *testing.T) {
	result := trace.TraceResult{
		Query: "Login",
		Mode:  "fast",
		Symbol: &trace.Symbol{
			Name:     "Login",
			Kind:     trace.KindFunction,
			File:     "auth.go",
			Line:     12,
			Language: "go",
		},
		Callers: []trace.CallerInfo{
			{
				Symbol: trace.Symbol{
					Name:     "HandleRequest",
					Kind:     trace.KindFunction,
					File:     "handler.go",
					Line:     33,
					Language: "go",
				},
				CallSite: trace.CallSite{
					File:    "handler.go",
					Line:    55,
					Context: "Login()",
				},
			},
		},
	}
	m := newTraceUIModel(result, traceViewCallers)
	m.width = 50
	m.height = 24

	out := m.View()
	if hasLineContainingBoth(out, "Symbols/Nodes", "Detail") {
		t.Fatalf("narrow trace UI should stack panels vertically: %q", out)
	}
}

func TestWorkspaceStatusUIFallsBackToSingleColumnOnNarrowWidth(t *testing.T) {
	cfg := config.DefaultWorkspaceConfig()
	ws, err := buildWorkspaceFromFlags("demo", "qdrant", "ollama", "", "", "", "", 0, "", true)
	if err != nil {
		t.Fatalf("buildWorkspaceFromFlags() failed: %v", err)
	}
	if err := cfg.AddWorkspace(*ws); err != nil {
		t.Fatalf("AddWorkspace() failed: %v", err)
	}

	m := newWorkspaceStatusModel(cfg, "")
	m.width = 54
	m.height = 20

	out := m.View()
	if hasLineContainingBoth(out, "Workspaces", "Workspace Detail") {
		t.Fatalf("narrow workspace status UI should stack panels vertically: %q", out)
	}
}

func TestWorkspaceStatusEmptyStateRespectsViewportWidth(t *testing.T) {
	m := workspaceStatusModel{
		theme:      newTUITheme(),
		width:      36,
		height:     12,
		workspaces: nil,
	}

	out := m.View()
	if got := maxLineLen(out); got > 40 {
		t.Fatalf("empty-state card too wide for viewport: max line %d, output=%q", got, out)
	}
}

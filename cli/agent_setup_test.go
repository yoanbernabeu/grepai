package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSubagent(t *testing.T) {
	tmpDir := t.TempDir()

	// Test creating subagent
	err := createSubagent(tmpDir)
	if err != nil {
		t.Fatalf("failed to create subagent: %v", err)
	}

	// Verify file exists
	subagentPath := filepath.Join(tmpDir, ".claude", "agents", "deep-explore.md")
	if _, err := os.Stat(subagentPath); os.IsNotExist(err) {
		t.Fatal("subagent file was not created")
	}

	// Verify content contains marker
	content, err := os.ReadFile(subagentPath)
	if err != nil {
		t.Fatalf("failed to read subagent file: %v", err)
	}

	if !strings.Contains(string(content), subagentMarker) {
		t.Error("subagent file does not contain expected marker")
	}

	if !strings.Contains(string(content), "grepai search") {
		t.Error("subagent file does not contain grepai search instructions")
	}

	if !strings.Contains(string(content), "grepai trace") {
		t.Error("subagent file does not contain grepai trace instructions")
	}
}

func TestCreateSubagentIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subagent twice
	err := createSubagent(tmpDir)
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}

	err = createSubagent(tmpDir)
	if err != nil {
		t.Fatalf("second creation failed: %v", err)
	}

	// Should still only have one file with expected content
	subagentPath := filepath.Join(tmpDir, ".claude", "agents", "deep-explore.md")
	content, err := os.ReadFile(subagentPath)
	if err != nil {
		t.Fatalf("failed to read subagent file: %v", err)
	}

	// Count occurrences of marker to ensure no duplication
	count := strings.Count(string(content), subagentMarker)
	if count != 1 {
		t.Errorf("expected 1 occurrence of marker, got %d", count)
	}
}

func TestCreateSubagentDirectoryStructure(t *testing.T) {
	tmpDir := t.TempDir()

	// Ensure .claude/agents/ directory is created
	err := createSubagent(tmpDir)
	if err != nil {
		t.Fatalf("failed to create subagent: %v", err)
	}

	agentsDir := filepath.Join(tmpDir, ".claude", "agents")
	info, err := os.Stat(agentsDir)
	if os.IsNotExist(err) {
		t.Fatal(".claude/agents directory was not created")
	}

	if !info.IsDir() {
		t.Fatal(".claude/agents is not a directory")
	}
}

func TestCreateSubagentTemplateContent(t *testing.T) {
	tmpDir := t.TempDir()

	err := createSubagent(tmpDir)
	if err != nil {
		t.Fatalf("failed to create subagent: %v", err)
	}

	subagentPath := filepath.Join(tmpDir, ".claude", "agents", "deep-explore.md")
	content, err := os.ReadFile(subagentPath)
	if err != nil {
		t.Fatalf("failed to read subagent file: %v", err)
	}

	contentStr := string(content)

	// Verify YAML frontmatter
	if !strings.Contains(contentStr, "name: deep-explore") {
		t.Error("missing name in frontmatter")
	}
	if !strings.Contains(contentStr, "description:") {
		t.Error("missing description in frontmatter")
	}
	if !strings.Contains(contentStr, "tools: Read, Grep, Glob, Bash") {
		t.Error("missing or incorrect tools in frontmatter")
	}
	if !strings.Contains(contentStr, "model: inherit") {
		t.Error("missing or incorrect model in frontmatter")
	}

	// Verify instructions content
	if !strings.Contains(contentStr, "Semantic Search") {
		t.Error("missing Semantic Search section")
	}
	if !strings.Contains(contentStr, "Call Graph Tracing") {
		t.Error("missing Call Graph Tracing section")
	}
	if !strings.Contains(contentStr, "Workflow") {
		t.Error("missing Workflow section")
	}
}

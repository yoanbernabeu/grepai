package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentSetupCursorRules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .cursor directory and rules file
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("failed to create .cursor directory: %v", err)
	}

	rulesPath := filepath.Join(cursorDir, "rules")
	initialContent := "# Cursor Rules\n"
	if err := os.WriteFile(rulesPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create rules file: %v", err)
	}

	// Test that file is detected and can be configured
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("failed to read rules file: %v", err)
	}

	// Verify the file doesn't contain marker initially
	if strings.Contains(string(content), agentMarker) {
		t.Error("rules file should not contain marker initially")
	}

	// Simulate adding instructions (as runAgentSetup would do)
	f, err := os.OpenFile(rulesPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open rules file: %v", err)
	}
	if _, err := f.WriteString("\n" + agentInstructions); err != nil {
		f.Close()
		t.Fatalf("failed to write instructions: %v", err)
	}
	f.Close()

	// Verify instructions were added
	content, err = os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("failed to read rules file after update: %v", err)
	}

	if !strings.Contains(string(content), agentMarker) {
		t.Error("rules file should contain grepai marker after update")
	}

	if !strings.Contains(string(content), "grepai search") {
		t.Error("rules file should contain grepai search instructions")
	}
}

func TestAgentSetupCursorRulesIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .cursor directory and rules file with existing instructions
	cursorDir := filepath.Join(tmpDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0755); err != nil {
		t.Fatalf("failed to create .cursor directory: %v", err)
	}

	rulesPath := filepath.Join(cursorDir, "rules")
	contentWithMarker := "# Cursor Rules\n\n" + agentInstructions
	if err := os.WriteFile(rulesPath, []byte(contentWithMarker), 0644); err != nil {
		t.Fatalf("failed to create rules file: %v", err)
	}

	// Verify the marker exists
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("failed to read rules file: %v", err)
	}

	// Count occurrences of marker
	count := strings.Count(string(content), agentMarker)
	if count != 1 {
		t.Errorf("expected 1 occurrence of marker, got %d", count)
	}

	// Simulating idempotence check (as runAgentSetup would do)
	if strings.Contains(string(content), agentMarker) {
		// Should skip - this is the expected behavior
		t.Log("File already configured, would skip (correct behavior)")
	}
}

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

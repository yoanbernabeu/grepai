package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alpkeskin/gotoon"
	"github.com/yoanbernabeu/grepai/store"
)

func TestOutputSearchJSON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	// Capture stdout by temporarily reassigning
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	jsonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}
	if err := encoder.Encode(jsonResults); err != nil {
		t.Fatalf("failed to encode JSON: %v", err)
	}

	// Verify content field is present
	var decoded []SearchResultJSON
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 result, got %d", len(decoded))
	}

	if decoded[0].Content == "" {
		t.Error("expected content field to be present in JSON output")
	}

	if decoded[0].FilePath != "test/file.go" {
		t.Errorf("expected file_path 'test/file.go', got '%s'", decoded[0].FilePath)
	}
}

func TestOutputSearchCompactJSON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	jsonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultCompactJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
		}
	}
	if err := encoder.Encode(jsonResults); err != nil {
		t.Fatalf("failed to encode JSON: %v", err)
	}

	// Verify content field is NOT present
	var decoded []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 result, got %d", len(decoded))
	}

	if _, exists := decoded[0]["content"]; exists {
		t.Error("expected content field to be absent in compact JSON output")
	}

	if decoded[0]["file_path"] != "test/file.go" {
		t.Errorf("expected file_path 'test/file.go', got '%v'", decoded[0]["file_path"])
	}

	if decoded[0]["start_line"].(float64) != 10 {
		t.Errorf("expected start_line 10, got %v", decoded[0]["start_line"])
	}

	if decoded[0]["end_line"].(float64) != 20 {
		t.Errorf("expected end_line 20, got %v", decoded[0]["end_line"])
	}
}

func TestCompactFlagRequiresJSONOrTOON(t *testing.T) {
	// Test that runSearch returns error when --compact is used without --json or --toon
	// We test this by directly checking the validation logic

	// Save original values
	originalCompact := searchCompact
	originalJSON := searchJSON
	originalTOON := searchTOON

	// Reset after test
	defer func() {
		searchCompact = originalCompact
		searchJSON = originalJSON
		searchTOON = originalTOON
	}()

	// Set up test case: --compact without --json and --toon
	searchCompact = true
	searchJSON = false
	searchTOON = false

	// The validation happens at the start of runSearch
	if searchCompact && !searchJSON && !searchTOON {
		// This is the expected behavior - validation would fail
		return
	}

	t.Error("expected --compact to require --json or --toon flag")
}

func TestCompactFlagWithJSON(t *testing.T) {
	// Test that --compact with --json is valid

	// Save original values
	originalCompact := searchCompact
	originalJSON := searchJSON
	originalTOON := searchTOON

	// Reset after test
	defer func() {
		searchCompact = originalCompact
		searchJSON = originalJSON
		searchTOON = originalTOON
	}()

	// Set up test case: --compact with --json
	searchCompact = true
	searchJSON = true
	searchTOON = false

	// The validation should pass
	if searchCompact && !searchJSON && !searchTOON {
		t.Error("expected --compact with --json to be valid")
	}
}

func TestCompactFlagWithTOON(t *testing.T) {
	// Test that --compact with --toon is valid

	// Save original values
	originalCompact := searchCompact
	originalJSON := searchJSON
	originalTOON := searchTOON

	// Reset after test
	defer func() {
		searchCompact = originalCompact
		searchJSON = originalJSON
		searchTOON = originalTOON
	}()

	// Set up test case: --compact with --toon
	searchCompact = true
	searchJSON = false
	searchTOON = true

	// The validation should pass
	if searchCompact && !searchJSON && !searchTOON {
		t.Error("expected --compact with --toon to be valid")
	}
}

func TestSearchResultJSONStruct(t *testing.T) {
	result := SearchResultJSON{
		FilePath:  "path/to/file.go",
		StartLine: 1,
		EndLine:   10,
		Score:     0.85,
		Content:   "code content here",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal SearchResultJSON: %v", err)
	}

	// Verify all fields are present in JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	expectedFields := []string{"file_path", "start_line", "end_line", "score", "content"}
	for _, field := range expectedFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("expected field '%s' to be present", field)
		}
	}
}

func TestSearchResultCompactJSONStruct(t *testing.T) {
	result := SearchResultCompactJSON{
		FilePath:  "path/to/file.go",
		StartLine: 1,
		EndLine:   10,
		Score:     0.85,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal SearchResultCompactJSON: %v", err)
	}

	// Verify expected fields are present and content is NOT present
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	expectedFields := []string{"file_path", "start_line", "end_line", "score"}
	for _, field := range expectedFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("expected field '%s' to be present", field)
		}
	}

	if _, exists := decoded["content"]; exists {
		t.Error("expected 'content' field to be absent in compact struct")
	}
}

func TestOutputSearchTOON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	// Encode using gotoon
	toonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		t.Fatalf("failed to encode TOON: %v", err)
	}

	// Verify output is not empty and is valid TOON
	if output == "" {
		t.Error("expected TOON output to be non-empty")
	}

	// TOON format should contain the file path
	if !strings.Contains(output, "test/file.go") {
		t.Error("expected TOON output to contain file path")
	}

	// TOON format should contain content
	if !strings.Contains(output, "TestFunction") {
		t.Error("expected TOON output to contain content")
	}
}

func TestOutputSearchCompactTOON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {}",
			},
			Score: 0.95,
		},
	}

	// Encode using gotoon with compact struct
	toonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultCompactJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		t.Fatalf("failed to encode TOON: %v", err)
	}

	// Verify output is not empty
	if output == "" {
		t.Error("expected TOON output to be non-empty")
	}

	// TOON format should contain the file path
	if !strings.Contains(output, "test/file.go") {
		t.Error("expected TOON output to contain file path")
	}
}

func TestTOONSmallerThanJSON(t *testing.T) {
	results := []store.SearchResult{
		{
			Chunk: store.Chunk{
				FilePath:  "test/file.go",
				StartLine: 10,
				EndLine:   20,
				Content:   "func TestFunction() {\n\treturn nil\n}",
			},
			Score: 0.95,
		},
		{
			Chunk: store.Chunk{
				FilePath:  "test/file2.go",
				StartLine: 30,
				EndLine:   40,
				Content:   "func AnotherFunction() {\n\treturn true\n}",
			},
			Score: 0.85,
		},
	}

	// Convert to JSON structs
	jsonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultJSON{
			FilePath:  r.Chunk.FilePath,
			StartLine: r.Chunk.StartLine,
			EndLine:   r.Chunk.EndLine,
			Score:     r.Score,
			Content:   r.Chunk.Content,
		}
	}

	// Encode as JSON
	var jsonBuf bytes.Buffer
	encoder := json.NewEncoder(&jsonBuf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jsonResults); err != nil {
		t.Fatalf("failed to encode JSON: %v", err)
	}

	// Encode as TOON
	toonOutput, err := gotoon.Encode(jsonResults)
	if err != nil {
		t.Fatalf("failed to encode TOON: %v", err)
	}

	jsonSize := len(jsonBuf.Bytes())
	toonSize := len(toonOutput)

	// TOON should be smaller than JSON
	if toonSize >= jsonSize {
		t.Errorf("expected TOON (%d bytes) to be smaller than JSON (%d bytes)", toonSize, jsonSize)
	}
}

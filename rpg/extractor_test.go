package rpg

import "testing"

func TestLocalExtractor_ExtractFeature(t *testing.T) {
	ext := NewLocalExtractor()

	tests := []struct {
		name       string
		symbolName string
		signature  string
		receiver   string
		comment    string
		expected   string
	}{
		{
			name:       "simple verb-object",
			symbolName: "HandleRequest",
			expected:   "handle-request",
		},
		{
			name:       "camelCase with verb",
			symbolName: "validateToken",
			expected:   "validate-token",
		},
		{
			name:       "multi-word object",
			symbolName: "GetUserByID",
			expected:   "get-user-by-id",
		},
		{
			name:       "single non-verb word",
			symbolName: "Server",
			expected:   "operate-server",
		},
		{
			name:       "verb inside compound",
			symbolName: "TokenValidator",
			expected:   "operate-token-validator",
		},
		{
			name:       "empty string",
			symbolName: "",
			expected:   "unknown",
		},
		{
			name:       "with receiver",
			symbolName: "Save",
			receiver:   "Config",
			expected:   "save@config",
		},
		{
			name:       "with pointer receiver",
			symbolName: "Load",
			receiver:   "*Database",
			expected:   "load@database",
		},
		{
			name:       "verb-object with receiver",
			symbolName: "HandleRequest",
			receiver:   "Server",
			expected:   "handle-request@server",
		},
		{
			name:       "long name capped at 4 words",
			symbolName: "GetUserAccountInformationDetails",
			expected:   "get-user-account-information",
		},
		{
			name:       "snake_case",
			symbolName: "parse_config_file",
			expected:   "parse-config-file",
		},
		{
			name:       "ACRONYM handling",
			symbolName: "HTTPServer",
			expected:   "operate-http-server",
		},
		{
			name:       "getHTTPResponse",
			symbolName: "getHTTPResponse",
			expected:   "get-http-response",
		},
		{
			name:       "single acronym",
			symbolName: "ID",
			expected:   "operate-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ext.ExtractFeature(tt.symbolName, tt.signature, tt.receiver, tt.comment)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSplitName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "camelCase",
			input:    "handleRequest",
			expected: []string{"handle", "Request"},
		},
		{
			name:     "PascalCase",
			input:    "HandleRequest",
			expected: []string{"Handle", "Request"},
		},
		{
			name:     "snake_case",
			input:    "get_user_id",
			expected: []string{"get", "user", "id"},
		},
		{
			name:     "ACRONYM only",
			input:    "ID",
			expected: []string{"ID"},
		},
		{
			name:     "ACRONYM followed by word",
			input:    "HTTPServer",
			expected: []string{"HTTP", "Server"},
		},
		{
			name:     "word with ACRONYM in middle",
			input:    "getHTTPResponse",
			expected: []string{"get", "HTTP", "Response"},
		},
		{
			name:     "multiple ACRONYMs",
			input:    "HTTPAPI",
			expected: []string{"HTTPAPI"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single lowercase word",
			input:    "handler",
			expected: []string{"handler"},
		},
		{
			name:     "single uppercase word",
			input:    "HANDLER",
			expected: []string{"HANDLER"},
		},
		{
			name:     "mixed separators",
			input:    "get_HTTP-Response",
			expected: []string{"get", "HTTP", "Response"},
		},
		{
			name:     "with numbers",
			input:    "Item2String",
			expected: []string{"Item2", "String"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitName(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d words, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Word %d: expected %s, got %s", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestIsVerb(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"get", true},
		{"handle", true},
		{"validate", true},
		{"server", false},
		{"config", false},
		{"", false},
		{"GET", true}, // should be case-insensitive
		{"Handle", true},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := isVerb(tt.word)
			if result != tt.expected {
				t.Errorf("isVerb(%s): expected %v, got %v", tt.word, tt.expected, result)
			}
		})
	}
}

func TestLocalExtractor_Mode(t *testing.T) {
	ext := NewLocalExtractor()
	if ext.Mode() != "local" {
		t.Errorf("Expected mode 'local', got '%s'", ext.Mode())
	}
}

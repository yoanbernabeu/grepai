package rpg

import (
	"reflect"
	"testing"
)

func TestParseAtomicFeatureResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "valid JSON array",
			input:    `["validate token", "load config"]`,
			expected: []string{"validate token", "load config"},
		},
		{
			name:     "JSON with markdown fences",
			input:    "```json\n[\"validate token\", \"load config\"]\n```",
			expected: []string{"validate token", "load config"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: nil,
		},
		{
			name:  "comma-separated fallback",
			input: "validate token, load config",
			// fallback splits on comma: ["validate token", " load config"] -> normalized
			expected: []string{"validate token", "load config"},
		},
		{
			name:     "newline-separated with bullet prefixes",
			input:    "- validate token\n- load config",
			expected: []string{"validate token", "load config"},
		},
		{
			name:  "malformed JSON falls back to regex split",
			input: `["validate token",`,
			// not valid JSON; fallback splits on nothing useful, whole string treated as one item
			// The regex splits on [\n,;]+ so it splits on the comma:
			// parts: ["[\"validate token\"", ""] -> trim space + trim leading "-"
			// "[\"validate token\"" -> normalizeAtomicFeature -> strips quotes -> "validate token"
			// "" -> skipped
			expected: []string{"validate token"},
		},
		{
			name:     "more than 5 items capped at 5",
			input:    `["a feature", "b feature", "c feature", "d feature", "e feature", "f feature"]`,
			expected: []string{"a feature", "b feature", "c feature", "d feature", "e feature"},
		},
		{
			name:     "duplicates deduplicated",
			input:    `["validate token", "Validate Token", "validate token"]`,
			expected: []string{"validate token"},
		},
		{
			name:     "single string not array — newline/comma-free",
			input:    "validate token",
			expected: []string{"validate token"},
		},
		{
			name:     "semicolon-separated fallback",
			input:    "validate token; load config; parse request",
			expected: []string{"validate token", "load config", "parse request"},
		},
		{
			name:     "mixed newline and comma separators",
			input:    "validate token\nload config, parse request",
			expected: []string{"validate token", "load config", "parse request"},
		},
		{
			name:     "markdown fence without json tag",
			input:    "```\n[\"validate token\", \"load config\"]\n```",
			expected: []string{"validate token", "load config"},
		},
		{
			name:  "JSON array with extra whitespace in values",
			input: `["  validate token  ", "load config"]`,
			// normalizeAtomicFeature trims the strings
			expected: []string{"validate token", "load config"},
		},
		{
			name:  "markdown fence with only 2 lines — not stripped",
			input: "```\n[\"validate token\"]",
			// len(lines) == 2, fence stripping skipped; raw stays as is
			// json.Unmarshal fails on "```\n[...]"; fallback splits on newline
			// parts: ["```", "[\"validate token\"]"]
			// normalize "```" -> strips backticks -> "" -> skipped
			// normalize "[\"validate token\"]" -> strips brackets and quotes -> "validate token"
			expected: []string{"validate token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAtomicFeatureResponse(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseAtomicFeatureResponse(%q)\n  got  %v\n  want %v", tt.input, got, tt.expected)
			}
		})
	}
}

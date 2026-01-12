package store

import (
	"strings"
	"testing"
)

// TestBuildEnsureVectorSQL_ContainsExpectedFragments verifies that the generated SQL
// targets the expected table/column and embeds the requested dimension.
func TestBuildEnsureVectorSQL_ContainsExpectedFragments(t *testing.T) {
	tests := []struct {
		name string
		dim  int
	}{
		{name: "768", dim: 768},
		{name: "1536", dim: 1536},
		{name: "3072", dim: 3072},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql := buildEnsureVectorSQL(tt.dim)

			// Quick sanity: ensure formatting didn't produce fmt error markers.
			if strings.Contains(sql, "%!") {
				t.Fatalf("generated SQL contains fmt error marker: %q", sql)
			}

			expected := []string{
				"DO $$",
				"FROM pg_attribute",
				"attrelid = 'chunks'::regclass",
				"attname = 'vector'",
				"IS DISTINCT FROM",
				"ALTER TABLE chunks ALTER COLUMN vector TYPE vector(",
			}

			for _, frag := range expected {
				if !strings.Contains(sql, frag) {
					t.Fatalf("expected SQL to contain %q, got: %q", frag, sql)
				}
			}

			if !strings.Contains(sql, "vector("+strconvItoa(tt.dim)+")") {
				t.Fatalf("expected SQL to contain vector(%d), got: %q", tt.dim, sql)
			}
		})
	}
}

// strconvItoa converts an int to string without importing strconv.
// This keeps the test dependency surface minimal.
func strconvItoa(n int) string {
	// Fast path for common dimensions used in this repo.
	switch n {
	case 768:
		return "768"
	case 1536:
		return "1536"
	case 3072:
		return "3072"
	default:
		// Fallback: simple base-10 conversion.
		if n == 0 {
			return "0"
		}
		neg := n < 0
		if neg {
			n = -n
		}
		var buf [32]byte
		i := len(buf)
		for n > 0 {
			i--
			buf[i] = byte('0' + n%10)
			n /= 10
		}
		if neg {
			i--
			buf[i] = '-'
		}
		return string(buf[i:])
	}
}

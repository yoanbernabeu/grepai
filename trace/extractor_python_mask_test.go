package trace

import (
	"context"
	"testing"
)

// Python comments start with "#" and may contain apostrophes, "//" is floor
// division, and ”' docstrings may contain apostrophes. None of these may
// hide the calls that follow them (issue #280).
func TestExtractReferences_PythonCommentsAndStringsDoNotHideCalls(t *testing.T) {
	content := `def handler(items):
    # don't call this from a hot loop
    first = load_items(items)
    half = len(first) // 2
    second = split_items(first, half)
    """Docstring with "quotes" inside."""
    third = merge_items(second)
    '''It's a docstring with an apostrophe.'''
    return save_items(third)
`
	refs, err := NewRegexExtractor().ExtractReferences(context.Background(), "app/handler.py", content)
	if err != nil {
		t.Fatalf("ExtractReferences: %v", err)
	}

	found := map[string]bool{}
	for _, r := range refs {
		found[r.SymbolName] = true
	}
	for _, want := range []string{"load_items", "split_items", "merge_items", "save_items"} {
		if !found[want] {
			t.Errorf("missing call to %s; got %v", want, found)
		}
	}
	if found["call"] {
		t.Errorf("text inside a comment was extracted as a call: %v", found)
	}
}

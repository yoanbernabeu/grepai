package rpg

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// FeatureExtractor extracts semantic feature labels from code symbols.
type FeatureExtractor interface {
	// ExtractFeature generates a feature label for a symbol.
	// Returns a verb-object string like "handle-request", "validate-token", "parse-config".
	ExtractFeature(ctx context.Context, symbolName, signature, receiver, comment string) string

	// ExtractAtomicFeatures generates one or more atomic semantic features.
	// Returns normalized verb-object phrases like "handle request".
	ExtractAtomicFeatures(ctx context.Context, symbolName, signature, receiver, comment string) []string

	// GenerateSummary generates a high-level summary for a node.
	// Returns the summary string or error.
	GenerateSummary(ctx context.Context, name, contextStr string) (string, error)

	// Mode returns the extractor mode name.
	Mode() string
}

// LocalExtractor generates feature labels using heuristic rules.
// It splits camelCase/PascalCase/snake_case names into verb-object patterns.
type LocalExtractor struct{}

// NewLocalExtractor creates a new local heuristic feature extractor.
func NewLocalExtractor() *LocalExtractor { return &LocalExtractor{} }

// Mode returns the extractor mode name.
func (e *LocalExtractor) Mode() string { return "local" }

// knownVerbs is the set of common verb prefixes recognized by the local extractor.
var knownVerbs = map[string]bool{
	"get": true, "set": true, "new": true, "create": true,
	"delete": true, "remove": true, "update": true, "handle": true,
	"process": true, "validate": true, "parse": true, "format": true,
	"convert": true, "build": true, "init": true, "close": true,
	"open": true, "read": true, "write": true, "send": true,
	"receive": true, "start": true, "stop": true, "run": true,
	"execute": true, "check": true, "is": true, "has": true,
	"can": true, "should": true, "find": true, "search": true,
	"lookup": true, "save": true, "load": true, "persist": true,
	"encode": true, "decode": true, "marshal": true, "unmarshal": true,
	"register": true, "add": true, "make": true, "do": true,
	"list": true, "count": true, "reset": true, "flush": true,
	"sync": true, "fetch": true, "put": true, "patch": true,
	"apply": true, "resolve": true, "notify": true, "emit": true,
	"on": true, "to": true, "from": true, "with": true,
	"ensure": true, "assert": true, "test": true, "bench": true,
	"serve": true, "listen": true, "connect": true, "disconnect": true,
	"subscribe": true, "unsubscribe": true, "publish": true,
	"lock": true, "unlock": true, "wait": true, "signal": true,
	"log": true, "print": true, "render": true, "draw": true,
	"sort": true, "filter": true, "map": true, "reduce": true,
	"merge": true, "split": true, "join": true, "append": true,
	"insert": true, "pop": true, "push": true, "peek": true,
	"scan": true, "walk": true, "visit": true, "traverse": true,
	"compute": true, "calculate": true, "measure": true,
	"wrap": true, "unwrap": true, "extract": true, "inject": true,
	"index": true, "reindex": true, "rebuild": true, "refresh": true,
	"compile": true, "transform": true, "configure": true, "setup": true,
	"teardown": true, "destroy": true, "dispose": true, "release": true,
	"acquire": true, "allocate": true, "free": true,
	"enable": true, "disable": true, "toggle": true,
	"show": true, "hide": true, "expand": true, "collapse": true,
	"match": true, "compare": true, "diff": true, "clone": true,
	"copy": true, "move": true, "rename": true, "swap": true,
	"trim": true, "strip": true, "clean": true, "sanitize": true,
	"normalize": true, "flatten": true, "chunk": true,
	"embed": true, "query": true, "watch": true, "poll": true,
	"dial": true, "accept": true, "bind": true, "attach": true,
	"detach": true, "mount": true, "unmount": true,
}

// ExtractFeature generates a feature label for a symbol using heuristic rules.
// It splits the symbol name into words, identifies a verb-object pattern,
// and returns a lowercase kebab-case string like "handle-request".
func (e *LocalExtractor) ExtractFeature(_ context.Context, symbolName, signature, receiver, comment string) string {
	features := e.ExtractAtomicFeatures(context.Background(), symbolName, signature, receiver, comment)
	if len(features) == 0 {
		return "unknown"
	}
	return primaryFromAtomicFeature(features[0])
}

// ExtractAtomicFeatures generates normalized atomic semantic features.
func (e *LocalExtractor) ExtractAtomicFeatures(_ context.Context, symbolName, signature, receiver, comment string) []string {
	if symbolName == "" {
		return []string{"unknown"}
	}

	words := splitName(symbolName)
	if len(words) == 0 {
		return []string{"unknown"}
	}

	// Lowercase all words for uniform processing.
	lower := make([]string, len(words))
	for i, w := range words {
		lower[i] = strings.ToLower(w)
	}

	// If the first word is a recognized verb, use verb + remaining as object.
	if isVerb(lower[0]) {
		return []string{atomicFromPrimaryFeature(buildLabel(lower))}
	}

	// If the name is a single word that is not a verb, it might be a noun
	// (e.g., a type name like "Server", "Config"). Use "operate" as default verb.
	if len(lower) == 1 {
		return []string{atomicFromPrimaryFeature(buildLabel(append([]string{"operate"}, lower...)))}
	}

	// Multi-word but first word is not a recognized verb.
	// Check if any word in the sequence is a verb (e.g., "TokenValidator" -> "validate-token").
	for i, w := range lower {
		if isVerb(w) {
			// Rearrange: verb first, then remaining words.
			reordered := []string{w}
			reordered = append(reordered, lower[:i]...)
			reordered = append(reordered, lower[i+1:]...)
			return []string{atomicFromPrimaryFeature(buildLabel(reordered))}
		}
	}

	// No recognized verb found at all. Use "operate" as default.
	return []string{atomicFromPrimaryFeature(buildLabel(append([]string{"operate"}, lower...)))}
}

// GenerateSummary builds a deterministic local summary from child feature hints.
func (e *LocalExtractor) GenerateSummary(_ context.Context, name, contextStr string) (string, error) {
	featureHints := make([]string, 0)
	for _, line := range strings.Split(contextStr, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if item == "" {
			continue
		}
		if idx := strings.Index(item, ":"); idx >= 0 {
			item = strings.TrimSpace(item[:idx])
		}
		if normalized := normalizeAtomicFeature(item); normalized != "" {
			featureHints = append(featureHints, normalized)
		}
	}

	top := aggregateAtomicFeatures(featureHints, 3)
	target := strings.TrimSpace(name)
	if target == "" {
		target = "this module"
	}

	if len(top) == 0 {
		return fmt.Sprintf("Provides %s responsibilities.", target), nil
	}
	return fmt.Sprintf("Provides %s responsibilities for %s.", target, strings.Join(top, ", ")), nil
}

// buildLabel constructs a kebab-case feature label from words, capped at 4 words.
func buildLabel(words []string) string {
	// Cap at 4 words for conciseness.
	if len(words) > 4 {
		words = words[:4]
	}

	return strings.Join(words, "-")
}

// splitName splits a symbol name into words, handling camelCase, PascalCase,
// snake_case, and sequences of uppercase characters (acronyms like ID, HTTP, URL).
func splitName(name string) []string {
	// First, replace underscores with a boundary marker and split.
	// Handle snake_case by splitting on underscores.
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	var words []string
	var current []rune

	runes := []rune(name)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Underscore is a word boundary.
		if r == '_' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
			continue
		}

		// Non-letter/digit characters are boundaries.
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
			continue
		}

		if unicode.IsUpper(r) {
			if len(current) == 0 {
				// Starting a new word.
				current = append(current, r)
				continue
			}

			prevIsUpper := unicode.IsUpper(current[len(current)-1])

			if !prevIsUpper {
				// Transition from lowercase to uppercase: new word boundary.
				// e.g., "handle|Request"
				words = append(words, string(current))
				current = []rune{r}
				continue
			}

			// Previous was also uppercase. Check if this is the end of an acronym.
			// An acronym ends when the next character is lowercase.
			// e.g., "HTTP|Server" -> at 'S', previous run is "HTTP", 'S' starts "Server".
			if i+1 < len(runes) && unicode.IsLetter(runes[i+1]) && unicode.IsLower(runes[i+1]) {
				// The current uppercase letter starts a new word.
				// Everything before it in the current run is an acronym.
				if len(current) > 0 {
					words = append(words, string(current))
				}
				current = []rune{r}
				continue
			}

			// Still in an acronym run (e.g., "HTT" in "HTTP").
			current = append(current, r)
		} else {
			// Lowercase or digit: continue current word.
			current = append(current, r)
		}
	}

	if len(current) > 0 {
		words = append(words, string(current))
	}

	return words
}

// isVerb checks if a word is a known verb prefix.
func isVerb(word string) bool {
	return knownVerbs[strings.ToLower(word)]
}

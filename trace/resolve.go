package trace

// symbolLess orders same-name definitions by file, then line. Trace uses it
// whenever it must pick one definition, so the choice never depends on the
// order in which files were indexed (#372, #212).
func symbolLess(a, b Symbol) bool {
	if a.File != b.File {
		return a.File < b.File
	}
	return a.Line < b.Line
}

// firstSymbol returns the definition that sorts first by file, then line.
func firstSymbol(candidates []Symbol) *Symbol {
	if len(candidates) == 0 {
		return nil
	}
	best := 0
	for i := 1; i < len(candidates); i++ {
		if symbolLess(candidates[i], candidates[best]) {
			best = i
		}
	}
	return &candidates[best]
}

// PickTargetSymbol chooses which of several same-name definitions a trace
// result reports as its symbol. It prefers the definition referenced most
// often from its own file, and breaks ties by file then line.
func PickTargetSymbol(candidates []Symbol, refs []Reference) *Symbol {
	if len(candidates) == 0 {
		return nil
	}
	best := 0
	bestScore := -1
	for i, sym := range candidates {
		score := 0
		for _, ref := range refs {
			// Prefer symbols referenced from their own file first (component-local/private usage).
			if ref.File == sym.File || ref.CallerFile == sym.File {
				score++
			}
		}
		if score > bestScore || (score == bestScore && symbolLess(sym, candidates[best])) {
			bestScore = score
			best = i
		}
	}
	return &candidates[best]
}

// PickSymbolForFile chooses the definition declared in preferredFile, or the
// first one by file then line when none is.
func PickSymbolForFile(candidates []Symbol, preferredFile string) *Symbol {
	if len(candidates) == 0 {
		return nil
	}
	best := -1
	if preferredFile != "" {
		for i := range candidates {
			if candidates[i].File == preferredFile && (best < 0 || candidates[i].Line < candidates[best].Line) {
				best = i
			}
		}
	}
	if best >= 0 {
		return &candidates[best]
	}
	return firstSymbol(candidates)
}

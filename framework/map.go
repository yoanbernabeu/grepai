package framework

import "strings"

// RemapLineRange maps a generated line range to source lines.
func RemapLineRange(generatedToSource []int, generatedStart, generatedEnd int) (int, int) {
	if len(generatedToSource) == 0 {
		return generatedStart, generatedEnd
	}
	if generatedStart < 1 {
		generatedStart = 1
	}
	if generatedEnd < generatedStart {
		generatedEnd = generatedStart
	}
	if generatedStart > len(generatedToSource) {
		return generatedStart, generatedEnd
	}
	if generatedEnd > len(generatedToSource) {
		generatedEnd = len(generatedToSource)
	}

	minLine := 0
	maxLine := 0
	for i := generatedStart; i <= generatedEnd; i++ {
		src := generatedToSource[i-1]
		if src <= 0 {
			continue
		}
		if minLine == 0 || src < minLine {
			minLine = src
		}
		if src > maxLine {
			maxLine = src
		}
	}
	if minLine == 0 {
		return generatedStart, generatedEnd
	}
	return minLine, maxLine
}

// RemapLine maps a generated line to source line.
func RemapLine(generatedToSource []int, generatedLine int) int {
	if generatedLine < 1 || generatedLine > len(generatedToSource) {
		return generatedLine
	}
	src := generatedToSource[generatedLine-1]
	if src <= 0 {
		return generatedLine
	}
	return src
}

// SourceSnippet returns an inclusive line-range snippet from source.
func SourceSnippet(source string, startLine, endLine int) string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine
	}
	lines := strings.Split(source, "\n")
	if len(lines) == 0 {
		return ""
	}
	if startLine > len(lines) {
		return ""
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}

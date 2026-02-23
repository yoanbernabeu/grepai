package stats

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ReadAll reads all entries from the NDJSON stats file at statsPath.
// Malformed lines are skipped with a warning to stderr.
// Returns an empty slice (not an error) when the file does not exist.
func ReadAll(statsPath string) ([]Entry, error) {
	f, err := os.Open(statsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stats: open: %w", err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			fmt.Fprintf(os.Stderr, "stats: skipping malformed line %d: %v\n", lineNum, err)
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return entries, fmt.Errorf("stats: read: %w", err)
	}
	return entries, nil
}

// Summarize aggregates entries into a Summary.
// CostSavedUSD is set only for cloud providers.
func Summarize(entries []Entry, provider string) Summary {
	s := Summary{
		ByCommandType: map[string]int{
			Search:       0,
			TraceCallers: 0,
			TraceCallees: 0,
			TraceGraph:   0,
		},
		ByOutputMode: map[string]int{
			Full:    0,
			Compact: 0,
			Toon:    0,
		},
	}

	for _, e := range entries {
		s.TotalQueries++
		s.OutputTokens += e.OutputTokens
		s.GrepTokens += e.GrepTokens
		s.ByCommandType[e.CommandType]++
		s.ByOutputMode[e.OutputMode]++
	}

	s.TokensSaved = s.GrepTokens - s.OutputTokens
	if s.GrepTokens > 0 {
		s.SavingsPct = float64(s.TokensSaved) / float64(s.GrepTokens) * 100
	}

	if IsCloudProvider(provider) {
		saved := float64(s.TokensSaved) / 1_000_000 * CostPerMTokenUSD
		s.CostSavedUSD = &saved
	}

	return s
}

// HistoryByDay groups entries by calendar day (UTC) and returns a slice
// sorted in descending order (most recent first).
func HistoryByDay(entries []Entry) []DaySummary {
	byDate := map[string]*DaySummary{}

	for _, e := range entries {
		day := ""
		if len(e.Timestamp) >= 10 {
			day = e.Timestamp[:10] // "YYYY-MM-DD"
		} else {
			day = "unknown"
		}
		d, ok := byDate[day]
		if !ok {
			d = &DaySummary{Date: day}
			byDate[day] = d
		}
		d.QueryCount++
		d.OutputTokens += e.OutputTokens
		d.GrepTokens += e.GrepTokens
		d.TokensSaved += e.GrepTokens - e.OutputTokens
	}

	days := make([]DaySummary, 0, len(byDate))
	for _, d := range byDate {
		days = append(days, *d)
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Date > days[j].Date
	})
	return days
}

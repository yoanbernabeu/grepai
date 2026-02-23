package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/stats"
)

var (
	statsJSON    bool
	statsHistory bool
	statsLimit   int
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show token savings achieved by using grepai",
	Long: `Display a summary of token savings achieved by grepai compared to
a traditional grep-based workflow.

Every successful search and trace command records an entry locally in
.grepai/stats.json. This command aggregates those entries and shows
how many tokens (and optionally dollars) have been saved.`,
	RunE: runStats,
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().BoolVarP(&statsJSON, "json", "j", false, "Output results in JSON format")
	statsCmd.Flags().BoolVar(&statsHistory, "history", false, "Show per-day history breakdown")
	statsCmd.Flags().IntVarP(&statsLimit, "limit", "l", 30, "Max days shown with --history")
}

func runStats(cmd *cobra.Command, args []string) error {
	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	statsPath := stats.StatsPath(projectRoot)
	entries, err := stats.ReadAll(statsPath)
	if err != nil {
		return fmt.Errorf("failed to read stats: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No stats recorded yet.")
		fmt.Println("Run a search or trace command to start tracking token savings.")
		return nil
	}

	summary := stats.Summarize(entries, cfg.Embedder.Provider)

	if statsJSON {
		return outputStatsJSON(summary, entries)
	}

	return outputStatsHuman(summary, entries, cfg.Embedder.Provider)
}

// outputStatsJSON renders the summary (and optional history) as JSON.
func outputStatsJSON(summary stats.Summary, entries []stats.Entry) error {
	if !statsHistory {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	days := stats.HistoryByDay(entries)
	if statsLimit > 0 && len(days) > statsLimit {
		days = days[:statsLimit]
	}

	out := struct {
		Summary stats.Summary      `json:"summary"`
		History []stats.DaySummary `json:"history"`
	}{
		Summary: summary,
		History: days,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// outputStatsHuman renders the summary using lipgloss styles.
func outputStatsHuman(summary stats.Summary, entries []stats.Entry, provider string) error {
	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(22)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	content := headerStyle.Render("grepai stats — Token Savings Report") + "\n\n"

	content += labelStyle.Render("Total queries") + valueStyle.Render(fmt.Sprintf("%d", summary.TotalQueries)) + "\n"
	content += labelStyle.Render("Tokens (grepai)") + valueStyle.Render(formatInt(summary.OutputTokens)) + "\n"
	content += labelStyle.Render("Tokens (grep est.)") + valueStyle.Render(formatInt(summary.GrepTokens)) + "\n"
	content += labelStyle.Render("Tokens saved") +
		valueStyle.Render(fmt.Sprintf("%s  ▲ %.1f%%", formatInt(summary.TokensSaved), summary.SavingsPct)) + "\n"

	if summary.CostSavedUSD != nil {
		content += labelStyle.Render("Est. cost saved") +
			valueStyle.Render(fmt.Sprintf("$%.4f", *summary.CostSavedUSD)) +
			dimStyle.Render("  (cloud provider)") + "\n"
	}

	// Command breakdown
	content += "\n"
	cmdLine := "By command:  "
	for _, k := range []string{"search", "trace-callers", "trace-callees", "trace-graph"} {
		if v := summary.ByCommandType[k]; v > 0 {
			cmdLine += fmt.Sprintf("%s %d · ", k, v)
		}
	}
	content += dimStyle.Render(trimSuffix(cmdLine, " · ")) + "\n"

	modeLine := "By mode:     "
	for _, k := range []string{"full", "compact", "toon"} {
		if v := summary.ByOutputMode[k]; v > 0 {
			modeLine += fmt.Sprintf("%s %d · ", k, v)
		}
	}
	content += dimStyle.Render(trimSuffix(modeLine, " · ")) + "\n"

	fmt.Println(boxStyle.Render(content))

	if statsHistory {
		printHistoryTable(entries, dimStyle, valueStyle)
	}

	return nil
}

func printHistoryTable(entries []stats.Entry, dimStyle, valueStyle lipgloss.Style) {
	days := stats.HistoryByDay(entries)
	if statsLimit > 0 && len(days) > statsLimit {
		days = days[:statsLimit]
	}

	colDate := lipgloss.NewStyle().Width(14)
	colNum := lipgloss.NewStyle().Width(10)
	colSaved := lipgloss.NewStyle().Width(16)
	colPct := lipgloss.NewStyle().Width(10)

	header := dimStyle.Render(
		colDate.Render("Date") +
			colNum.Render("Queries") +
			colSaved.Render("Tokens saved") +
			colPct.Render("Savings"),
	)
	sep := dimStyle.Render(fmt.Sprintf("%-14s %-10s %-16s %-10s", "──────────────", "─────────", "───────────────", "────────"))
	fmt.Println(header)
	fmt.Println(sep)

	for _, d := range days {
		pct := 0.0
		if d.GrepTokens > 0 {
			pct = float64(d.TokensSaved) / float64(d.GrepTokens) * 100
		}
		row := colDate.Render(d.Date) +
			colNum.Render(fmt.Sprintf("%d", d.QueryCount)) +
			colSaved.Render(formatInt(d.TokensSaved)) +
			colPct.Render(fmt.Sprintf("%.1f%%", pct))
		fmt.Println(valueStyle.Render(row))
	}
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	s := fmt.Sprintf("%d", n)
	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

package stats

import "path/filepath"

// CommandType represents the type of grepai command that was executed.
type CommandType = string

const (
	Search       CommandType = "search"
	TraceCallers CommandType = "trace-callers"
	TraceCallees CommandType = "trace-callees"
	TraceGraph   CommandType = "trace-graph"
)

// OutputMode represents the output format used for the command result.
type OutputMode = string

const (
	Full    OutputMode = "full"
	Compact OutputMode = "compact"
	Toon    OutputMode = "toon"
)

// GrepExpansionFactor is the multiplier applied to result count when estimating
// how many tokens a grep-based workflow would have consumed. A factor of 3
// accounts for grep returning full file sections rather than isolated chunks.
const GrepExpansionFactor = 3

// DefaultChunkTokens is the default chunk size in tokens used for grep estimation.
// Mirrors indexer.DefaultChunkSize.
const DefaultChunkTokens = 512

// CostPerMTokenUSD is the reference cost per million input tokens used for
// estimating USD savings on cloud providers (conservative middle-ground rate).
const CostPerMTokenUSD = 5.00

// MinGrepTokens is the minimum grep-equivalent token estimate when result count
// is zero, to avoid division-by-zero in savings percentage.
const MinGrepTokens = 50

// StatsFileName is the name of the NDJSON stats file inside .grepai/.
const StatsFileName = "stats.json"

// LockFileName is the name of the lock file used for safe concurrent writes.
const LockFileName = "stats.json.lock"

// cloudProviders is the set of provider names that have an associated token cost.
var cloudProviders = map[string]bool{
	"openai":     true,
	"openrouter": true,
	"synthetic":  true,
}

// IsCloudProvider returns true when the given provider name has a token cost.
func IsCloudProvider(provider string) bool {
	return cloudProviders[provider]
}

// GrepEquivalentTokens estimates how many tokens a grep-based workflow would
// have consumed for a given number of results.
func GrepEquivalentTokens(resultCount int) int {
	if resultCount == 0 {
		return MinGrepTokens
	}
	return resultCount * DefaultChunkTokens * GrepExpansionFactor
}

// Entry represents a single recorded command event.
type Entry struct {
	Timestamp    string `json:"timestamp"`    // RFC3339 UTC
	CommandType  string `json:"command_type"` // search | trace-callers | trace-callees | trace-graph
	OutputMode   string `json:"output_mode"`  // full | compact | toon
	ResultCount  int    `json:"result_count"`
	OutputTokens int    `json:"output_tokens"` // estimated tokens in grepai output
	GrepTokens   int    `json:"grep_tokens"`   // estimated tokens for grep equivalent
}

// Summary is the aggregated view of all recorded entries.
type Summary struct {
	TotalQueries  int            `json:"total_queries"`
	OutputTokens  int            `json:"output_tokens"`
	GrepTokens    int            `json:"grep_tokens"`
	TokensSaved   int            `json:"tokens_saved"`
	SavingsPct    float64        `json:"savings_pct"`
	CostSavedUSD  *float64       `json:"cost_saved_usd"`
	ByCommandType map[string]int `json:"by_command_type"`
	ByOutputMode  map[string]int `json:"by_output_mode"`
}

// DaySummary holds per-day aggregated stats for the --history view.
type DaySummary struct {
	Date         string `json:"date"`
	QueryCount   int    `json:"query_count"`
	OutputTokens int    `json:"output_tokens"`
	GrepTokens   int    `json:"grep_tokens"`
	TokensSaved  int    `json:"tokens_saved"`
}

// StatsPath returns the absolute path of the stats NDJSON file.
func StatsPath(projectRoot string) string {
	return filepath.Join(projectRoot, StatsFileName)
}

// LockPath returns the absolute path of the stats lock file.
func LockPath(projectRoot string) string {
	return filepath.Join(projectRoot, LockFileName)
}

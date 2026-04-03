package framework

import "context"

const (
	ModeAuto    = "auto"
	ModeRequire = "require"
	ModeOff     = "off"
)

// ProcessorCapabilities describes current support status for a processor.
type ProcessorCapabilities struct {
	Embedding bool
	Trace     bool
	Compiled  bool
}

// TransformResult is normalized output from framework processors.
type TransformResult struct {
	Processor             string
	FilePath              string
	VirtualPath           string
	Text                  string
	GeneratedToSourceLine []int // 1-indexed generated line -> source line. 0 means unmapped.
	Transformed           bool
	Warnings              []string
}

// FrameworkProcessor transforms framework files into trace/index friendly text.
type FrameworkProcessor interface {
	Name() string
	Supports(filePath string) bool
	Capabilities() ProcessorCapabilities
	TransformForEmbedding(ctx context.Context, filePath, source string) (TransformResult, error)
	TransformForTrace(ctx context.Context, filePath, source string) (TransformResult, error)
}

// RegistryConfig controls processor behavior.
type RegistryConfig struct {
	Enabled      bool
	Mode         string
	NodePath     string
	EnableVue    bool
	EnableSvelte bool
	EnableAstro  bool
	EnableSolid  bool
}

package framework

import "context"

// SolidProcessor is a placeholder for future Solid-specific JSX/TSX transforms.
type SolidProcessor struct{}

func (p *SolidProcessor) Name() string { return "solid" }
func (p *SolidProcessor) Supports(filePath string) bool {
	return false
}
func (p *SolidProcessor) Capabilities() ProcessorCapabilities {
	return ProcessorCapabilities{}
}
func (p *SolidProcessor) TransformForEmbedding(ctx context.Context, filePath, source string) (TransformResult, error) {
	return TransformResult{}, ErrNotImplemented
}
func (p *SolidProcessor) TransformForTrace(ctx context.Context, filePath, source string) (TransformResult, error) {
	return TransformResult{}, ErrNotImplemented
}

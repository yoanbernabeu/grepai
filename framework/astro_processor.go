package framework

import "context"

type AstroProcessor struct{}

func (p *AstroProcessor) Name() string { return "astro" }
func (p *AstroProcessor) Supports(filePath string) bool {
	return hasExt(filePath, ".astro")
}
func (p *AstroProcessor) Capabilities() ProcessorCapabilities {
	return ProcessorCapabilities{}
}
func (p *AstroProcessor) TransformForEmbedding(ctx context.Context, filePath, source string) (TransformResult, error) {
	return TransformResult{}, ErrNotImplemented
}
func (p *AstroProcessor) TransformForTrace(ctx context.Context, filePath, source string) (TransformResult, error) {
	return TransformResult{}, ErrNotImplemented
}

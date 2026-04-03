package framework

import "context"

type SvelteProcessor struct{}

func (p *SvelteProcessor) Name() string { return "svelte" }
func (p *SvelteProcessor) Supports(filePath string) bool {
	return hasExt(filePath, ".svelte")
}
func (p *SvelteProcessor) Capabilities() ProcessorCapabilities {
	return ProcessorCapabilities{}
}
func (p *SvelteProcessor) TransformForEmbedding(ctx context.Context, filePath, source string) (TransformResult, error) {
	return TransformResult{}, ErrNotImplemented
}
func (p *SvelteProcessor) TransformForTrace(ctx context.Context, filePath, source string) (TransformResult, error) {
	return TransformResult{}, ErrNotImplemented
}

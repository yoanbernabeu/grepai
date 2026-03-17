package framework

import (
	"context"
	"errors"
	"fmt"
)

// ProcessorRegistry resolves and executes framework processors.
type ProcessorRegistry struct {
	cfg        RegistryConfig
	processors []FrameworkProcessor
}

func NewProcessorRegistry(cfg RegistryConfig, processors ...FrameworkProcessor) *ProcessorRegistry {
	if cfg.Mode == "" {
		cfg.Mode = ModeAuto
	}
	if cfg.NodePath == "" {
		cfg.NodePath = "node"
	}
	if !cfg.Enabled {
		cfg.Mode = ModeOff
	}
	return &ProcessorRegistry{cfg: cfg, processors: processors}
}

func (r *ProcessorRegistry) TransformForEmbedding(ctx context.Context, filePath, source string) (TransformResult, error) {
	return r.transform(ctx, filePath, source, true)
}

func (r *ProcessorRegistry) TransformForTrace(ctx context.Context, filePath, source string) (TransformResult, error) {
	return r.transform(ctx, filePath, source, false)
}

func (r *ProcessorRegistry) transform(ctx context.Context, filePath, source string, embedding bool) (TransformResult, error) {
	if r.cfg.Mode == ModeOff {
		return passthrough(filePath, source), nil
	}

	p := r.findProcessor(filePath)
	if p == nil {
		return passthrough(filePath, source), nil
	}

	var (
		res TransformResult
		err error
	)
	if embedding {
		res, err = p.TransformForEmbedding(ctx, filePath, source)
	} else {
		res, err = p.TransformForTrace(ctx, filePath, source)
	}
	if err == nil {
		if res.FilePath == "" {
			res.FilePath = filePath
		}
		if res.VirtualPath == "" {
			res.VirtualPath = filePath
		}
		if res.Processor == "" {
			res.Processor = p.Name()
		}
		if res.Text == "" {
			res.Text = source
		}
		return res, nil
	}

	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotImplemented) {
		if r.cfg.Mode == ModeRequire {
			return TransformResult{}, fmt.Errorf("%s processor failed for %s: %w", p.Name(), filePath, err)
		}
		if res.Text != "" {
			if len(res.Warnings) == 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s processor fallback: %v", p.Name(), err))
			}
			return res, nil
		}
		out := passthrough(filePath, source)
		out.Warnings = append(out.Warnings, fmt.Sprintf("%s processor fallback: %v", p.Name(), err))
		return out, nil
	}

	return TransformResult{}, err
}

func (r *ProcessorRegistry) findProcessor(filePath string) FrameworkProcessor {
	for _, p := range r.processors {
		if !r.processorEnabled(p.Name()) {
			continue
		}
		if p.Supports(filePath) {
			return p
		}
	}
	return nil
}

func (r *ProcessorRegistry) processorEnabled(name string) bool {
	switch name {
	case "vue":
		return r.cfg.EnableVue
	case "svelte":
		return r.cfg.EnableSvelte
	case "astro":
		return r.cfg.EnableAstro
	case "solid":
		return r.cfg.EnableSolid
	default:
		return true
	}
}

func passthrough(filePath, source string) TransformResult {
	return TransformResult{
		Processor:   "passthrough",
		FilePath:    filePath,
		VirtualPath: filePath,
		Text:        source,
	}
}

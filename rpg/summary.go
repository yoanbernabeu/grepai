package rpg

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
)

// Summarizer handles the generation of semantic summaries for the hierarchy.
type Summarizer struct {
	graph     *Graph
	extractor FeatureExtractor
}

// NewSummarizer creates a new Summarizer.
func NewSummarizer(graph *Graph, extractor FeatureExtractor) *Summarizer {
	return &Summarizer{
		graph:     graph,
		extractor: extractor,
	}
}

// SummarizeHierarchy traverses the hierarchy bottom-up and generates summaries.
// It skips nodes that already have a summary unless force is true.
func (s *Summarizer) SummarizeHierarchy(ctx context.Context, force bool) error {
	// Bottom-up order: Subcategory -> Category -> Area

	// 1. Summarize Subcategories
	if err := s.summarizeNodes(ctx, KindSubcategory, force); err != nil {
		return err
	}

	// 2. Summarize Categories
	if err := s.summarizeNodes(ctx, KindCategory, force); err != nil {
		return err
	}

	// 3. Summarize Areas
	if err := s.summarizeNodes(ctx, KindArea, force); err != nil {
		return err
	}

	return nil
}

func (s *Summarizer) summarizeNodes(ctx context.Context, kind NodeKind, force bool) error {
	nodes := s.graph.GetNodesByKind(kind)
	consecutiveFailures := 0
	const maxConsecutiveFailures = 3

	for _, node := range nodes {
		if node.Summary != "" && !force {
			continue
		}

		summary, err := s.generateNodeSummary(ctx, node)
		if err != nil {
			// Log error but continue? Or fail?
			// For now, let's log and continue, as LLM failures shouldn't block everything.
			// In a real app we'd have better logging.
			log.Printf("Failed to summarize node %s: %v\n", node.ID, err)
			consecutiveFailures++
			if consecutiveFailures >= maxConsecutiveFailures {
				log.Printf("rpg: summarization circuit breaker tripped after %d consecutive failures, skipping remaining nodes", maxConsecutiveFailures)
				break
			}
			continue
		}
		node.Summary = summary
		consecutiveFailures = 0
	}
	return nil
}

func (s *Summarizer) generateNodeSummary(ctx context.Context, node *Node) (string, error) {
	// Collect children info to build context
	children := s.getChildren(node)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Node: %s (%s)\n", node.ID, node.Feature))
	if node.SemanticLabel != "" {
		sb.WriteString(fmt.Sprintf("Label: %s\n", node.SemanticLabel))
	}
	sb.WriteString("Children:\n")

	// Limit children to avoid token limits.
	// Prioritize children with summaries, then features.
	maxChildren := 20
	if len(children) > maxChildren {
		// Simple random sample or first N? First N is fine for now as they are sorted by ID usually.
		children = children[:maxChildren]
	}

	for _, child := range children {
		sb.WriteString(fmt.Sprintf("- %s", child.Feature))
		if child.Summary != "" {
			sb.WriteString(fmt.Sprintf(": %s", child.Summary))
		}
		sb.WriteString("\n")
	}

	return s.extractor.GenerateSummary(ctx, node.Feature, sb.String())
}

func (s *Summarizer) getChildren(node *Node) []*Node {
	var children []*Node
	outgoing := s.graph.GetOutgoing(node.ID)
	for _, e := range outgoing {
		if e.Type == EdgeFeatureParent || e.Type == EdgeContains {
			child := s.graph.GetNode(e.To)
			if child != nil {
				children = append(children, child)
			}
		}
	}

	// Sort for determinism
	sort.Slice(children, func(i, j int) bool {
		return children[i].ID < children[j].ID
	})

	return children
}

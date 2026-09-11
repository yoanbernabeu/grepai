package trace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PostgresSymbolStore) GetCallGraph(ctx context.Context, symbolName string, depth int) (graph *CallGraph, retErr error) {
	graph = &CallGraph{Root: symbolName, Nodes: map[string]Symbol{}, Edges: []CallEdge{}, Depth: depth}
	if depth < 0 {
		return graph, nil
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("failed to begin call graph snapshot transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			retErr = errors.Join(retErr, fmt.Errorf("failed to rollback call graph snapshot transaction: %w", rollbackErr))
		}
	}()
	visited, edgeSeen := map[string]bool{}, map[string]bool{}
	frontier := []string{symbolName}
	// One edge query and one symbol batch query are issued per breadth level. A
	// future optimization may replace the edge loop with a recursive CTE.
	for level := 0; level <= depth && len(frontier) > 0; level++ {
		frontierIDs := make([][]byte, 0, len(frontier))
		for _, name := range frontier {
			if !visited[name] {
				visited[name] = true
				frontierIDs = append(frontierIDs, identityBytes(name))
			}
		}
		if len(frontierIDs) == 0 {
			break
		}
		edges, err := s.graphEdgesForLevel(ctx, tx, frontierIDs, level, symbolName)
		if err != nil {
			return nil, err
		}
		names := append([]string(nil), frontier...)
		for _, candidate := range edges {
			edge := candidate.edge
			names = append(names, edge.Caller, edge.Callee)
		}
		symbols, err := s.lookupSymbolsBatch(ctx, tx, names)
		if err != nil {
			return nil, err
		}
		for _, name := range frontier {
			if syms := symbols[name]; len(syms) > 0 {
				graph.Nodes[name] = syms[0]
			}
		}

		next := []string{}
		for _, candidate := range edges {
			edge := candidate.edge
			if isPostgresDeclarationSelfEdge(edge, symbols[edge.Caller]) {
				continue
			}
			key := edge.Caller + "->" + edge.Callee
			if !edgeSeen[key] {
				graph.Edges = append(graph.Edges, edge)
				edgeSeen[key] = true
			}
			if !visited[edge.Callee] && level < depth && len(symbols[edge.Callee]) == 1 {
				next = append(next, edge.Callee)
			}
			if level == 0 && edge.Callee == symbolName && len(symbols[edge.Caller]) > 0 {
				graph.Nodes[edge.Caller] = symbols[edge.Caller][0]
			}
		}
		frontier = next
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit call graph snapshot transaction: %w", err)
	}
	return graph, nil
}

func (s *PostgresSymbolStore) graphEdgesForLevel(ctx context.Context, q postgresQuerier, frontier [][]byte, level int, root string) ([]callEdgeCandidate, error) {
	rows, err := q.Query(ctx, `SELECT caller,callee,file,line,call_type,ordinal FROM call_edges WHERE project_id=$1 AND (caller=ANY($2::bytea[]) OR ($3=0 AND callee=$4)) ORDER BY caller,callee,file,line,call_type,ordinal`, identityBytes(s.projectID), frontier, level, identityBytes(root))
	if err != nil {
		return nil, fmt.Errorf("failed to query call graph: %w", err)
	}
	defer rows.Close()
	edges := []callEdgeCandidate{}
	for rows.Next() {
		var edge CallEdge
		var ordinal int
		var caller, callee, file []byte
		if err := rows.Scan(&caller, &callee, &file, &edge.Line, &edge.CallType, &ordinal); err != nil {
			return nil, err
		}
		edge.Caller, edge.Callee, edge.File = string(caller), string(callee), string(file)
		edges = append(edges, callEdgeCandidate{edge: edge, ordinal: ordinal})
	}
	return canonicalCallEdgeCandidates(edges), rows.Err()
}

func isPostgresDeclarationSelfEdge(edge CallEdge, symbols []Symbol) bool {
	if edge.Caller != edge.Callee {
		return false
	}
	for _, symbol := range symbols {
		if symbol.File == edge.File && symbol.Line == edge.Line {
			return true
		}
	}
	return false
}

package trace

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const symbolColumns = `name,kind,file,line,end_line,signature,receiver,package_name,exported,language,docstring,feature_path`
const refColumns = `symbol_name,ref_type,file,line,col,context,caller,caller_file,caller_line`

type postgresQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func scanSymbols(rows pgx.Rows) ([]Symbol, error) {
	result := []Symbol{}
	for rows.Next() {
		var sym Symbol
		var name, file []byte
		if err := rows.Scan(&name, &sym.Kind, &file, &sym.Line, &sym.EndLine, &sym.Signature, &sym.Receiver, &sym.Package, &sym.Exported, &sym.Language, &sym.Docstring, &sym.FeaturePath); err != nil {
			return nil, err
		}
		sym.Name, sym.File = string(name), string(file)
		result = append(result, sym)
	}
	return result, rows.Err()
}

func scanRefs(rows pgx.Rows) ([]Reference, error) {
	result := []Reference{}
	for rows.Next() {
		var ref Reference
		var name, file, caller, callerFile []byte
		if err := rows.Scan(&name, &ref.Kind, &file, &ref.Line, &ref.Column, &ref.Context, &caller, &callerFile, &ref.CallerLine); err != nil {
			return nil, err
		}
		ref.SymbolName, ref.File = string(name), string(file)
		ref.CallerName, ref.CallerFile = string(caller), string(callerFile)
		result = append(result, ref)
	}
	return result, rows.Err()
}

func (s *PostgresSymbolStore) LookupSymbol(ctx context.Context, name string) ([]Symbol, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+symbolColumns+` FROM symbols WHERE project_id=$1 AND name=$2 ORDER BY file,line`, identityBytes(s.projectID), identityBytes(name))
	if err != nil {
		return nil, fmt.Errorf("failed to lookup symbol: %w", err)
	}
	defer rows.Close()
	return scanSymbols(rows)
}

func (s *PostgresSymbolStore) LookupSymbolsBatch(ctx context.Context, names []string) (map[string][]Symbol, error) {
	return s.lookupSymbolsBatch(ctx, s.pool, names)
}

func (s *PostgresSymbolStore) lookupSymbolsBatch(ctx context.Context, q postgresQuerier, names []string) (map[string][]Symbol, error) {
	result := make(map[string][]Symbol)
	if len(names) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		unique = append(unique, name)
	}
	identities := make([][]byte, len(unique))
	for i, name := range unique {
		identities[i] = identityBytes(name)
	}
	rows, err := q.Query(ctx, `SELECT `+symbolColumns+` FROM symbols WHERE project_id=$1 AND name=ANY($2::bytea[]) ORDER BY name,file,line`, identityBytes(s.projectID), identities)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup symbols batch: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sym Symbol
		var name, file []byte
		if err := rows.Scan(&name, &sym.Kind, &file, &sym.Line, &sym.EndLine, &sym.Signature, &sym.Receiver, &sym.Package, &sym.Exported, &sym.Language, &sym.Docstring, &sym.FeaturePath); err != nil {
			return nil, fmt.Errorf("failed to scan symbol batch: %w", err)
		}
		sym.Name, sym.File = string(name), string(file)
		result[sym.Name] = append(result[sym.Name], sym)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read symbol batch: %w", err)
	}
	return result, nil
}

func (s *PostgresSymbolStore) lookupRefs(ctx context.Context, q postgresQuerier, symbolName, kind string) ([]Reference, error) {
	query := `SELECT ` + refColumns + ` FROM refs WHERE project_id=$1 AND symbol_name=$2`
	args := []any{identityBytes(s.projectID), identityBytes(symbolName)}
	if kind == RefKindCall {
		query += ` AND (ref_type=$3 OR ref_type='')`
		args = append(args, kind)
	} else {
		query += ` AND ref_type=$3`
		args = append(args, kind)
	}
	query += ` ORDER BY file,line,ordinal`
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup references: %w", err)
	}
	defer rows.Close()
	return scanRefs(rows)
}

func (s *PostgresSymbolStore) LookupCallers(ctx context.Context, symbolName string) ([]Reference, error) {
	return s.lookupRefs(ctx, s.pool, symbolName, RefKindCall)
}
func (s *PostgresSymbolStore) LookupReaders(ctx context.Context, symbolName string) ([]Reference, error) {
	return s.lookupRefs(ctx, s.pool, symbolName, RefKindRead)
}
func (s *PostgresSymbolStore) LookupWriters(ctx context.Context, symbolName string) ([]Reference, error) {
	return s.lookupRefs(ctx, s.pool, symbolName, RefKindWrite)
}

func (s *PostgresSymbolStore) GetSymbolsForFile(ctx context.Context, filePath string) ([]Symbol, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+symbolColumns+` FROM symbols WHERE project_id=$1 AND file=$2 ORDER BY line,name`, identityBytes(s.projectID), identityBytes(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols for file: %w", err)
	}
	defer rows.Close()
	return scanSymbols(rows)
}

func (s *PostgresSymbolStore) GetCallEdges(ctx context.Context) ([]CallEdge, error) {
	rows, err := s.pool.Query(ctx, `SELECT caller,callee,file,line,call_type FROM call_edges WHERE project_id=$1 ORDER BY file,line,ordinal`, identityBytes(s.projectID))
	if err != nil {
		return nil, fmt.Errorf("failed to get call edges: %w", err)
	}
	defer rows.Close()
	edges := []CallEdge{}
	for rows.Next() {
		var edge CallEdge
		var caller, callee, file []byte
		if err := rows.Scan(&caller, &callee, &file, &edge.Line, &edge.CallType); err != nil {
			return nil, err
		}
		edge.Caller, edge.Callee, edge.File = string(caller), string(callee), string(file)
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

// CountSymbols returns the exact number of symbols for this store's project.
func (s *PostgresSymbolStore) CountSymbols(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM symbols WHERE project_id=$1`, identityBytes(s.projectID)).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count symbols: %w", err)
	}
	return total, nil
}

func (s *PostgresSymbolStore) GetStats(ctx context.Context) (*SymbolStats, error) {
	var stats SymbolStats
	projectID := identityBytes(s.projectID)
	// One statement gives one PostgreSQL snapshot. Unactivated projects have
	// epoch LastUpdated; activated empty projects retain their activation time.
	err := s.pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM symbols WHERE project_id=$1),
		(SELECT COUNT(*) FROM refs WHERE project_id=$1),
		(SELECT COUNT(*) FROM symbol_files WHERE project_id=$1),
		COALESCE((SELECT SUM(pg_column_size(t)) FROM symbols t WHERE project_id=$1),0)+
		COALESCE((SELECT SUM(pg_column_size(t)) FROM refs t WHERE project_id=$1),0)+
		COALESCE((SELECT SUM(pg_column_size(t)) FROM call_edges t WHERE project_id=$1),0)+
		COALESCE((SELECT SUM(pg_column_size(t)) FROM symbol_files t WHERE project_id=$1),0),
		COALESCE((SELECT last_mutation_at FROM symbol_migrations WHERE project_id=$1 AND state='completed'),'1970-01-01'::timestamptz)`, projectID).
		Scan(&stats.TotalSymbols, &stats.TotalReferences, &stats.TotalFiles, &stats.IndexSize, &stats.LastUpdated)
	if err != nil {
		return nil, fmt.Errorf("failed to read PostgreSQL symbol statistics: %w", err)
	}
	return &stats, nil
}

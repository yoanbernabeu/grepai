package trace

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var symbolRowColumns = []string{"project_id", "name", "file", "line", "end_line", "kind", "signature", "receiver", "package_name", "exported", "language", "docstring", "feature_path"}
var refRowColumns = []string{"project_id", "symbol_name", "file", "line", "col", "ref_type", "context", "caller", "caller_file", "caller_line", "ordinal"}
var edgeRowColumns = []string{"project_id", "caller", "callee", "file", "line", "call_type", "ordinal"}
var fileRowColumns = []string{"project_id", "path", "content_hash", "extractor_version", "mod_time"}

type postgresSymbolRow struct {
	projectID, name, file                                                []byte
	line, endLine                                                        int
	kind, signature, receiver, packageName, language, docstring, feature string
	exported                                                             bool
}

type postgresRefRow struct {
	projectID, symbolName, file, caller, callerFile []byte
	line, column, callerLine, ordinal               int
	refType, context                                string
}

type postgresEdgeRow struct {
	projectID, caller, callee, file []byte
	line, ordinal                   int
	callType                        string
}

type postgresFileRow struct {
	projectID, path               []byte
	contentHash, extractorVersion string
	modTime                       time.Time
}

type postgresFileRows struct {
	symbols []postgresSymbolRow
	refs    []postgresRefRow
	edges   []postgresEdgeRow
	files   []postgresFileRow
}

type migrationFileRows struct {
	filePath, contentHash, extractorVersion string
	symbols                                 []Symbol
	refs                                    []Reference
	modTime                                 time.Time
}

type postgresCopyFromer interface {
	CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error)
}

func buildPostgresFileRows(projectID, filePath, contentHash, extractorVersion string, symbols []Symbol, refs []Reference, modTime time.Time) postgresFileRows {
	rows := postgresFileRows{symbols: make([]postgresSymbolRow, 0, len(symbols)), refs: make([]postgresRefRow, 0, len(refs)), edges: make([]postgresEdgeRow, 0, len(refs)), files: make([]postgresFileRow, 0, 1)}
	project := identityBytes(projectID)
	for _, symbol := range symbols {
		rows.symbols = append(rows.symbols, postgresSymbolRow{projectID: project, name: identityBytes(symbol.Name), file: identityBytes(symbol.File), line: symbol.Line, endLine: symbol.EndLine, kind: sanUTF8(string(symbol.Kind)), signature: sanUTF8(symbol.Signature), receiver: sanUTF8(symbol.Receiver), packageName: sanUTF8(symbol.Package), exported: symbol.Exported, language: sanUTF8(symbol.Language), docstring: sanUTF8(symbol.Docstring), feature: sanUTF8(symbol.FeaturePath)})
	}
	for ordinal, ref := range refs {
		rows.refs = append(rows.refs, postgresRefRow{projectID: project, symbolName: identityBytes(ref.SymbolName), file: identityBytes(ref.File), line: ref.Line, column: ref.Column, refType: sanUTF8(ref.Kind), context: sanUTF8(ref.Context), caller: identityBytes(ref.CallerName), callerFile: identityBytes(ref.CallerFile), callerLine: ref.CallerLine, ordinal: ordinal})
		if ref.CallerName != "" && ref.CallerName != "<top-level>" {
			rows.edges = append(rows.edges, postgresEdgeRow{projectID: project, caller: identityBytes(ref.CallerName), callee: identityBytes(ref.SymbolName), file: identityBytes(ref.File), line: ref.Line, callType: "direct", ordinal: ordinal})
		}
	}
	rows.files = append(rows.files, postgresFileRow{projectID: project, path: identityBytes(filePath), contentHash: sanUTF8(contentHash), extractorVersion: sanUTF8(extractorVersion), modTime: modTime})
	return rows
}

func appendPostgresRows(dst *postgresFileRows, src postgresFileRows) {
	dst.symbols = append(dst.symbols, src.symbols...)
	dst.refs = append(dst.refs, src.refs...)
	dst.edges = append(dst.edges, src.edges...)
	dst.files = append(dst.files, src.files...)
}

func copyMigrationFileBatch(ctx context.Context, copier postgresCopyFromer, projectID string, files []migrationFileRows) error {
	batch := postgresFileRows{}
	for _, file := range files {
		modTime := file.modTime
		if modTime.IsZero() {
			modTime = time.Now().UTC()
		}
		appendPostgresRows(&batch, buildPostgresFileRows(projectID, file.filePath, file.contentHash, file.extractorVersion, file.symbols, file.refs, modTime))
	}
	return copyPostgresRows(ctx, copier, batch)
}

func copyPostgresRows(ctx context.Context, copier postgresCopyFromer, rows postgresFileRows) error {
	operations := []struct {
		table   string
		columns []string
		values  [][]any
	}{
		{"symbols", symbolRowColumns, symbolValues(rows.symbols)},
		{"refs", refRowColumns, refValues(rows.refs)},
		{"call_edges", edgeRowColumns, edgeValues(rows.edges)},
		{"symbol_files", fileRowColumns, fileValues(rows.files)},
	}
	for _, operation := range operations {
		if len(operation.values) == 0 {
			continue
		}
		if _, err := copier.CopyFrom(ctx, pgx.Identifier{operation.table}, operation.columns, pgx.CopyFromRows(operation.values)); err != nil {
			return fmt.Errorf("failed to insert %s: %w", operation.table, err)
		}
	}
	return nil
}

func symbolValues(rows []postgresSymbolRow) [][]any {
	values := make([][]any, len(rows))
	for i, row := range rows {
		values[i] = []any{row.projectID, row.name, row.file, row.line, row.endLine, row.kind, row.signature, row.receiver, row.packageName, row.exported, row.language, row.docstring, row.feature}
	}
	return values
}

func refValues(rows []postgresRefRow) [][]any {
	values := make([][]any, len(rows))
	for i, row := range rows {
		values[i] = []any{row.projectID, row.symbolName, row.file, row.line, row.column, row.refType, row.context, row.caller, row.callerFile, row.callerLine, row.ordinal}
	}
	return values
}

func edgeValues(rows []postgresEdgeRow) [][]any {
	values := make([][]any, len(rows))
	for i, row := range rows {
		values[i] = []any{row.projectID, row.caller, row.callee, row.file, row.line, row.callType, row.ordinal}
	}
	return values
}

func fileValues(rows []postgresFileRow) [][]any {
	values := make([][]any, len(rows))
	for i, row := range rows {
		values[i] = []any{row.projectID, row.path, row.contentHash, row.extractorVersion, row.modTime}
	}
	return values
}

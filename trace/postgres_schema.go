package trace

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func freshSymbolSchemaQueries() []string {
	return []string{
		`CREATE TABLE symbol_files (project_id BYTEA NOT NULL, path BYTEA NOT NULL, content_hash TEXT NOT NULL DEFAULT '', extractor_version TEXT NOT NULL DEFAULT '', mod_time TIMESTAMPTZ NOT NULL, PRIMARY KEY(project_id, path))`,
		`CREATE TABLE symbols (project_id BYTEA NOT NULL, name BYTEA NOT NULL, file BYTEA NOT NULL, line INTEGER NOT NULL, end_line INTEGER NOT NULL DEFAULT 0, kind TEXT NOT NULL, signature TEXT NOT NULL DEFAULT '', receiver TEXT NOT NULL DEFAULT '', package_name TEXT NOT NULL DEFAULT '', exported BOOLEAN NOT NULL DEFAULT FALSE, language TEXT NOT NULL DEFAULT '', docstring TEXT NOT NULL DEFAULT '', feature_path TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE refs (project_id BYTEA NOT NULL, symbol_name BYTEA NOT NULL, file BYTEA NOT NULL, line INTEGER NOT NULL, col INTEGER NOT NULL DEFAULT 0, ref_type TEXT NOT NULL DEFAULT '', context TEXT NOT NULL DEFAULT '', caller BYTEA NOT NULL DEFAULT ''::bytea, caller_file BYTEA NOT NULL DEFAULT ''::bytea, caller_line INTEGER NOT NULL DEFAULT 0, ordinal INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE call_edges (project_id BYTEA NOT NULL, caller BYTEA NOT NULL, callee BYTEA NOT NULL, file BYTEA NOT NULL, line INTEGER NOT NULL, call_type TEXT NOT NULL DEFAULT '', ordinal INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE symbol_migrations (project_id BYTEA PRIMARY KEY, state TEXT NOT NULL, source_path BYTEA NOT NULL, source_digest BYTEA, source_size BIGINT, started_at TIMESTAMPTZ NOT NULL, completed_at TIMESTAMPTZ, last_mutation_at TIMESTAMPTZ NOT NULL)`,
		`CREATE TABLE symbol_store_meta (key TEXT PRIMARY KEY, value INTEGER NOT NULL)`,
	}
}

var identityColumns = map[string][]string{
	"call_edges":        {"project_id", "caller", "callee", "file"},
	"refs":              {"project_id", "symbol_name", "file", "caller", "caller_file"},
	"symbol_files":      {"project_id", "path"},
	"symbol_migrations": {"project_id", "source_path"},
	"symbols":           {"project_id", "name", "file"},
}

type symbolSchemaExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func executeSymbolSchemaQueries(ctx context.Context, executor symbolSchemaExecutor, queries []string, hook func(int, string) error) error {
	for i, query := range queries {
		if hook != nil {
			if err := hook(i, query); err != nil {
				return fmt.Errorf("injected symbol schema DDL failure: %w", err)
			}
		}
		if _, err := executor.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to execute symbol schema DDL: %w", err)
		}
	}
	return nil
}

func qualifiedSchemaQueries(schema string) []string {
	queries := freshSymbolSchemaQueries()
	for i, table := range reservedSymbolTables {
		queries[i] = qualifyCreatedTable(queries[i], schema, table)
	}
	return queries
}

func qualifyCreatedTable(query, schema, table string) string {
	plain := "CREATE TABLE " + table
	return "CREATE TABLE " + pgx.Identifier{schema, table}.Sanitize() + query[len(plain):]
}

func symbolSchemaPlan(schema string, inv symbolSchemaInventory, ownership symbolSchemaOwnership) []string {
	if ownership == symbolSchemaFresh {
		queries := qualifiedSchemaQueries(schema)
		return append(queries, missingIndexQueries(schema, inv)...)
	}
	var queries []string
	setCallerDefault := false
	setCallerFileDefault := false
	tables := make([]string, 0, len(identityColumns))
	for table := range identityColumns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		columns := identityColumns[table]
		entry, ok := inv.tables[table]
		if !ok {
			continue
		}
		for _, name := range columns {
			for _, column := range entry.columns {
				if column.name == name && column.typeOID == pgtype.TextOID {
					qualified := pgx.Identifier{schema, table}.Sanitize()
					identifier := pgx.Identifier{name}.Sanitize()
					queries = append(queries,
						`ALTER TABLE `+qualified+` ALTER COLUMN `+identifier+` DROP DEFAULT`,
						`ALTER TABLE `+qualified+` ALTER COLUMN `+identifier+` TYPE BYTEA USING convert_to(`+identifier+`, 'UTF8')`)
					setCallerDefault = setCallerDefault || table == "refs" && name == "caller"
					setCallerFileDefault = setCallerFileDefault || table == "refs" && name == "caller_file"
				}
			}
		}
	}
	if setCallerDefault {
		queries = append(queries, `ALTER TABLE `+pgx.Identifier{schema, "refs"}.Sanitize()+` ALTER COLUMN caller SET DEFAULT ''::bytea`)
	}
	if setCallerFileDefault {
		queries = append(queries, `ALTER TABLE `+pgx.Identifier{schema, "refs"}.Sanitize()+` ALTER COLUMN caller_file SET DEFAULT ''::bytea`)
	}
	if table, ok := inv.tables["refs"]; ok && !hasSchemaColumn(table, "ordinal") {
		queries = append(queries, `ALTER TABLE `+pgx.Identifier{schema, "refs"}.Sanitize()+` ADD COLUMN ordinal INTEGER NOT NULL DEFAULT 0`)
	}
	if table, ok := inv.tables["call_edges"]; ok && !hasSchemaColumn(table, "ordinal") {
		queries = append(queries, `ALTER TABLE `+pgx.Identifier{schema, "call_edges"}.Sanitize()+` ADD COLUMN ordinal INTEGER NOT NULL DEFAULT 0`)
	}
	if table, ok := inv.tables["symbol_migrations"]; ok {
		if !hasSchemaColumn(table, "source_digest") {
			queries = append(queries, `ALTER TABLE `+pgx.Identifier{schema, "symbol_migrations"}.Sanitize()+` ADD COLUMN source_digest BYTEA`)
		}
		if !hasSchemaColumn(table, "source_size") {
			queries = append(queries, `ALTER TABLE `+pgx.Identifier{schema, "symbol_migrations"}.Sanitize()+` ADD COLUMN source_size BIGINT`)
		}
		if !hasSchemaColumn(table, "last_mutation_at") {
			migrationTable := pgx.Identifier{schema, "symbol_migrations"}.Sanitize()
			filesTable := pgx.Identifier{schema, "symbol_files"}.Sanitize()
			queries = append(queries,
				`ALTER TABLE `+migrationTable+` ADD COLUMN last_mutation_at TIMESTAMPTZ`,
				`UPDATE `+migrationTable+` m SET last_mutation_at=COALESCE((SELECT MAX(f.mod_time) FROM `+filesTable+` f WHERE f.project_id=m.project_id),m.completed_at,m.started_at,clock_timestamp()) WHERE last_mutation_at IS NULL`,
				`ALTER TABLE `+migrationTable+` ALTER COLUMN last_mutation_at SET NOT NULL`)
		}
	} else {
		queries = append(queries, qualifiedSchemaQueries(schema)[4])
		if ownership == symbolSchemaLegacy {
			queries = append(queries, legacyPostgresActivationBackfillQuery(schema))
		}
	}
	if _, ok := inv.tables["symbol_store_meta"]; !ok {
		queries = append(queries, qualifiedSchemaQueries(schema)[5])
	}
	queries = append(queries, missingIndexQueries(schema, inv)...)
	return queries
}

func symbolMigrationNeedsDDL(inv symbolSchemaInventory) bool {
	table, ok := inv.tables["symbol_migrations"]
	if !ok {
		return false
	}
	for _, name := range []string{"source_digest", "source_size", "last_mutation_at"} {
		if !hasSchemaColumn(table, name) {
			return true
		}
	}
	for _, column := range table.columns {
		if slices.Contains(identityColumns["symbol_migrations"], column.name) && column.typeOID == pgtype.TextOID {
			return true
		}
	}
	return false
}

func hasSchemaColumn(table schemaTable, name string) bool {
	for _, column := range table.columns {
		if column.name == name {
			return true
		}
	}
	return false
}

func missingIndexQueries(schema string, inv symbolSchemaInventory) []string {
	var queries []string
	for _, spec := range symbolIndexSpecs {
		if _, ok := inv.indexes[spec.name]; ok {
			continue
		}
		create := "CREATE INDEX "
		if spec.unique {
			create = "CREATE UNIQUE INDEX "
		}
		queries = append(queries, create+pgx.Identifier{spec.name}.Sanitize()+` ON `+
			pgx.Identifier{schema, spec.table}.Sanitize()+` (`+joinIdentifiers(spec.columns)+`)`)
	}
	return queries
}

func joinIdentifiers(names []string) string {
	result := ""
	for i, name := range names {
		if i != 0 {
			result += ","
		}
		result += pgx.Identifier{name}.Sanitize()
	}
	return result
}

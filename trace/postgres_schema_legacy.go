package trace

import "github.com/jackc/pgx/v5"

const legacyPostgresActivationSource = "legacy-postgres-no-gob"

// legacyPostgresActivationBackfillQuery activates only projects represented by
// authoritative symbol_files rows from the accepted pre-migration-table
// PostgreSQL layout. It deliberately records no GOB fingerprint.
func legacyPostgresActivationBackfillQuery(schema string) string {
	migrations := pgx.Identifier{schema, "symbol_migrations"}.Sanitize()
	files := pgx.Identifier{schema, "symbol_files"}.Sanitize()
	return `INSERT INTO ` + migrations + ` (` +
		`project_id,state,source_path,source_digest,source_size,started_at,completed_at,last_mutation_at) ` +
		`SELECT f.project_id,'completed',convert_to('` + legacyPostgresActivationSource + `','UTF8'),NULL,NULL,` +
		`statement_timestamp(),statement_timestamp(),MAX(f.mod_time) FROM ` + files + ` f GROUP BY f.project_id`
}

package trace

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrPostgresSymbolStoreActivationRequired marks a mutation attempted before
// Load has activated the project in PostgreSQL.
var ErrPostgresSymbolStoreActivationRequired = errors.New("postgres symbol store activation required")

// PostgresSymbolStoreActivationRequiredError describes a rejected mutation.
type PostgresSymbolStoreActivationRequiredError struct {
	ProjectID string
	Operation string
	State     string
}

func (e *PostgresSymbolStoreActivationRequiredError) Error() string {
	detail := "no activation marker exists"
	if e.State != "" {
		detail = fmt.Sprintf("activation state is %q", e.State)
	}
	return fmt.Sprintf("cannot %s PostgreSQL symbols for project %q: %s; call Load before mutating the symbol store", e.Operation, e.ProjectID, detail)
}

func (*PostgresSymbolStoreActivationRequiredError) Unwrap() error {
	return ErrPostgresSymbolStoreActivationRequired
}

func (s *PostgresSymbolStore) requireProjectActivation(ctx context.Context, tx pgx.Tx, operation string) error {
	var state string
	err := tx.QueryRow(ctx, `SELECT state FROM symbol_migrations WHERE project_id=$1 FOR KEY SHARE`, identityBytes(s.projectID)).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return &PostgresSymbolStoreActivationRequiredError{ProjectID: s.projectID, Operation: operation}
	}
	if err != nil {
		return fmt.Errorf("failed to verify PostgreSQL symbol store activation: %w", err)
	}
	if state != "completed" {
		return &PostgresSymbolStoreActivationRequiredError{ProjectID: s.projectID, Operation: operation, State: state}
	}
	return nil
}

func (s *PostgresSymbolStore) recordProjectMutation(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `UPDATE symbol_migrations SET last_mutation_at=clock_timestamp() WHERE project_id=$1`, identityBytes(s.projectID)); err != nil {
		return fmt.Errorf("failed to record PostgreSQL symbol mutation: %w", err)
	}
	return nil
}

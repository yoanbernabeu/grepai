package trace

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresSymbolStore(ctx context.Context, dsn, projectID, projectRoot string) (*PostgresSymbolStore, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to configure postgres: %w", err)
	}
	return newPostgresSymbolStoreWithPoolConfig(ctx, poolConfig, projectID, projectRoot)
}

func newPostgresSymbolStoreWithPoolConfig(ctx context.Context, poolConfig *pgxpool.Config, projectID, projectRoot string) (*PostgresSymbolStore, error) {
	probe, err := pgxpool.NewWithConfig(ctx, poolConfig.Copy())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	var schema string
	if err := probe.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		probe.Close()
		return nil, fmt.Errorf("failed to resolve postgres symbol schema: %w", err)
	}
	probe.Close()
	if schema == "" {
		return nil, fmt.Errorf("failed to resolve postgres symbol schema: current_schema is null")
	}

	pinned := poolConfig.Copy()
	if pinned.ConnConfig.RuntimeParams == nil {
		pinned.ConnConfig.RuntimeParams = make(map[string]string)
	}
	searchPath := pgx.Identifier{schema}.Sanitize()
	pinned.ConnConfig.RuntimeParams["search_path"] = searchPath
	afterConnect := pinned.AfterConnect
	pinned.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if afterConnect != nil {
			if err := afterConnect(ctx, conn); err != nil {
				return err
			}
		}
		_, err := conn.Exec(ctx, `SET search_path TO `+searchPath)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pinned)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	s := &PostgresSymbolStore{pool: pool, schema: schema, projectID: projectID, projectRoot: projectRoot}
	if err := s.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

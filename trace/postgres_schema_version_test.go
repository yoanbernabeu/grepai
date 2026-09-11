package trace

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type schemaVersionRow struct {
	version int
	err     error
}

func (r schemaVersionRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*int)) = r.version
	return nil
}

type schemaVersionReaderStub struct{ row schemaVersionRow }

func (r schemaVersionReaderStub) QueryRow(context.Context, string, ...any) pgx.Row {
	return r.row
}

func TestReadSymbolSchemaVersionRecognizesOnlyUndefinedTable(t *testing.T) {
	version, missing, err := readSymbolSchemaVersion(context.Background(), schemaVersionReaderStub{row: schemaVersionRow{err: &pgconn.PgError{Code: "42P01"}}})
	if err != nil || !missing || version != 0 {
		t.Fatalf("undefined table = version %d, missing %v, err %v", version, missing, err)
	}

	want := errors.New("connection failed")
	_, missing, err = readSymbolSchemaVersion(context.Background(), schemaVersionReaderStub{row: schemaVersionRow{err: want}})
	if !errors.Is(err, want) || missing {
		t.Fatalf("unrelated error = missing %v, err %v", missing, err)
	}
}

func TestSchemaAdvisoryKeyHasDedicatedNamespace(t *testing.T) {
	s1, s2 := schemaAdvisoryKey()
	m1, m2 := migrationAdvisoryKey("project")
	f1, f2 := fileMutationAdvisoryKey("project", "file")
	if (s1 == m1 && s2 == m2) || (s1 == f1 && s2 == f2) {
		t.Fatal("schema advisory key collides with another lock namespace")
	}
}

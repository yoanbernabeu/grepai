package trace

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type countingCopyFrom struct {
	calls int
	rows  int
}

func (c *countingCopyFrom) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, source pgx.CopyFromSource) (int64, error) {
	var count int64
	for source.Next() {
		if _, err := source.Values(); err != nil {
			return 0, err
		}
		count++
	}
	if err := source.Err(); err != nil {
		return 0, err
	}
	c.calls++
	c.rows += int(count)
	return count, nil
}

func TestMigrationBatchCopyCountIsBoundedByBatchesAndTables(t *testing.T) {
	files := make([]migrationFileRows, 1001)
	for i := range files {
		file := "file.go"
		files[i] = migrationFileRows{
			filePath: file,
			symbols:  []Symbol{{Name: "Symbol", File: file}},
			refs:     []Reference{{SymbolName: "Target", File: file, CallerName: "Symbol"}},
		}
	}
	spy := &countingCopyFrom{}
	for start := 0; start < len(files); start += migrationBatchSize {
		end := min(start+migrationBatchSize, len(files))
		if err := copyMigrationFileBatch(context.Background(), spy, "project", files[start:end]); err != nil {
			t.Fatal(err)
		}
	}
	if spy.calls != 12 {
		t.Fatalf("CopyFrom calls = %d, want 12 for 3 batches × 4 tables", spy.calls)
	}
	if spy.rows != 1001*4 {
		t.Fatalf("copied rows = %d, want %d", spy.rows, 1001*4)
	}
}

func TestPostgresRowBuilderPreservesIdentityDisplayAndOrdinals(t *testing.T) {
	file := "bad/\xff.go"
	rows := buildPostgresFileRows("project\xfe", file, "hash", "extractor", []Symbol{{Name: "Name\xfd", File: file, Signature: "sig\xff"}}, []Reference{{SymbolName: "Target\xfc", File: file, Context: "ctx\xff", CallerName: "Name\xfd", CallerFile: file}}, time.Unix(1, 0))
	if string(rows.symbols[0].projectID) != "project\xfe" || string(rows.symbols[0].name) != "Name\xfd" || string(rows.symbols[0].file) != file {
		t.Fatalf("identity bytes changed: %#v", rows.symbols[0])
	}
	if rows.symbols[0].signature != "sig�" || rows.refs[0].context != "ctx�" {
		t.Fatalf("display fields were not sanitized: %#v %#v", rows.symbols[0], rows.refs[0])
	}
	if rows.refs[0].ordinal != 0 || rows.edges[0].ordinal != 0 {
		t.Fatalf("ordinals drifted: %#v %#v", rows.refs[0], rows.edges[0])
	}
}

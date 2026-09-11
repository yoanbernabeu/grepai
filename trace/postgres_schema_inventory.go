package trace

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type schemaColumn struct {
	name, defaultExpr string
	typeOID           uint32
	nullable          bool
}

type schemaTable struct {
	kind    byte
	columns []schemaColumn
	pk      []string
}

type schemaIndex struct {
	table, method      string
	columns            []string
	valid, ready       bool
	unique, expression bool
}

type symbolSchemaInventory struct {
	tables  map[string]schemaTable
	indexes map[string]schemaIndex
}

func inspectSymbolSchema(ctx context.Context, tx pgx.Tx, schema string) (symbolSchemaInventory, error) {
	inv := symbolSchemaInventory{tables: make(map[string]schemaTable), indexes: make(map[string]schemaIndex)}
	rows, err := tx.Query(ctx, `
		SELECT c.relname,c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=$1 AND c.relname=ANY($2)`, schema, reservedSymbolTables)
	if err != nil {
		return inv, fmt.Errorf("failed to inventory symbol relations: %w", err)
	}
	for rows.Next() {
		var name string
		var kind byte
		if err := rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return inv, err
		}
		inv.tables[name] = schemaTable{kind: kind}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return inv, err
	}
	rows.Close()
	rows, err = tx.Query(ctx, `
		SELECT c.relname,c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=$1 AND c.relname=ANY($2)`, schema, reservedSymbolIndexes())
	if err != nil {
		return inv, fmt.Errorf("failed to inventory reserved symbol index names: %w", err)
	}
	for rows.Next() {
		var name string
		var kind byte
		if err := rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return inv, err
		}
		if kind != 'i' {
			rows.Close()
			return inv, fmt.Errorf("reserved symbol index name %q is owned by a non-index relation", name)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return inv, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `
		SELECT c.relname,a.attname,t.oid,NOT a.attnotnull,coalesce(pg_get_expr(d.adbin,0),'')
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
		JOIN pg_type t ON t.oid=a.atttypid LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum
		WHERE n.nspname=$1 AND c.relname=ANY($2) ORDER BY c.relname,a.attnum`, schema, reservedSymbolTables)
	if err != nil {
		return inv, fmt.Errorf("failed to inventory symbol columns: %w", err)
	}
	for rows.Next() {
		var table string
		var column schemaColumn
		if err := rows.Scan(&table, &column.name, &column.typeOID, &column.nullable, &column.defaultExpr); err != nil {
			rows.Close()
			return inv, err
		}
		entry := inv.tables[table]
		entry.columns = append(entry.columns, column)
		inv.tables[table] = entry
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return inv, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `
		SELECT c.relname,array_agg(a.attname ORDER BY u.ord)
		FROM pg_constraint p JOIN pg_class c ON c.oid=p.conrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		JOIN unnest(p.conkey) WITH ORDINALITY u(attnum,ord) ON true
		JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=u.attnum
		WHERE n.nspname=$1 AND c.relname=ANY($2) AND p.contype='p' GROUP BY c.relname`, schema, reservedSymbolTables)
	if err != nil {
		return inv, fmt.Errorf("failed to inventory symbol primary keys: %w", err)
	}
	for rows.Next() {
		var table string
		var columns []string
		if err := rows.Scan(&table, &columns); err != nil {
			rows.Close()
			return inv, err
		}
		entry := inv.tables[table]
		entry.pk = columns
		inv.tables[table] = entry
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return inv, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `
		SELECT ic.relname,tc.relname,am.amname,i.indisvalid,i.indisready,i.indisunique,
		       i.indexprs IS NOT NULL OR i.indpred IS NOT NULL,
		       ARRAY(SELECT a.attname FROM unnest(i.indkey) WITH ORDINALITY k(attnum,ord)
		             JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum
		             WHERE k.ord<=i.indnkeyatts ORDER BY k.ord)
		FROM pg_class ic JOIN pg_namespace n ON n.oid=ic.relnamespace
		JOIN pg_index i ON i.indexrelid=ic.oid JOIN pg_class tc ON tc.oid=i.indrelid
		JOIN pg_am am ON am.oid=ic.relam WHERE n.nspname=$1 AND ic.relname=ANY($2)`, schema, reservedSymbolIndexes())
	if err != nil {
		return inv, fmt.Errorf("failed to inventory symbol indexes: %w", err)
	}
	for rows.Next() {
		var name string
		var index schemaIndex
		if err := rows.Scan(&name, &index.table, &index.method, &index.valid, &index.ready, &index.unique, &index.expression, &index.columns); err != nil {
			rows.Close()
			return inv, err
		}
		inv.indexes[name] = index
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return inv, err
	}
	rows.Close()
	return inv, nil
}

func (inv symbolSchemaInventory) validateIndexes() error {
	for _, spec := range symbolIndexSpecs {
		index, ok := inv.indexes[spec.name]
		if !ok {
			continue
		}
		if index.table != spec.table || index.method != "btree" || !index.valid || !index.ready || index.unique != spec.unique || index.expression || !slices.Equal(index.columns, spec.columns) {
			return fmt.Errorf("reserved symbol index %q has incompatible owner or definition", spec.name)
		}
	}
	return nil
}

func (inv symbolSchemaInventory) hasAllIndexes() bool {
	for _, spec := range symbolIndexSpecs {
		if _, ok := inv.indexes[spec.name]; !ok {
			return false
		}
	}
	return true
}

func defaultKind(expr string) string {
	canonical := strings.TrimSpace(expr)
	switch canonical {
	case "":
		return ""
	case "0":
		return "zero"
	case "false":
		return "false"
	case "''::text", `'\x'::bytea`:
		return "empty"
	default:
		return canonical
	}
}

func validateTable(name string, got schemaTable, allowed [][]columnRule, pk []string) error {
	if got.kind != 'r' {
		return fmt.Errorf("reserved symbol relation %q is not an ordinary table", name)
	}
	for _, shape := range allowed {
		if len(shape) != len(got.columns) || !slices.Equal(got.pk, pk) {
			continue
		}
		columns := make(map[string]schemaColumn, len(got.columns))
		for _, column := range got.columns {
			columns[column.name] = column
		}
		valid := true
		for _, rule := range shape {
			column, ok := columns[rule.name]
			if !ok || !slices.Contains(rule.typeOIDs, column.typeOID) || column.nullable != rule.nullable || defaultKind(column.defaultExpr) != rule.defaultKind {
				valid = false
				break
			}
		}
		if valid {
			return nil
		}
	}
	return fmt.Errorf("reserved symbol table %q has an incompatible shape", name)
}

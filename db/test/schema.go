package dbtest

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Querier is the subset of *sql.DB / *db.DB needed to introspect a schema.
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// SQLiteSchema returns a normalized, comparable description of the database schema: tables
// with their columns, named indexes, and foreign keys. Column order, declared type spelling
// (INT vs INTEGER) and default values are not part of the description, so the schema produced
// by a migration chain can be compared to a freshly created one.
func SQLiteSchema(t testing.TB, d Querier) string {
	t.Helper()
	lines := make([]string, 0)
	for _, table := range sqliteTables(t, d) {
		lines = append(lines, "table "+table)
		lines = append(lines, sqliteColumns(t, d, table)...)
		lines = append(lines, sqliteForeignKeys(t, d, table)...)
		lines = append(lines, sqliteIndexes(t, d, table)...)
	}
	return strings.Join(lines, "\n")
}

// PostgresSchema is SQLiteSchema's PostgreSQL counterpart, describing the current schema's
// tables, columns, constraints and indexes in a normalized, comparable way.
func PostgresSchema(t testing.TB, d Querier) string {
	t.Helper()
	lines := make([]string, 0)
	for _, table := range postgresTables(t, d) {
		lines = append(lines, "table "+table)
		lines = append(lines, postgresColumns(t, d, table)...)
	}
	lines = append(lines, postgresConstraints(t, d)...)
	lines = append(lines, postgresIndexes(t, d)...)
	return strings.Join(lines, "\n")
}

func sqliteTables(t testing.TB, d Querier) []string {
	t.Helper()
	return queryStrings(t, d, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
}

func sqliteColumns(t testing.TB, d Querier, table string) []string {
	t.Helper()
	rows, err := d.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	require.Nil(t, err)
	defer rows.Close()
	lines := make([]string, 0)
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		require.Nil(t, rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk))
		typ = strings.ToUpper(typ)
		if typ == "INT" { // INT and INTEGER are the same affinity; migrations spell them inconsistently
			typ = "INTEGER"
		}
		lines = append(lines, fmt.Sprintf("  col %s %s notnull=%d pk=%d", name, typ, notNull, pk))
	}
	require.Nil(t, rows.Err())
	sort.Strings(lines)
	return lines
}

func sqliteForeignKeys(t testing.TB, d Querier, table string) []string {
	t.Helper()
	rows, err := d.Query(fmt.Sprintf(`PRAGMA foreign_key_list(%q)`, table))
	require.Nil(t, err)
	defer rows.Close()
	lines := make([]string, 0)
	for rows.Next() {
		var id, seq int
		var refTable, from, onUpdate, onDelete, match string
		var to sql.NullString // NULL when referencing the parent's primary key implicitly
		require.Nil(t, rows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match))
		lines = append(lines, fmt.Sprintf("  fk %s -> %s(%s) on_delete=%s", from, refTable, to.String, onDelete))
	}
	require.Nil(t, rows.Err())
	sort.Strings(lines)
	return lines
}

func sqliteIndexes(t testing.TB, d Querier, table string) []string {
	t.Helper()
	rows, err := d.Query(fmt.Sprintf(`PRAGMA index_list(%q)`, table))
	require.Nil(t, err)
	type index struct {
		name            string
		unique, partial int
	}
	indexes := make([]index, 0)
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		require.Nil(t, rows.Scan(&seq, &name, &unique, &origin, &partial))
		// Skip auto-indexes backing PRIMARY KEY/UNIQUE table constraints; those are described
		// by the column and constraint listings already
		if strings.HasPrefix(name, "sqlite_autoindex_") {
			continue
		}
		indexes = append(indexes, index{name, unique, partial})
	}
	require.Nil(t, rows.Err())
	require.Nil(t, rows.Close())
	lines := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		cols := queryStrings(t, d, fmt.Sprintf(`SELECT name FROM pragma_index_info(%q) ORDER BY seqno`, idx.name))
		lines = append(lines, fmt.Sprintf("  index %s unique=%d partial=%d cols=(%s)", idx.name, idx.unique, idx.partial, strings.Join(cols, ",")))
	}
	sort.Strings(lines)
	return lines
}

func postgresTables(t testing.TB, d Querier) []string {
	t.Helper()
	return queryStrings(t, d, `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`)
}

func postgresColumns(t testing.TB, d Querier, table string) []string {
	t.Helper()
	rows, err := d.Query(`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 ORDER BY column_name`, table)
	require.Nil(t, err)
	defer rows.Close()
	lines := make([]string, 0)
	for rows.Next() {
		var name, typ, nullable string
		require.Nil(t, rows.Scan(&name, &typ, &nullable))
		lines = append(lines, fmt.Sprintf("  col %s %s nullable=%s", name, typ, nullable))
	}
	require.Nil(t, rows.Err())
	return lines
}

func postgresConstraints(t testing.TB, d Querier) []string {
	t.Helper()
	return queryStrings(t, d, `
		SELECT 'constraint ' || conrelid::regclass::text || ': ' || pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE connamespace = current_schema()::regnamespace
		ORDER BY 1
	`)
}

func postgresIndexes(t testing.TB, d Querier) []string {
	t.Helper()
	rows, err := d.Query(`SELECT indexname, indexdef, schemaname FROM pg_indexes WHERE schemaname = current_schema() ORDER BY indexname`)
	require.Nil(t, err)
	defer rows.Close()
	lines := make([]string, 0)
	for rows.Next() {
		var name, def, schema string
		require.Nil(t, rows.Scan(&name, &def, &schema))
		// The index definition qualifies the table with the (test-specific) schema name; strip
		// it so snapshots from different test schemas compare equal
		def = strings.ReplaceAll(def, schema+".", "")
		lines = append(lines, "index "+def)
	}
	require.Nil(t, rows.Err())
	return lines
}

func queryStrings(t testing.TB, d Querier, query string) []string {
	t.Helper()
	rows, err := d.Query(query)
	require.Nil(t, err)
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var value string
		require.Nil(t, rows.Scan(&value))
		values = append(values, value)
	}
	require.Nil(t, rows.Err())
	return values
}

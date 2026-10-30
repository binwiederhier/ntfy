package message

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// See TestPostgresQueries_GenericPlansUseOnlyIndexConditions
func TestSQLiteQueries_SinceIDAndTailUseOnlyIndexConditions(t *testing.T) {
	// SQLite plans without the parameter values, so this is the plan it always uses
	c, err := NewSQLiteStore(filepath.Join(t.TempDir(), "cache.db"), "", 0, 0, 0, false)
	require.Nil(t, err)
	defer c.Close()
	for name, q := range map[string]struct {
		query string
		args  []any
	}{
		"since id":                    {sqliteQueries.selectMessagesSinceID, []any{"mytopic", 1000, 1792160000}},
		"since id, include scheduled": {sqliteQueries.selectMessagesSinceIDScheduled, []any{"mytopic", 1000, 1792160000}},
		"tail":                        {sqliteQueries.selectMessagesAfterRowID, []any{1000, "[998, 999]", 1000}},
	} {
		rows, err := c.db.Primary().Query("EXPLAIN QUERY PLAN "+q.query, q.args...)
		require.Nil(t, err)
		plan := make([]string, 0)
		for rows.Next() {
			var id, parent, notUsed int
			var detail string
			require.Nil(t, rows.Scan(&id, &parent, &notUsed, &detail))
			plan = append(plan, detail)
		}
		require.Nil(t, rows.Close())
		for _, step := range plan {
			// "(topic=?)" alone reads every row of the topic, and "rowid<?" every row written before the marker
			require.False(t, strings.HasPrefix(step, "SCAN messages"), "%s: full table scan:\n%s", name, strings.Join(plan, "\n"))
			require.False(t, strings.HasSuffix(step, "(topic=?)"), "%s: reads the whole topic:\n%s", name, strings.Join(plan, "\n"))
			require.False(t, strings.HasSuffix(step, "(topic=? AND rowid<?)"), "%s: reads the topic's history:\n%s", name, strings.Join(plan, "\n"))
		}
	}
}

package message

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
)

// The since=<id> replay must narrow every scan of the message table with index conditions alone. A
// condition the index cannot check becomes a filter applied after each row is read from the table:
// on 2026-10-16 (v2.31.0) an OR in this query made every reconnect read its whole topic, and the
// reconnects of ntfy.sh's subscribers saturated a read replica.

func TestPostgresQueries_SinceIDUsesOnlyIndexConditions(t *testing.T) {
	d := dbtest.CreateTestPostgres(t)
	_, err := NewPostgresStore(d, 0, 0)
	require.Nil(t, err)
	conn, err := d.Primary().Conn(context.Background())
	require.Nil(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(context.Background(), "SET enable_seqscan = off") // An empty test table would otherwise be read sequentially
	require.Nil(t, err)
	for name, query := range map[string]string{
		"published":         postgresQueries.selectMessagesSinceID,
		"include scheduled": postgresQueries.selectMessagesSinceIDScheduled,
	} {
		rows, err := conn.QueryContext(context.Background(), "EXPLAIN "+query, "mytopic", 1000, 1792160000)
		require.Nil(t, err)
		plan := make([]string, 0)
		for rows.Next() {
			var line string
			require.Nil(t, rows.Scan(&line))
			plan = append(plan, line)
		}
		require.Nil(t, rows.Close())
		require.NotContains(t, strings.Join(plan, "\n"), "Filter:", "%s: rows are filtered after being read:\n%s", name, strings.Join(plan, "\n"))
	}
}

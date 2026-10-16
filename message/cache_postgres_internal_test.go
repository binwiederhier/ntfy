package message

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
)

// The since=<id> replay and the tail read must narrow every scan of the message table with index
// conditions alone. A condition the index cannot check becomes a filter applied after each row is
// read from the table: on 2026-10-16 (v2.31.0) an OR in the replay query made every reconnect read
// its whole topic, and the reconnects of ntfy.sh's subscribers saturated a read replica.
//
// The plans checked are the generic ones: the driver prepares its statements, and after five
// executions Postgres may switch to a plan made without the parameter values. That plan is the one
// that read the whole topic; with the values, the planner had found a cheaper plan.

func TestPostgresQueries_GenericPlansUseOnlyIndexConditions(t *testing.T) {
	d := dbtest.CreateTestPostgres(t)
	_, err := NewPostgresStore(d, 0, 0)
	require.Nil(t, err)
	for name, q := range map[string]struct {
		query string
		args  string
	}{
		"since id":                    {postgresQueries.selectMessagesSinceID, `'mytopic', 1000, 1792160000`},
		"since id, include scheduled": {postgresQueries.selectMessagesSinceIDScheduled, `'mytopic', 1000, 1792160000`},
		"tail":                        {postgresQueries.selectMessagesAfterRowID, `1000, '[998, 999]', 1000`},
	} {
		plan := postgresGenericPlan(t, d.Primary(), q.query, q.args)
		require.NotContains(t, plan, "Filter:", "%s: rows are filtered after being read:\n%s", name, plan)
	}
}

// postgresGenericPlan returns the generic plan of a query, prepared and explained over the simple
// protocol, which passes $1, $2, ... through to Postgres instead of binding them
func postgresGenericPlan(t *testing.T, db *sql.DB, query, args string) string {
	conn, err := db.Conn(context.Background())
	require.Nil(t, err)
	defer conn.Close()
	var plan string
	require.Nil(t, conn.Raw(func(driverConn any) error {
		pgConn := driverConn.(*stdlib.Conn).Conn().PgConn()
		results, err := pgConn.Exec(context.Background(), `
			DEALLOCATE ALL; -- The pool reuses connections
			SET enable_seqscan = off; -- An empty test table would otherwise be read sequentially
			SET plan_cache_mode = force_generic_plan;
			PREPARE q AS `+query+`;
			EXPLAIN EXECUTE q(`+args+`);
		`).ReadAll()
		if err != nil {
			return err
		}
		lines := make([]string, 0)
		for _, row := range results[len(results)-1].Rows {
			lines = append(lines, string(row[0]))
		}
		plan = strings.Join(lines, "\n")
		return nil
	}))
	return plan
}

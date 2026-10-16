package message

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	dbtest "heckel.io/ntfy/v2/db/test"
)

// The since=<id> replay and the tail read must start every scan of the message table at the marker:
// the index condition needs a lower bound (id >, time >=) or an exact set of ids (= ANY). Narrowed by
// topic alone, or bounded only from above (id <), a scan reads the topic's whole history: on 2026-10-16
// (v2.31.0) an OR in the replay query made every reconnect read its whole topic, and the reconnects of
// ntfy.sh's subscribers saturated a read replica.
//
// The plans checked are the generic ones: the driver prepares its statements, and after five
// executions Postgres may switch to a plan made without the parameter values. That plan is the one
// that read the whole topic; with the values, the planner had found a cheaper plan.

func TestPostgresQueries_GenericPlansUseOnlyIndexConditions(t *testing.T) {
	d := dbtest.CreateTestPostgres(t)
	_, err := NewPostgresStore(d, 0, 0)
	require.Nil(t, err)
	// Rows and statistics shaped like ntfy.sh's cache: a few busy topics (6% of the rows each) among many
	// small ones, written in time order. The planner picks plans by these statistics: on a small or even
	// table, it finds cheap plans even for the queries that read whole topics on a real server.
	_, err = d.Primary().Exec(`
		INSERT INTO message (mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions,
			attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, sender, user_id, content_type, encoding, published)
		SELECT 'm' || g, 'm' || g, 1792100000 + g / 5, 'message', 1792200000 + g, CASE
				WHEN g % 100 < 6 THEN 'mytopic'
				WHEN g % 100 < 30 THEN 'busy' || (g % 4)
				ELSE 'small' || (g % 20000)
			END,
			'body', '', 3, '', '', '', '', '', '', 0, 0, '', '', '', '', '', TRUE
		FROM generate_series(1, 200000) g;
		ANALYZE message`)
	require.Nil(t, err)
	for name, q := range map[string]struct {
		query       string
		args        string
		topicScoped bool // A replay reads one topic, the tail all of them
	}{
		"since id":                    {postgresQueries.selectMessagesSinceID, `'mytopic', 1000, 1792160000`, true},
		"since id, include scheduled": {postgresQueries.selectMessagesSinceIDScheduled, `'mytopic', 1000, 1792160000`, true},
		"tail":                        {postgresQueries.selectMessagesAfterRowID, `1000, '[998, 999]', 1000`, false},
	} {
		plan := postgresGenericPlan(t, d.Primary(), q.query, q.args)
		require.NotContains(t, plan, "Seq Scan on message", "%s: reads the whole table:\n%s", name, plan)
		scans := postgresIndexScanRe.FindAllStringSubmatch(plan, -1)
		require.NotEmpty(t, scans, "%s: no index scan:\n%s", name, plan)
		for _, scan := range scans {
			index, condition := scan[1], scan[2]
			bounds, known := postgresScanBounds[index]
			require.True(t, known, "%s: scan on unexpected index %s:\n%s", name, index, plan)
			require.False(t, q.topicScoped && index == "message_pkey", "%s: a scan reads other topics' rows:\n%s", name, plan)
			require.Regexp(t, bounds, condition, "%s: the scan on %s does not start at the marker:\n%s", name, index, plan)
		}
	}
}

func TestPostgresStore_CreatesTopicPublishedIDIndex(t *testing.T) {
	// since=<id> replays read the rows after the marker through (topic, published, id); without it,
	// v2.30.0's replay walked every index entry of the topic (ntfy.sh, 2026-10-16)
	d := dbtest.CreateTestPostgres(t)
	_, err := NewPostgresStore(d, 0, 0)
	require.Nil(t, err)
	var definition string
	require.Nil(t, d.Primary().QueryRow(`SELECT indexdef FROM pg_indexes WHERE tablename = 'message' AND indexname = 'idx_message_topic_published_id'`).Scan(&definition))
	require.Contains(t, definition, "(topic, published, id)")
}

var (
	// postgresIndexScanRe matches an index scan and its index condition (these queries read only the message table)
	postgresIndexScanRe = regexp.MustCompile(`Index (?:Only )?Scan(?: Backward)? (?:using|on) (\S+)[^\n]*\n\s+Index Cond: ([^\n]+)`)

	// postgresScanBounds are the conditions that start a scan of each index at the marker. A condition on a
	// later index column does not: on (topic, published, time, id), "id > $2" still walks the whole topic.
	// Unpublished (scheduled) messages are listed in full, and they are few on a whole server.
	postgresScanBounds = map[string]*regexp.Regexp{
		"idx_message_topic_published_time": regexp.MustCompile(`"time" >= |published = false`),
		"idx_message_topic_published_id":   regexp.MustCompile(`id > |published = false`),
		"idx_message_published_expires":    regexp.MustCompile(`published = false`),
		"message_pkey":                     regexp.MustCompile(`id > |id = ANY`),
	}
)

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
			SET enable_seqscan = off; -- A small test table would otherwise be read sequentially
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

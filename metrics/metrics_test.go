package metrics

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
)

// expectedMetricNames is the exact set of metrics the server exposes. These names are a public
// contract: renaming or dropping one silently breaks existing dashboards and alerts.
var expectedMetricNames = []string{
	"ntfy_attachments_total_size",
	"ntfy_calls_made_failure",
	"ntfy_calls_made_success",
	"ntfy_cluster_batches_sent_total",
	"ntfy_cluster_leader",
	"ntfy_cluster_messages_forwarded_total",
	"ntfy_cluster_messages_wasted_total",
	"ntfy_cluster_peer_batches_received_total",
	"ntfy_cluster_peer_batches_sent_total",
	"ntfy_cluster_peer_last_send_success_timestamp_seconds",
	"ntfy_cluster_peers",
	"ntfy_cluster_queue_dropped_total",
	"ntfy_cluster_send_errors_total",
	"ntfy_db_pool_connections_idle",
	"ntfy_db_pool_connections_in_use",
	"ntfy_db_pool_connections_max",
	"ntfy_db_pool_connections_open",
	"ntfy_db_pool_wait_seconds_total",
	"ntfy_db_pool_waits_total",
	"ntfy_emails_received_failure",
	"ntfy_emails_received_success",
	"ntfy_emails_sent_failure",
	"ntfy_emails_sent_success",
	"ntfy_firebase_published_failure",
	"ntfy_firebase_published_success",
	"ntfy_http_requests_total",
	"ntfy_matrix_published_failure",
	"ntfy_matrix_published_success",
	"ntfy_message_publish_duration_ms",
	"ntfy_messages_cached_total",
	"ntfy_messages_published_failure",
	"ntfy_messages_published_success",
	"ntfy_subscribers_total",
	"ntfy_tail_messages_lag_seconds",
	"ntfy_tail_messages_missed_total",
	"ntfy_tail_messages_read_total",
	"ntfy_topics_total",
	"ntfy_unifiedpush_published_success",
	"ntfy_users_total",
	"ntfy_visitors_total",
}

func TestRegisteredMetricNames(t *testing.T) {
	HTTPRequests.WithLabelValues("200", "20000", "GET").Inc() // Vectors are only gathered once they have a series
	ClusterPeerBatchesSent.WithLabelValues("node-b", "ok").Inc()
	ClusterPeerBatchesReceived.WithLabelValues("node-b").Inc()
	ClusterPeerLastSendSuccess.WithLabelValues("node-b").Set(1)
	SetDBPoolsFunc(func() []*DBPool {
		return []*DBPool{
			{
				Role: db.PoolRolePrimary,
				Addr: "db:5432",
			},
		}
	})
	defer SetDBPoolsFunc(nil)
	families, err := prometheus.DefaultGatherer.Gather()
	require.Nil(t, err)
	names := make([]string, 0)
	for _, family := range families {
		if strings.HasPrefix(family.GetName(), "ntfy_") {
			names = append(names, family.GetName())
		}
	}
	sort.Strings(names)
	require.Equal(t, expectedMetricNames, names)
}

func TestCollectors_NeverNil(t *testing.T) {
	// Call sites update metrics unconditionally, even when metrics are disabled, so no collector
	// may ever be nil
	MessagesPublishedSuccess.Inc()
	MessagesCached.Set(1)
	HTTPRequests.WithLabelValues("200", "20000", "PUT").Inc()
}

func TestDBPoolMetrics(t *testing.T) {
	// Read from the pools on every scrape, one series per pool, so a burst that exhausts a pool
	// shows up as waits even between two scrapes
	SetDBPoolsFunc(func() []*DBPool {
		return []*DBPool{
			{
				Role: db.PoolRolePrimary,
				Addr: "db:5432",
				Stats: sql.DBStats{
					MaxOpenConnections: 30,
					OpenConnections:    12,
					InUse:              5,
					Idle:               7,
					WaitCount:          3,
					WaitDuration:       1500 * time.Millisecond,
				},
			},
			{
				Role: db.PoolRoleReplica,
				Addr: "replica:5432",
				Stats: sql.DBStats{
					MaxOpenConnections: 20,
					OpenConnections:    2,
					InUse:              1,
					Idle:               1,
				},
			},
		}
	})
	defer SetDBPoolsFunc(nil)
	expected := fmt.Sprintf(`
# HELP ntfy_db_pool_connections_in_use Database connections currently in use, per pool
# TYPE ntfy_db_pool_connections_in_use gauge
ntfy_db_pool_connections_in_use{host="db:5432",role="%[1]s"} 5
ntfy_db_pool_connections_in_use{host="replica:5432",role="%[2]s"} 1
# HELP ntfy_db_pool_connections_max Maximum open database connections (pool_max_conns), per pool
# TYPE ntfy_db_pool_connections_max gauge
ntfy_db_pool_connections_max{host="db:5432",role="%[1]s"} 30
ntfy_db_pool_connections_max{host="replica:5432",role="%[2]s"} 20
# HELP ntfy_db_pool_wait_seconds_total Total time spent waiting for a free database connection, per pool
# TYPE ntfy_db_pool_wait_seconds_total counter
ntfy_db_pool_wait_seconds_total{host="db:5432",role="%[1]s"} 1.5
ntfy_db_pool_wait_seconds_total{host="replica:5432",role="%[2]s"} 0
# HELP ntfy_db_pool_waits_total Times a query had to wait for a free database connection, per pool
# TYPE ntfy_db_pool_waits_total counter
ntfy_db_pool_waits_total{host="db:5432",role="%[1]s"} 3
ntfy_db_pool_waits_total{host="replica:5432",role="%[2]s"} 0
`, db.PoolRolePrimary, db.PoolRoleReplica)
	require.Nil(t, testutil.GatherAndCompare(prometheus.DefaultGatherer, strings.NewReader(expected),
		"ntfy_db_pool_connections_in_use", "ntfy_db_pool_connections_max", "ntfy_db_pool_waits_total", "ntfy_db_pool_wait_seconds_total"))
	open, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "ntfy_db_pool_connections_open", "ntfy_db_pool_connections_idle")
	require.Nil(t, err)
	require.Equal(t, 4, open)

	// Without a database (SQLite, or before the server sets it), there are no series
	SetDBPoolsFunc(nil)
	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, "ntfy_db_pool_connections_max")
	require.Nil(t, err)
	require.Equal(t, 0, count)
}

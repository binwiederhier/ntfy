// Package metrics defines the Prometheus metrics exposed by the ntfy server, and registers them
// with the default Prometheus registry on import. It is decoupled from the ntfy server, so that
// call sites can update metrics without depending on the server package.
package metrics

import (
	"database/sql"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// Collectors for all metrics exposed by the server.
//
// These are never nil, so that call sites can update them unconditionally. If metrics are
// disabled, the server never mounts the /metrics handler, and the values are simply never read.
var (
	MessagesPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_messages_published_success",
	})
	MessagesPublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_messages_published_failure",
	})
	MessagesCached = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_messages_cached_total",
	})
	MessagePublishDurationMillis = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_message_publish_duration_ms",
	})
	FirebasePublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_firebase_published_success",
	})
	FirebasePublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_firebase_published_failure",
	})
	EmailsPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_sent_success",
	})
	EmailsPublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_sent_failure",
	})
	EmailsReceivedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_received_success",
	})
	EmailsReceivedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_received_failure",
	})
	CallsMadeSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_calls_made_success",
	})
	CallsMadeFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_calls_made_failure",
	})
	UnifiedPushPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_unifiedpush_published_success",
	})
	MatrixPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_matrix_published_success",
	})
	MatrixPublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_matrix_published_failure",
	})
	AttachmentsTotalSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_attachments_total_size",
	})
	Visitors = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_visitors_total",
	})
	Users = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_users_total",
	})
	Subscribers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_subscribers_total",
	})
	Topics = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_topics_total",
	})
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ntfy_http_requests_total",
	}, []string{"http_code", "ntfy_code", "http_method"})
	ClusterPeers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_cluster_peers",
	})
	ClusterMessagesForwarded = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_messages_forwarded_total",
	})
	ClusterBatchesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_batches_sent_total",
	})
	ClusterSendErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_send_errors_total",
	})
	ClusterQueueDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_queue_dropped_total",
	})
	UsageRowsWritten = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_usage_rows_written_total",
	})
	UsageFlushErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_usage_flush_errors_total",
	})
	LimitKeys = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_limit_keys",
	})
	ClusterMessagesWasted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_messages_wasted_total",
	})
	ClusterLeader = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_cluster_leader",
	})
	ClusterPeerBatchesSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ntfy_cluster_peer_batches_sent_total", // Per peer, result "ok" or "error": a link that only fails shows up here
	}, []string{"peer", "result"})
	ClusterPeerBatchesReceived = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ntfy_cluster_peer_batches_received_total",
	}, []string{"peer"})
	ClusterPeerLastSendSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ntfy_cluster_peer_last_send_success_timestamp_seconds",
	}, []string{"peer"})
	TailMessagesLagSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_tail_messages_lag_seconds", // Age of the newest message of the last read; well above the write delay means it is behind
	})
	TailMessagesRead = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_tail_messages_read_total",
	})
	TailMessagesMissed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_tail_messages_missed_total", // Sent by the tail, per subscriber that had missed it
	})
	dbPools = newDBPoolCollector() // Read through SetDBPoolsFunc
)

// init registers all collectors with the default Prometheus registry. Registration is
// unconditional: the collectors are only ever exposed if the server mounts the /metrics handler,
// so there is nothing to be gained by tying registration to the config.
func init() {
	prometheus.MustRegister(
		MessagesPublishedSuccess,
		MessagesPublishedFailure,
		MessagesCached,
		MessagePublishDurationMillis,
		FirebasePublishedSuccess,
		FirebasePublishedFailure,
		EmailsPublishedSuccess,
		EmailsPublishedFailure,
		EmailsReceivedSuccess,
		EmailsReceivedFailure,
		CallsMadeSuccess,
		CallsMadeFailure,
		UnifiedPushPublishedSuccess,
		MatrixPublishedSuccess,
		MatrixPublishedFailure,
		AttachmentsTotalSize,
		ClusterPeers,
		ClusterMessagesForwarded,
		ClusterBatchesSent,
		ClusterSendErrors,
		ClusterQueueDropped,
		UsageRowsWritten,
		UsageFlushErrors,
		LimitKeys,
		ClusterMessagesWasted,
		ClusterLeader,
		ClusterPeerBatchesSent,
		ClusterPeerBatchesReceived,
		ClusterPeerLastSendSuccess,
		TailMessagesLagSeconds,
		TailMessagesRead,
		TailMessagesMissed,
		Visitors,
		Users,
		Subscribers,
		Topics,
		HTTPRequests,
		dbPools,
	)
}

// DBPool is one database connection pool's statistics: the primary's or a replica's
type DBPool struct {
	Role  string // db.PoolRolePrimary or db.PoolRoleReplica
	Addr  string // "host:port"
	Stats sql.DBStats
}

// SetDBPoolsFunc sets the function that the ntfy_db_pool_* metrics read their values from on
// every scrape; nil (the default) reports no pools
func SetDBPoolsFunc(fn func() []*DBPool) {
	dbPools.fn.Store(&fn)
}

// dbPoolCollector exports the database connection pools' statistics. It reads them at scrape
// time rather than on a timer, so the in-use count is as fresh as the scrape, and a burst that
// exhausts a pool between two scrapes still shows up in the wait counters.
type dbPoolCollector struct {
	fn      atomic.Pointer[func() []*DBPool]
	metrics []*dbPoolMetric
}

// dbPoolMetric is one ntfy_db_pool_* metric: its description, and how to read it from a pool
type dbPoolMetric struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
	value     func(stats sql.DBStats) float64
}

func newDBPoolCollector() *dbPoolCollector {
	return &dbPoolCollector{
		metrics: []*dbPoolMetric{
			newDBPoolMetric("ntfy_db_pool_connections_max", "Maximum open database connections (pool_max_conns), per pool", prometheus.GaugeValue, func(stats sql.DBStats) float64 {
				return float64(stats.MaxOpenConnections)
			}),
			newDBPoolMetric("ntfy_db_pool_connections_open", "Open database connections, in use or idle, per pool", prometheus.GaugeValue, func(stats sql.DBStats) float64 {
				return float64(stats.OpenConnections)
			}),
			newDBPoolMetric("ntfy_db_pool_connections_in_use", "Database connections currently in use, per pool", prometheus.GaugeValue, func(stats sql.DBStats) float64 {
				return float64(stats.InUse)
			}),
			newDBPoolMetric("ntfy_db_pool_connections_idle", "Idle database connections, per pool", prometheus.GaugeValue, func(stats sql.DBStats) float64 {
				return float64(stats.Idle)
			}),
			newDBPoolMetric("ntfy_db_pool_waits_total", "Times a query had to wait for a free database connection, per pool", prometheus.CounterValue, func(stats sql.DBStats) float64 {
				return float64(stats.WaitCount)
			}),
			newDBPoolMetric("ntfy_db_pool_wait_seconds_total", "Total time spent waiting for a free database connection, per pool", prometheus.CounterValue, func(stats sql.DBStats) float64 {
				return stats.WaitDuration.Seconds()
			}),
		},
	}
}

func newDBPoolMetric(name, help string, valueType prometheus.ValueType, value func(stats sql.DBStats) float64) *dbPoolMetric {
	return &dbPoolMetric{
		desc:      prometheus.NewDesc(name, help, []string{"role", "host"}, nil),
		valueType: valueType,
		value:     value,
	}
}

// Describe implements prometheus.Collector
func (c *dbPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m.desc
	}
}

// Collect implements prometheus.Collector
func (c *dbPoolCollector) Collect(ch chan<- prometheus.Metric) {
	fn := c.fn.Load()
	if fn == nil || *fn == nil {
		return
	}
	for _, pool := range (*fn)() {
		for _, m := range c.metrics {
			ch <- prometheus.MustNewConstMetric(m.desc, m.valueType, m.value(pool.Stats), pool.Role, pool.Addr)
		}
	}
}

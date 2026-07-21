// Package metrics defines the Prometheus metrics exposed by the ntfy server, and registers them
// with the default Prometheus registry on import. It is decoupled from the ntfy server, so that
// call sites can update metrics without depending on the server package.
package metrics

import (
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
	ClusterMessagesBroadcast = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_messages_broadcast_total",
	})
	ClusterSendErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_send_errors_total",
	})
	ClusterQueueDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_cluster_queue_dropped_total",
	})
	ClusterLeader = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_cluster_leader",
	})
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
		Visitors,
		Users,
		Subscribers,
		Topics,
		HTTPRequests,
		ClusterPeers,
		ClusterMessagesBroadcast,
		ClusterSendErrors,
		ClusterQueueDropped,
		ClusterLeader,
	)
}

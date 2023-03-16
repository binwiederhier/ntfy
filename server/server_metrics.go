package server

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricMessagesPublishedSuccess    prometheus.Counter
	metricMessagesPublishedFailure    prometheus.Counter
	metricMessagesCached              prometheus.Gauge
	metricFirebasePublishedSuccess    prometheus.Counter
	metricFirebasePublishedFailure    prometheus.Counter
	metricEmailsPublishedSuccess      prometheus.Counter
	metricEmailsPublishedFailure      prometheus.Counter
	metricEmailsReceivedSuccess       prometheus.Counter
	metricEmailsReceivedFailure       prometheus.Counter
	metricUnifiedPushPublishedSuccess prometheus.Counter
	metricMatrixPublishedSuccess      prometheus.Counter
	metricMatrixPublishedFailure      prometheus.Counter
	metricAttachmentsTotalSize        prometheus.Gauge
	metricVisitors                    prometheus.Gauge
	metricSubscribers                 prometheus.Gauge
	metricTopics                      prometheus.Gauge
	metricHTTPRequests                *prometheus.CounterVec
)

func initMetrics() {
	metricMessagesPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_messages_published_success",
	})
	metricMessagesPublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_messages_published_failure",
	})
	metricMessagesCached = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_messages_cached_total",
	})
	metricFirebasePublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_firebase_published_success",
	})
	metricFirebasePublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_firebase_published_failure",
	})
	metricEmailsPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_sent_success",
	})
	metricEmailsPublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_sent_failure",
	})
	metricEmailsReceivedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_received_success",
	})
	metricEmailsReceivedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_emails_received_failure",
	})
	metricUnifiedPushPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_unifiedpush_published_success",
	})
	metricMatrixPublishedSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_matrix_published_success",
	})
	metricMatrixPublishedFailure = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ntfy_matrix_published_failure",
	})
	metricAttachmentsTotalSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_attachments_total_size",
	})
	metricVisitors = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_visitors_total",
	})
	metricSubscribers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_subscribers_total",
	})
	metricTopics = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ntfy_topics_total",
	})
	metricHTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ntfy_http_requests_total",
	}, []string{"http_code", "ntfy_code", "http_method"})
	prometheus.MustRegister(
		metricMessagesPublishedSuccess,
		metricMessagesPublishedFailure,
		metricMessagesCached,
		metricFirebasePublishedSuccess,
		metricFirebasePublishedFailure,
		metricEmailsPublishedSuccess,
		metricEmailsPublishedFailure,
		metricEmailsReceivedSuccess,
		metricEmailsReceivedFailure,
		metricUnifiedPushPublishedSuccess,
		metricMatrixPublishedSuccess,
		metricMatrixPublishedFailure,
		metricAttachmentsTotalSize,
		metricVisitors,
		metricSubscribers,
		metricTopics,
		metricHTTPRequests,
	)
}

// minc increments a prometheus.Counter if it is non-nil
func minc(counter prometheus.Counter) {
	if counter != nil {
		counter.Inc()
	}
}

// mset sets a prometheus.Gauge if it is non-nil
func mset[T int | int64 | float64](gauge prometheus.Gauge, value T) {
	if gauge != nil {
		gauge.Set(float64(value))
	}
}

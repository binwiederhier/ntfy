package server

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	metrics = newMetrics()
)

type serverMetrics struct {
	messagesPublishedSuccess    prometheus.Counter
	messagesPublishedFailure    prometheus.Counter
	messagesCached              prometheus.Gauge
	firebasePublishedSuccess    prometheus.Counter
	firebasePublishedFailure    prometheus.Counter
	emailsPublishedSuccess      prometheus.Counter
	emailsPublishedFailure      prometheus.Counter
	emailsReceivedSuccess       prometheus.Counter
	emailsReceivedFailure       prometheus.Counter
	unifiedPushPublishedSuccess prometheus.Counter
	matrixPublishedSuccess      prometheus.Counter
	matrixPublishedFailure      prometheus.Counter
	attachmentsTotalSize        prometheus.Gauge
	visitors                    prometheus.Gauge
	subscribers                 prometheus.Gauge
	topics                      prometheus.Gauge
	httpRequests                *prometheus.CounterVec
}

func newMetrics() *serverMetrics {
	m := &serverMetrics{
		messagesPublishedSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_messages_published_success",
		}),
		messagesPublishedFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_messages_published_failure",
		}),
		messagesCached: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ntfy_messages_cached_total",
		}),
		firebasePublishedSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_firebase_published_success",
		}),
		firebasePublishedFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_firebase_published_failure",
		}),
		emailsPublishedSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_emails_sent_success",
		}),
		emailsPublishedFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_emails_sent_failure",
		}),
		emailsReceivedSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_emails_received_success",
		}),
		emailsReceivedFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_emails_received_failure",
		}),
		unifiedPushPublishedSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_unifiedpush_published_success",
		}),
		matrixPublishedSuccess: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_matrix_published_success",
		}),
		matrixPublishedFailure: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ntfy_matrix_published_failure",
		}),
		attachmentsTotalSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ntfy_attachments_total_size",
		}),
		visitors: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ntfy_visitors_total",
		}),
		subscribers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ntfy_subscribers_total",
		}),
		topics: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ntfy_topics_total",
		}),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ntfy_http_requests_total",
		}, []string{"http_code", "ntfy_code", "http_method"}),
	}
	prometheus.MustRegister(
		m.messagesPublishedSuccess,
		m.messagesPublishedFailure,
		m.messagesCached,
		m.firebasePublishedSuccess,
		m.firebasePublishedFailure,
		m.emailsPublishedSuccess,
		m.emailsPublishedFailure,
		m.emailsReceivedSuccess,
		m.emailsReceivedFailure,
		m.unifiedPushPublishedSuccess,
		m.matrixPublishedSuccess,
		m.matrixPublishedFailure,
		m.attachmentsTotalSize,
		m.visitors,
		m.subscribers,
		m.topics,
		m.httpRequests,
	)
	return m
}

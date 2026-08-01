package metrics

import (
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// expectedMetricNames is the exact set of metrics the server exposes. These names are a public
// contract: renaming or dropping one silently breaks existing dashboards and alerts.
var expectedMetricNames = []string{
	"ntfy_attachments_total_size",
	"ntfy_calls_made_failure",
	"ntfy_calls_made_success",
	"ntfy_cluster_batches_sent_total",
	"ntfy_cluster_leader",
	"ntfy_cluster_messages_relayed_total",
	"ntfy_cluster_messages_wasted_total",
	"ntfy_cluster_peers",
	"ntfy_cluster_queue_dropped_total",
	"ntfy_cluster_route_skipped_total",
	"ntfy_cluster_send_errors_total",
	"ntfy_cluster_state_pushes_total",
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
	"ntfy_topics_total",
	"ntfy_unifiedpush_published_success",
	"ntfy_users_total",
	"ntfy_visitors_total",
}

func TestRegisteredMetricNames(t *testing.T) {
	HTTPRequests.WithLabelValues("200", "20000", "GET").Inc()
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

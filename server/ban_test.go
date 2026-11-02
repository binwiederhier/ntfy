package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/ban"
)

// TestServer_BanFeed_WritesOffenderToFile is the end-to-end wiring test: a rejected request flows
// through s.handle -> the error responder -> s.ban.Record, and once the offender's prefix
// breaches, its ban line lands in the ban file (flushed on Close).
func TestServer_BanFeed_WritesOffenderToFile(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	conf := newTestConfig(t, "")
	conf.BanFile = banFile
	conf.BanWindow = time.Minute
	conf.BanThreshold = 1                 // capacity 1: the 2nd rejection breaches
	conf.BanWeights = ban.Weights{"*": 1} // any 4xx counts one strike
	s := newTestServer(t, conf)
	require.NotNil(t, s.ban)

	// A delayed message with caching disabled is a deterministic 400 (errHTTPBadRequestDelayNoCache).
	// request() sends from RemoteAddr 9.9.9.9.
	reject := map[string]string{"Cache": "no", "In": "30 min"}
	for i := 0; i < 3; i++ {
		response := request(t, s, "PUT", "/mytopic", "", reject)
		require.Equal(t, 400, response.Code)
	}

	// Writes are async; Close flushes the buffer. The offender's prefix must be in the feed exactly
	// once (throttled to one line per window).
	s.ban.Close()
	data, err := os.ReadFile(banFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], " 9.9.9.9 9.9.9.9/32 400 ") // <ip> <prefix> <http-code> <ntfy-code>
}

// TestServer_BanFeed_DisabledByDefault verifies the feature is off with no ban file: s.ban is
// nil and the error path skips it (guarded), so a rejected request must not panic.
func TestServer_BanFeed_DisabledByDefault(t *testing.T) {
	conf := newTestConfig(t, "") // no BanFile
	s := newTestServer(t, conf)
	require.Nil(t, s.ban)

	reject := map[string]string{"Cache": "no", "In": "30 min"}
	response := request(t, s, "PUT", "/mytopic", "", reject) // guarded callsite, no Record
	require.Equal(t, 400, response.Code)
}

func TestServer_BanFeed_DefaultWeights(t *testing.T) {
	// Auth-failure floods still ban faster than other rejections, and a refused UnifiedPush push
	// counts for little: a big app server pushing to many offline phones is not banned for it
	weights, err := ban.ParseWeights(DefaultBanWeights)
	require.Nil(t, err)
	require.Equal(t, 2.0, weights.WeightFor(errHTTPTooManyRequestsLimitAuthFailure.Code))
	require.Equal(t, 0.05, weights.WeightFor(errHTTPInsufficientStorageUnifiedPush.Code))
	require.Equal(t, 0.05, weights.WeightFor(errHTTPGoneUnifiedPushTopicAbandoned.Code))
	require.Equal(t, 1.0, weights.WeightFor(errHTTPTooManyRequestsLimitRequests.Code))
}

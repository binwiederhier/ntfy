package ban

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var testIP = netip.MustParseAddr("1.2.3.4")

// newTestService creates a Service wired for testing, with the given ban file, weighted-bucket
// threshold, and weights, plus a 1-minute window (so the emit throttle only fires once per test).
func newTestService(t *testing.T, banFile string, threshold int, weights map[string]float64) *Service {
	t.Helper()
	s := NewService(&Config{
		File:           banFile,
		Window:         time.Minute,
		Threshold:      threshold,
		Weights:        weights,
		PrefixBitsIPv4: 32,
		PrefixBitsIPv6: 64,
	})
	t.Cleanup(s.Close)
	return s
}

// flushAndRead forces a synchronous flush of the buffered bans (writes are otherwise async, on the
// runWriteLoop ticker) and returns the ban file's lines.
func flushAndRead(t *testing.T, s *Service, path string) []string {
	t.Helper()
	s.flush()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestService_Record_Weight2BansAtHalfThreshold(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	// Threshold 10, code weight 2 -> the budget covers exactly 5 hits, so the 6th breaches.
	s := newTestService(t, banFile, 10, map[string]float64{"*": 2})
	for i := 0; i < 5; i++ {
		s.Record(testIP, 400, 40001)
	}
	s.flush()
	require.NoFileExists(t, banFile) // 5 hits * weight 2 = 10 == budget, exactly at the limit, not over
	s.Record(testIP, 400, 40001)     // 6th hit cannot be covered -> breach
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
	require.True(t, strings.HasSuffix(lines[0], " 1.2.3.4 1.2.3.4/32 400 40001")) // <ip> <prefix> <http> <ntfy-code>
}

func TestService_Record_Weight10BansFast(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	// Threshold 10, code weight 10 -> a single hit drains the whole budget, so the 2nd breaches.
	s := newTestService(t, banFile, 10, map[string]float64{"42909": 10, "*": 1})
	s.Record(testIP, 429, 42909)
	s.flush()
	require.NoFileExists(t, banFile)
	s.Record(testIP, 429, 42909) // 2nd hit cannot be covered -> breach
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
	require.True(t, strings.HasSuffix(lines[0], " 1.2.3.4 1.2.3.4/32 429 42909"))
}

func TestService_Record_Weight0NeverBans(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	// The legit-quota code is exempt (weight 0), so no number of hits ever bans.
	s := newTestService(t, banFile, 10, map[string]float64{"42908": 0, "*": 1})
	for i := 0; i < 100; i++ {
		s.Record(testIP, 429, 42908)
	}
	s.flush()
	require.NoFileExists(t, banFile)
}

func TestService_Record_FractionalWeightBansAfterManyHits(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	// Threshold 10, code weight 0.05 -> the budget covers exactly 200 hits, so the 201st breaches.
	s := newTestService(t, banFile, 10, map[string]float64{"50701": 0.05, "*": 1})
	for i := 0; i < 200; i++ {
		s.Record(testIP, 507, 50701)
	}
	s.flush()
	require.NoFileExists(t, banFile)
	s.Record(testIP, 507, 50701)
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
	require.True(t, strings.HasSuffix(lines[0], " 1.2.3.4 1.2.3.4/32 507 50701"))
}

func TestService_Record_SingleBucketNoRelaxation(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	// One shared bucket per prefix: different codes draw down the SAME budget, so mixing them creates
	// no extra headroom (unlike per-code buckets, which would relax the effective limit for a mixed
	// offender).
	s := newTestService(t, banFile, 10, map[string]float64{"403*": 2, "*": 1})
	s.Record(testIP, 403, 40301) // weight 2 -> 8 left
	s.Record(testIP, 403, 40301) // weight 2 -> 6 left
	s.Record(testIP, 403, 40301) // weight 2 -> 4 left
	s.flush()
	require.NoFileExists(t, banFile)
	for i := 0; i < 4; i++ {
		s.Record(testIP, 400, 40001) // weight 1 each -> drains the remaining 4 -> 0 left
	}
	s.flush()
	require.NoFileExists(t, banFile) // 6 + 4 = 10 == budget exactly, still not over
	s.Record(testIP, 400, 40001)     // one more cannot be covered -> breach
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
}

func TestService_Record_ExactLineFormat(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 1, map[string]float64{"*": 1})
	before := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		s.Record(testIP, 429, 42901)
	}
	after := time.Now().UTC()
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
	parts := strings.Split(lines[0], " ")
	require.Len(t, parts, 5) // "<timestamp> <ip> <prefix> <http-code> <ntfy-code>"
	ts, err := time.Parse(time.RFC3339, parts[0])
	require.NoError(t, err)
	require.Equal(t, time.UTC, ts.Location())
	require.False(t, ts.Before(before))
	require.False(t, ts.After(after.Add(time.Second)))
	require.Equal(t, "1.2.3.4", parts[1])    // full IP
	require.Equal(t, "1.2.3.4/32", parts[2]) // masked to the default IPv4 prefix (/32)
	require.Equal(t, "429", parts[3])        // HTTP status
	require.Equal(t, "42901", parts[4])      // ntfy code
}

func TestService_Record_IPv6MaskedToPrefix(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 1, map[string]float64{"*": 1})
	ip := netip.MustParseAddr("2001:db8::abcd")
	for i := 0; i < 3; i++ {
		s.Record(ip, 429, 42901)
	}
	parts := strings.Split(flushAndRead(t, s, banFile)[0], " ")
	require.Len(t, parts, 5)
	require.Equal(t, "2001:db8::abcd", parts[1]) // full IPv6 address
	require.Equal(t, "2001:db8::/64", parts[2])  // masked to the default IPv6 prefix (/64)
}

func TestService_Record_PerPrefixIsolation(t *testing.T) {
	// Each source prefix gets its own bucket: one IP hammering to a breach must not push a different,
	// quiet IP over the edge. This is the whole point of keying by prefix instead of by visitor.
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 2, map[string]float64{"*": 1})
	noisy := netip.MustParseAddr("1.1.1.1")
	quiet := netip.MustParseAddr("2.2.2.2")
	for i := 0; i < 5; i++ {
		s.Record(noisy, 429, 42901) // breaches its own bucket
	}
	s.Record(quiet, 429, 42901) // single hit, well under threshold
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1) // only the noisy prefix is written
	require.True(t, strings.HasSuffix(lines[0], " 1.1.1.1 1.1.1.1/32 429 42901"))
}

func TestService_Record_OncePerWindowThrottle(t *testing.T) {
	// Once a prefix has been written, further breaches within the window must not re-append it, so a
	// persistent offender produces exactly one line per window (mirrors the old per-visitor banEmit).
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 1, map[string]float64{"*": 1})
	for i := 0; i < 50; i++ {
		s.Record(testIP, 429, 42901)
	}
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
}

func TestService_Record_BansPassedIP(t *testing.T) {
	// The Service bans the exact IP passed to Record -- the caller passes the offending request's IP,
	// which for an account-keyed visitor is not the visitor's stored IP.
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 1, map[string]float64{"*": 1})
	offender := netip.MustParseAddr("5.6.7.8")
	for i := 0; i < 3; i++ {
		s.Record(offender, 429, 42901)
	}
	parts := strings.Split(flushAndRead(t, s, banFile)[0], " ")
	require.Equal(t, "5.6.7.8", parts[1])    // the IP passed to Record
	require.Equal(t, "5.6.7.8/32", parts[2]) // its prefix
}

func TestService_Record_Ignores2xx3xx(t *testing.T) {
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 3, map[string]float64{"*": 1})
	// Success and redirects must never count toward a ban, even over the threshold -- otherwise a
	// legit high-volume publisher (lots of 200s) would get banned.
	for i := 0; i < 20; i++ {
		s.Record(testIP, 200, 20000)
		s.Record(testIP, 302, 30000)
	}
	s.flush()
	require.NoFileExists(t, banFile)
	// A 4xx over the same budget still gets written.
	for i := 0; i < 5; i++ {
		s.Record(testIP, 400, 40001)
	}
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
	require.True(t, strings.HasSuffix(lines[0], " 1.2.3.4 1.2.3.4/32 400 40001"))
}

func TestService_Record_BuffersUntilFlush(t *testing.T) {
	// Writes are async: a breach buffers the ban line rather than writing it synchronously on the
	// request path. The line only reaches the file when runWriteLoop (or an explicit flush) runs.
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 1, map[string]float64{"*": 1})
	for i := 0; i < 3; i++ {
		s.Record(testIP, 429, 42901)
	}
	require.NoFileExists(t, banFile) // not written synchronously
	s.mu.Lock()
	require.Len(t, s.pending, 1) // one line buffered (throttled to once per window)
	s.mu.Unlock()
	lines := flushAndRead(t, s, banFile)
	require.Len(t, lines, 1)
	require.True(t, strings.HasSuffix(lines[0], " 1.2.3.4 1.2.3.4/32 429 42901"))
}

func TestService_Close_FlushesPending(t *testing.T) {
	// Close must flush buffered bans so nothing is lost on shutdown, and must block until it has.
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := NewService(&Config{File: banFile, Window: time.Minute, Threshold: 1, Weights: Weights{"*": 1}, PrefixBitsIPv4: 32, PrefixBitsIPv6: 64})
	for i := 0; i < 3; i++ {
		s.Record(testIP, 429, 42901)
	}
	require.NoFileExists(t, banFile) // still buffered
	s.Close()                        // blocks until the final flush completes
	data, err := os.ReadFile(banFile)
	require.NoError(t, err)
	require.Len(t, strings.Split(strings.TrimRight(string(data), "\n"), "\n"), 1)
}

func TestService_Prune_DropsIdlePrefixes(t *testing.T) {
	// A prefix idle for a full window has a refilled bucket, so prune drops it to bound memory. An
	// active prefix (seen within the window) is kept.
	banFile := filepath.Join(t.TempDir(), "ban.log")
	s := newTestService(t, banFile, 10, map[string]float64{"*": 1})
	idle := netip.MustParseAddr("9.9.9.9")
	s.Record(idle, 400, 40001)
	s.Record(testIP, 400, 40001)
	require.Len(t, s.trackers, 2)

	// Backdate the idle prefix past the window, then prune.
	idlePrefix := s.prefix(idle)
	s.mu.Lock()
	s.trackers[idlePrefix].seen = time.Now().Add(-2 * time.Minute)
	s.mu.Unlock()
	s.prune()

	require.Len(t, s.trackers, 1)
	_, ok := s.trackers[idlePrefix]
	require.False(t, ok) // idle prefix dropped
	_, ok = s.trackers[s.prefix(testIP)]
	require.True(t, ok) // active prefix kept
}

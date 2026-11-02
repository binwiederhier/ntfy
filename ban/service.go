// Package ban implements the abuse ban-feed: it tracks per-prefix weighted "strikes" from rejected
// requests and appends breaching prefixes to a ban file that fail2ban tails. Keying by prefix (not
// by visitor) makes the accounting match the unit fail2ban bans, even for shared account visitors.
package ban

import (
	"fmt"
	"math"
	"net/netip"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"heckel.io/ntfy/v2/log"
)

const (
	tag           = "ban"
	pruneInterval = 10 * time.Minute
	writeInterval = 3 * time.Second

	// strikeUnits is how finely strikes are counted: weights go down to 0.01 (e.g. 0.05 = 20 hits
	// make one strike), and the token bucket only takes whole tokens
	strikeUnits = 100
)

// Config is the Service's config, kept separate from server.Config to avoid an import cycle.
type Config struct {
	File           string        // Ban file that fail2ban tails (must be non-empty; the caller decides whether the feature is enabled)
	Window         time.Duration // Rolling window over which weighted strikes are counted
	Threshold      int           // Weighted strikes per Window before a prefix is banned
	Weights        Weights       // Code matcher -> strike weight (0 = exempt)
	PrefixBitsIPv4 int           // Mask width for the ban unit, e.g. 32 (matches rate-limiting granularity)
	PrefixBitsIPv6 int           // Mask width for the ban unit, e.g. 64
}

// tracker is the per-prefix strike state: a weighted breach detector plus timestamps for pruning and throttling.
type tracker struct {
	limiter *rate.Limiter
	seen    time.Time // Last strike, for pruning idle prefixes
	emitted time.Time // Last ban-line write for this prefix, throttles re-emits to once per Window
}

// Service owns the ban-feed: per-prefix strike accounting, buffered file writes, and idle-prefix
// pruning. The caller owns the enable/disable decision -- only construct a Service when the feature
// is on (see server.New, which builds one only when a ban file is configured).
type Service struct {
	conf      *Config
	mu        sync.Mutex // Guards trackers and pending
	trackers  map[netip.Prefix]*tracker
	pending   []string      // Formatted ban lines buffered by Record, flushed to the ban file by runWriteLoop
	writeDone chan struct{} // Closed when runWriteLoop exits after its final flush
	closeChan chan struct{}
	closeOnce sync.Once
}

// NewService builds a Service and starts its background loops. The caller must only call it when the
// feature is enabled (conf non-nil, File non-empty); the Service does not model a disabled state.
func NewService(conf *Config) *Service {
	s := &Service{
		conf:      conf,
		trackers:  make(map[netip.Prefix]*tracker),
		closeChan: make(chan struct{}),
		writeDone: make(chan struct{}),
	}
	go s.runPruneLoop()
	go s.runWriteLoop()
	return s
}

// Record counts one rejection against the IP's prefix bucket and, on breach, buffers a ban line
// (throttled to once per Window per prefix). No-ops for a non-4xx/5xx status or a zero-weight code.
func (s *Service) Record(ip netip.Addr, httpCode, errorCode int) {
	if httpCode < 400 {
		return
	}
	weight := s.conf.Weights.WeightFor(errorCode)
	if weight == 0 {
		return // Weight 0: exempt, no strike
	}
	prefix := s.prefix(ip)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.trackers[prefix]
	if t == nil {
		budget := s.conf.Threshold * strikeUnits
		t = &tracker{limiter: rate.NewLimiter(rate.Limit(float64(budget)/s.conf.Window.Seconds()), budget)}
		s.trackers[prefix] = t
	}
	t.seen = now
	if t.limiter.AllowN(now, int(math.Round(weight*strikeUnits))) {
		return // Within the strike budget, no breach
	}
	if !t.emitted.IsZero() && now.Sub(t.emitted) < s.conf.Window {
		return // Already emitted this prefix within the window (one ban line per prefix per window)
	}
	t.emitted = now
	s.pending = append(s.pending, formatBanLine(now, ip, prefix, httpCode, errorCode))
}

// prefix masks ip to the ban unit (PrefixBitsIPv4/IPv6) -- what fail2ban bans, e.g. a whole /64.
func (s *Service) prefix(ip netip.Addr) netip.Prefix {
	if ip.Is4() {
		return netip.PrefixFrom(ip, s.conf.PrefixBitsIPv4).Masked()
	}
	return netip.PrefixFrom(ip, s.conf.PrefixBitsIPv6).Masked()
}

// formatBanLine builds the "<RFC3339-UTC> <ip> <prefix> <http> <ntfy>" line the fail2ban filter
// parses. The timestamp is captured at breach time, not flush time.
func formatBanLine(t time.Time, ip netip.Addr, prefix netip.Prefix, httpCode, errorCode int) string {
	return fmt.Sprintf("%s %s %s %d %d\n", t.UTC().Format(time.RFC3339), ip.String(), prefix.String(), httpCode, errorCode)
}

// runPruneLoop prunes idle prefixes until Close.
func (s *Service) runPruneLoop() {
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.prune()
		case <-s.closeChan:
			return
		}
	}
}

// runWriteLoop flushes buffered ban lines every writeInterval, plus a final flush on Close.
func (s *Service) runWriteLoop() {
	defer close(s.writeDone)
	ticker := time.NewTicker(writeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.closeChan:
			s.flush()
			return
		}
	}
}

// flush appends the buffered lines to the file in one open/write. Best-effort: a batch is dropped on
// error. Concurrent calls are safe -- pending is drained under mu, so only one flush writes a batch.
func (s *Service) flush() {
	s.mu.Lock()
	lines := s.pending
	s.pending = nil
	s.mu.Unlock()
	if len(lines) == 0 {
		return
	}
	f, err := os.OpenFile(s.conf.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Tag(tag).Err(err).Warn("Cannot open ban file %s, dropped %d ban(s)", s.conf.File, len(lines))
		return
	}
	defer f.Close()
	for i, line := range lines {
		if _, err := f.WriteString(line); err != nil {
			log.Tag(tag).Err(err).Warn("Cannot write to ban file %s, dropped %d ban(s)", s.conf.File, len(lines)-i)
			return
		}
	}
}

// prune drops prefixes idle for a full Window -- their bucket has refilled, so forgetting them is a
// no-op that bounds memory under a flood of distinct IPs.
func (s *Service) prune() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for prefix, t := range s.trackers {
		if now.Sub(t.seen) >= s.conf.Window {
			delete(s.trackers, prefix)
		}
	}
}

// Close stops the loops and blocks until the final flush completes. Idempotent.
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		close(s.closeChan)
		<-s.writeDone
	})
}

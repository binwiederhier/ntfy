// Package ban implements the abuse ban-feed: it tracks per-prefix weighted "strikes" from rejected
// requests and appends breaching prefixes to a ban file that fail2ban tails. Keying by prefix (not
// by visitor) makes the accounting match the unit fail2ban bans, even for shared account visitors.
// The strikes are a kind of usage in the limit service (limit.KindStrikes, one bucket per prefix),
// so in a cluster the nodes share them like the other buckets and a brute force spread over the
// nodes trips the threshold at the same point as on one node.
package ban

import (
	"fmt"
	"net/netip"
	"os"
	"sync"
	"time"

	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/log"
)

const (
	tag           = "ban"
	pruneInterval = 10 * time.Minute
	writeInterval = 3 * time.Second
	keyPrefix     = "ban:" // Limiter keys of the strike buckets: "ban:<prefix>"
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

// Service owns the ban-feed: per-prefix strike accounting (in the limit service), buffered file
// writes, and the once-per-window emit throttle. The caller owns the enable/disable decision --
// only construct a Service when the feature is on (see server.New, which builds one only when a
// ban file is configured).
type Service struct {
	conf      *Config
	limiter   *limit.Service             // Holds the strike buckets, one per prefix (see Key)
	emitted   map[netip.Prefix]time.Time // Last ban-line write per prefix, throttles re-emits to once per Window
	pending   []string                   // Formatted ban lines buffered by Record, flushed to the ban file by runWriteLoop
	writeDone chan struct{}              // Closed when runWriteLoop exits after its final flush
	closeChan chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex // Protects emitted and pending
}

// NewService builds a Service on the given limiter and starts its background loops. The caller
// must only call it when the feature is enabled (conf non-nil, File non-empty); the Service does
// not model a disabled state.
func NewService(conf *Config, limiter *limit.Service) *Service {
	s := &Service{
		conf:      conf,
		limiter:   limiter,
		emitted:   make(map[netip.Prefix]time.Time),
		closeChan: make(chan struct{}),
		writeDone: make(chan struct{}),
	}
	go s.runPruneLoop()
	go s.runWriteLoop()
	return s
}

// Key returns the limiter key of the prefix's strike bucket
func Key(prefix netip.Prefix) string {
	return keyPrefix + prefix.String()
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
	if s.limiter.Allow(Key(prefix), s.rule(), int64(weight)).OK {
		return // Within the strike budget, no breach
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if emitted, ok := s.emitted[prefix]; ok && now.Sub(emitted) < s.conf.Window {
		return // Already emitted this prefix within the window (one ban line per prefix per window)
	}
	s.emitted[prefix] = now
	s.pending = append(s.pending, formatBanLine(now, ip, prefix, httpCode, errorCode))
}

// rule is the strike bucket: Threshold weighted strikes, refilling over the Window
func (s *Service) rule() limit.Rule {
	return limit.Rule{Kind: limit.KindStrikes, Burst: int64(s.conf.Threshold), Refill: s.conf.Window / time.Duration(s.conf.Threshold)}
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

// prune forgets emits older than a full Window -- the throttle has run out, so forgetting them is a
// no-op that bounds memory under a flood of distinct IPs. The strike buckets are the limiter's,
// which prunes its own idle keys.
func (s *Service) prune() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for prefix, emitted := range s.emitted {
		if now.Sub(emitted) >= s.conf.Window {
			delete(s.emitted, prefix)
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

package user

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
)

// accessCache is an in-memory index over the entire user_access table.
//
// exact[username][escapedTopic] returns the matching entry in O(1) for the common
// case where the requested topic appears verbatim in some rule. The key is the
// stored form of the topic (i.e. with \_ escapes), so Lookup escapes incoming
// topics through escapeUnderscore before probing.
//
// pattern[username] is the linear-scan list of %-bearing rules for that user.
// Walked per request; trivially small in practice. Wildcards are NOT u_everyone-
// only -- any user can create them.
//
// Reload reloads at once; RequestReload asks the background loop (see Start) for a coalesced
// reload. Either way, the changed entries are reported to changed.
type accessCache struct {
	db             *db.DB                 // Reloads read the primary, so a reload after a write sees the just-written rows
	query          string                 // Loads every (user_name, topic, read, write) row
	changed        AccessCacheChangedFunc // Told about the entries a reload found changed; may be nil
	exact          map[string]map[string]aclEntry
	pattern        map[string][]aclEntry
	reloadRequests chan struct{} // Pending reload request, at most one (see RequestReload)
	quit           chan struct{} // Closed by Close to stop the reload loop
	reloadMu       sync.Mutex    // Serializes reloads, so a reload that scanned earlier never replaces a later one
	mu             sync.RWMutex  // Protects exact and pattern
}

// testHookReloadScanned, if non-nil, is invoked by reload after the DB scan but
// before the result is applied. Tests use it to inject a concurrent mutation
// into the full-reload race window; it is always nil in production.
var testHookReloadScanned func()

// aclEntry mirrors one user_access row. length feeds better()'s "longer
// pattern wins" tie-break. escapedTopicPattern is the stored topic or pattern; it
// is only used to report changed entries after a reload, never for lookups
// (surfacing wildcard "topics" like "up%" alongside real ones would invite
// misuse). pattern is the compiled regex form of the LIKE pattern; nil for
// exact entries.
type aclEntry struct {
	escapedTopicPattern string
	length              int
	pattern             *regexp.Regexp
	read                bool
	write               bool
}

func newAccessCache(d *db.DB, query string, changed AccessCacheChangedFunc) *accessCache {
	return &accessCache{
		db:             d,
		query:          query,
		changed:        changed,
		exact:          make(map[string]map[string]aclEntry),
		pattern:        make(map[string][]aclEntry),
		reloadRequests: make(chan struct{}, 1),
		quit:           make(chan struct{}),
	}
}

// Start loads the cache, then reloads it every interval and on RequestReload until Close. The
// initial load finds every entry new; that is no change, so nothing is reported.
func (c *accessCache) Start(interval time.Duration) error {
	if _, err := c.reload(); err != nil {
		return err
	}
	go c.asyncReloadLoop(interval)
	return nil
}

// Close stops the reload loop
func (c *accessCache) Close() {
	close(c.quit)
}

// Lookup returns the effective (read, write, found) permission for the given
// (username, topic), preserving the priority ordering of the original SQL query:
//  1. specific user beats Everyone
//  2. longer pattern beats shorter (more specific wins)
//  3. write beats read at equal length (write is "stronger")
func (c *accessCache) Lookup(username, topic string) (read, write, found bool) {
	escapedTopic := escapeUnderscore(topic)
	c.mu.RLock()
	if username != Everyone {
		if entry, found := c.lookupNoLock(username, topic, escapedTopic); found {
			c.mu.RUnlock()
			maybeLogACLDecision(username, username, topic, entry.read, entry.write)
			return entry.read, entry.write, true
		}
	}
	if entry, found := c.lookupNoLock(Everyone, topic, escapedTopic); found {
		c.mu.RUnlock()
		maybeLogACLDecision(username, Everyone, topic, entry.read, entry.write)
		return entry.read, entry.write, true
	}
	c.mu.RUnlock()
	maybeLogACLDecision(username, "", topic, false, false)
	return false, false, false
}

// Reload reloads the whole cache from the database now (about 15 ms for ntfy.sh's 5,500 entries,
// so there is no point in reloading parts of it), and reports the entries it found changed to
// c.changed before it returns. The callback runs after the reload released reloadMu, so it may
// write access entries, which reloads again.
func (c *accessCache) Reload() error {
	changes, err := c.reload()
	if err != nil {
		return err
	}
	if c.changed != nil && len(changes) > 0 {
		c.changed(changes)
	}
	return nil
}

// RequestReload asks for a reload soon, e.g. because another cluster node changed access entries,
// and returns at once. Requests that arrive while a reload runs collapse into one more reload, so
// a burst of them costs at most two, and the last one sees every change.
func (c *accessCache) RequestReload() {
	select {
	case c.reloadRequests <- struct{}{}:
	default: // A reload is pending already; it will see this change too
	}
}

// asyncReloadLoop reloads every interval, so that writes made by other processes against the
// same database (most notably the `ntfy access` CLI, or a peer whose signal was lost) become
// visible within it, and serves RequestReload
func (c *accessCache) asyncReloadLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.quit:
			return
		case <-ticker.C:
		case <-c.reloadRequests:
		}
		if err := c.Reload(); err != nil {
			log.Tag(tag).Err(err).Warn("Reloading ACL cache failed")
		}
	}
}

// reload replaces the cache with every access entry in the database and returns the entries
// that differ from before (added, removed, or with other permissions), so that the caller can
// re-check the live subscriptions they affect. Reloads are serialized: one that scanned earlier
// must never replace one that scanned later.
func (c *accessCache) reload() ([]*AccessChange, error) {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()
	started := time.Now()
	rows, err := c.db.Query(c.query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	exacts := make(map[string]map[string]aclEntry)
	patterns := make(map[string][]aclEntry)
	updatedEntries := 0
	for rows.Next() {
		var username, escapedTopic string
		var read, write bool
		if err := rows.Scan(&username, &escapedTopic, &read, &write); err != nil {
			return nil, err
		}
		entry, hasWildcard, err := toACLEntry(escapedTopic, read, write)
		if err != nil {
			return nil, err
		}
		if hasWildcard {
			patterns[username] = append(patterns[username], entry)
		} else {
			if exacts[username] == nil {
				exacts[username] = make(map[string]aclEntry)
			}
			exacts[username][escapedTopic] = entry
		}
		updatedEntries++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if testHookReloadScanned != nil {
		testHookReloadScanned()
	}
	// Diff without c.mu, so lookups do not wait for it: only reloads replace the maps, and this one
	// holds reloadMu. Then replace the maps.
	changes := diffACLEntries(c.exact, c.pattern, exacts, patterns)
	c.mu.Lock()
	c.exact = exacts
	c.pattern = patterns
	c.mu.Unlock()
	log.Tag(tag).
		Field("updated_entries", updatedEntries).
		Field("changed_entries", len(changes)).
		Field("duration_ms", time.Since(started).Milliseconds()).
		Debug("ACL cache reloaded")
	return changes, nil
}

// diffACLEntries returns the entries that differ between the old and the new maps: added,
// removed, or with a different read/write permission
func diffACLEntries(oldExact map[string]map[string]aclEntry, oldPattern map[string][]aclEntry, newExact map[string]map[string]aclEntry, newPattern map[string][]aclEntry) []*AccessChange {
	before := flattenACLEntries(oldExact, oldPattern)
	after := flattenACLEntries(newExact, newPattern)
	changes := make([]*AccessChange, 0)
	for key, entry := range after {
		if old, ok := before[key]; !ok || old.read != entry.read || old.write != entry.write {
			changes = append(changes, newAccessChange(key))
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			changes = append(changes, newAccessChange(key))
		}
	}
	return changes
}

// aclKey identifies one access entry
type aclKey struct {
	username            string
	escapedTopicPattern string
}

// flattenACLEntries indexes every exact and wildcard entry by user and stored topic
func flattenACLEntries(exact map[string]map[string]aclEntry, pattern map[string][]aclEntry) map[aclKey]aclEntry {
	entries := make(map[aclKey]aclEntry)
	for username, byTopic := range exact {
		for escapedTopic, entry := range byTopic {
			key := aclKey{
				username:            username,
				escapedTopicPattern: escapedTopic,
			}
			entries[key] = entry
		}
	}
	for username, patterns := range pattern {
		for _, entry := range patterns {
			key := aclKey{
				username:            username,
				escapedTopicPattern: entry.escapedTopicPattern,
			}
			entries[key] = entry
		}
	}
	return entries
}

// newAccessChange returns the change for one entry, with * wildcards
func newAccessChange(key aclKey) *AccessChange {
	return &AccessChange{
		Username:     key.username,
		TopicPattern: fromSQLWildcard(key.escapedTopicPattern),
	}
}

// lookupNoLock returns the highest-priority entry for a single user. When
// more than one of that user's rules matches the requested topic, the winner
// is chosen by:
//
//  1. longer stored pattern beats shorter (a more specific rule wins over a
//     more general one)
//  2. at equal length, write beats read (a stronger permission wins the tie)
//
// Exact and wildcard rules are ranked together under the same criteria, so
// an exact "foo" (length 3) beats a wildcard "f%" (length 2), but a wildcard
// "foo%" (length 4) beats an exact "foo" (length 3).
func (c *accessCache) lookupNoLock(username, topic, escapedTopic string) (*aclEntry, bool) {
	var best aclEntry
	var found bool
	if exact, exists := c.exact[username]; exists {
		if entry, exists := exact[escapedTopic]; exists {
			best, found = entry, true
		}
	}
	for _, pattern := range c.pattern[username] {
		if !pattern.pattern.MatchString(topic) {
			continue
		} else if !found || better(pattern, best) {
			best, found = pattern, true
		}
	}
	return &best, found
}

// toACLEntry builds an aclEntry from one user_access row's values. The
// isWildcard return tells the caller which storage slot the entry belongs in:
// the per-user wildcard slice if true, the per-user exact map if false.
// Wildcards have their LIKE pattern pre-compiled into entry.pattern; exact
// entries leave entry.pattern nil.
func toACLEntry(escapedTopic string, read, write bool) (entry aclEntry, hasWildcard bool, err error) {
	entry = aclEntry{
		escapedTopicPattern: escapedTopic,
		length:              len(escapedTopic),
		read:                read,
		write:               write,
	}
	if !strings.Contains(escapedTopic, "%") {
		return entry, false, nil
	}
	pattern, err := compileLikeToRegex(escapedTopic)
	if err != nil {
		return entry, true, err
	}
	entry.pattern = pattern
	return entry, true, nil
}

// better implements the (length DESC, write DESC) tie-break used by the original
// query's ORDER BY for entries owned by the same user.
func better(a, b aclEntry) bool {
	if a.length != b.length {
		return a.length > b.length
	} else if a.write != b.write {
		return a.write
	}
	return false
}

// compileLikeToRegex converts a stored ntfy LIKE pattern into an equivalent Go
// regexp. In ntfy's stored form, % is the only wildcard (translated from *) and
// \_ is a literal underscore; no other backslashes occur. Topics themselves are
// restricted to [A-Za-z0-9_-] (see AllowedTopic), so neither % nor stray
// backslashes appear in user-supplied input.
func compileLikeToRegex(pattern string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	i := 0
	for i < len(pattern) {
		switch {
		case pattern[i] == '\\' && i+1 < len(pattern) && pattern[i+1] == '_':
			sb.WriteString(regexp.QuoteMeta("_"))
			i += 2
		case pattern[i] == '%':
			sb.WriteString(".*")
			i++
		default:
			sb.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}

// maybeLogACLDecision logs an ACL lookup result
func maybeLogACLDecision(requestUser, matchedUser, topic string, read, write bool) {
	ev := log.Tag(tag).
		Field("user_name", requestUser).
		Field("topic", topic).
		Field("read", read).
		Field("write", write)
	if !ev.IsTrace() {
		return
	}
	if matchedUser == "" {
		ev.Trace("ACL no match")
		return
	}
	ev.Field("matched_user", matchedUser).Trace("ACL match")
}

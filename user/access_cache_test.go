package user

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// Cache-only unit tests. Integration with the Manager (loading from the DB,
// reload-after-mutation, end-to-end Authorize behavior) is covered by the
// existing TestStoreAuthorizeTopicAccess* tests in manager_test.go via
// forEachStoreBackend.

func TestCompileLikeToRegex_Exact(t *testing.T) {
	r := mustCompileLikeToRegex(t, "foo")
	require.True(t, r.MatchString("foo"))
	require.False(t, r.MatchString("foox"))
	require.False(t, r.MatchString("xfoo"))
}

func TestCompileLikeToRegex_TrailingPercent(t *testing.T) {
	r := mustCompileLikeToRegex(t, "up%")
	require.True(t, r.MatchString("up"))
	require.True(t, r.MatchString("up123"))
	require.False(t, r.MatchString("xup"))
}

func TestCompileLikeToRegex_LeadingAndEmbeddedPercent(t *testing.T) {
	r := mustCompileLikeToRegex(t, "%test%")
	require.True(t, r.MatchString("test"))
	require.True(t, r.MatchString("mytest"))
	require.True(t, r.MatchString("testxxx"))
	require.True(t, r.MatchString("xtestx"))
	require.False(t, r.MatchString("nope"))
}

func TestCompileLikeToRegex_EscapedUnderscore(t *testing.T) {
	// "my\_topic" is the stored form of a literal "my_topic" -- the underscore
	// must match itself, NOT act as a SQL one-character wildcard.
	r := mustCompileLikeToRegex(t, `my\_topic`)
	require.True(t, r.MatchString("my_topic"))
	require.False(t, r.MatchString("myXtopic"))
	require.False(t, r.MatchString("mytopic"))
}

func TestCompileLikeToRegex_EscapedUnderscoreAdjacentToPercent(t *testing.T) {
	// "nz\_vip\_%" is the stored form of "nz_vip_*" -- literal "nz_vip_" prefix
	// followed by any suffix.
	r := mustCompileLikeToRegex(t, `nz\_vip\_%`)
	require.True(t, r.MatchString("nz_vip_"))
	require.True(t, r.MatchString("nz_vip_alpha"))
	require.False(t, r.MatchString("nz_vipX"))
	require.False(t, r.MatchString("nzvip_alpha"))
}

func TestCompileLikeToRegex_RegexMetaCharsInTopic(t *testing.T) {
	// Topics in ntfy can include '-', which is benign, but make sure
	// regex metacharacters in the pattern are escaped properly anyway.
	r := mustCompileLikeToRegex(t, "foo-bar")
	require.True(t, r.MatchString("foo-bar"))
	require.False(t, r.MatchString("foo.bar")) // would match if '-' leaked into a character class
}

func TestACLCache_LookupBeforeReload(t *testing.T) {
	// A freshly-constructed cache has empty exact and wildcards maps. The
	// cache treats this as "no rule found", which the caller resolves via
	// DefaultAccess.
	c := newAccessCache()
	read, write, found := c.Lookup("phil", "mytopic")
	require.False(t, found)
	require.False(t, read)
	require.False(t, write)
}

func TestACLCache_ExactMatchHit(t *testing.T) {
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: "phil", topic: "mytopic", read: true, write: true},
	})
	read, write, found := c.Lookup("phil", "mytopic")
	require.True(t, found)
	require.True(t, read)
	require.True(t, write)
}

func TestACLCache_ExactMatchMiss(t *testing.T) {
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: "phil", topic: "mytopic", read: true, write: true},
	})
	_, _, found := c.Lookup("phil", "othertopic")
	require.False(t, found)
}

func TestACLCache_LiteralUnderscoreExactMatch(t *testing.T) {
	// Stored as "my\_topic" (toSQLWildcard of "my_topic"). A literal underscore
	// in the requested topic must match, while any other single char must not.
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: "phil", topic: `my\_topic`, read: true, write: false},
	})
	read, write, found := c.Lookup("phil", "my_topic")
	require.True(t, found)
	require.True(t, read)
	require.False(t, write)

	_, _, found = c.Lookup("phil", "myXtopic")
	require.False(t, found)
}

func TestACLCache_WildcardMatch(t *testing.T) {
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: Everyone, topic: "up%", read: false, write: true},
	})
	read, write, found := c.Lookup("phil", "up42")
	require.True(t, found)
	require.False(t, read)
	require.True(t, write)
}

func TestACLCache_SpecificUserBeatsEveryone(t *testing.T) {
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: Everyone, topic: "mytopic", read: true, write: false},
		{user: "phil", topic: "mytopic", read: false, write: false}, // deny-all for phil
	})
	read, write, found := c.Lookup("phil", "mytopic")
	require.True(t, found)
	require.False(t, read)
	require.False(t, write)
}

func TestACLCache_SpecificUserBeatsEveryoneEvenWhenShorter(t *testing.T) {
	// The SQL's "user_name DESC" sort key takes precedence over LENGTH(topic).
	// Concretely: a specific user with a shorter matching rule still wins over
	// Everyone with a longer matching rule.
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: Everyone, topic: "foo", read: true, write: true}, // exact, length 3
		{user: "phil", topic: "f%", read: false, write: false},  // wildcard, length 2, deny-all
	})
	read, write, found := c.Lookup("phil", "foo")
	require.True(t, found)
	require.False(t, read)
	require.False(t, write)
}

func TestACLCache_SpecificUserBeatsEveryoneRegardlessOfWrite(t *testing.T) {
	// Same-length rules but conflicting permissions across user boundary: the
	// specific user always wins, even if its permission set is weaker (or
	// stronger, in either direction).
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: Everyone, topic: "mytopic", read: true, write: true}, // wide-open
		{user: "phil", topic: "mytopic", read: true, write: false},  // read-only for phil
	})
	read, write, found := c.Lookup("phil", "mytopic")
	require.True(t, found)
	require.True(t, read)
	require.False(t, write)
}

func TestACLCache_AnonymousReadsEveryone(t *testing.T) {
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: Everyone, topic: "announcements", read: true, write: false},
	})
	read, write, found := c.Lookup(Everyone, "announcements")
	require.True(t, found)
	require.True(t, read)
	require.False(t, write)
}

func TestACLCache_LongerPatternWinsForSameUser(t *testing.T) {
	// Both rules belong to the same user (Everyone). The more specific (longer)
	// "mytopic%" should beat the catch-all "%".
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: Everyone, topic: "%", read: true, write: false},
		{user: Everyone, topic: "mytopic%", read: true, write: true},
	})
	read, write, found := c.Lookup(Everyone, "mytopicX")
	require.True(t, found)
	require.True(t, read)
	require.True(t, write)
}

func TestACLCache_ExactBeatsShorterWildcardSameUser(t *testing.T) {
	// Same user, two matching rules: exact "foo" (length 3) and wildcard "f%"
	// (length 2). The longer one wins, which is the exact rule -- mirroring
	// the SQL's "LENGTH(topic) DESC" tie-break. Crucially, the cache must seed
	// "best" from the exact map probe before walking wildcards, otherwise a
	// shorter wildcard could overwrite a longer exact.
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: "phil", topic: "foo", read: true, write: true},  // exact, length 3
		{user: "phil", topic: "f%", read: false, write: false}, // wildcard, length 2, deny-all
	})
	read, write, found := c.Lookup("phil", "foo")
	require.True(t, found)
	require.True(t, read)
	require.True(t, write)
}

func TestACLCache_LongerWildcardBeatsExactSameUser(t *testing.T) {
	// Same user, two matching rules: exact "foo" (length 3) and wildcard "foo%"
	// (length 4). The wildcard wins on length DESC. Exercises the "swap best
	// to wildcard when better() returns true" path.
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: "phil", topic: "foo", read: false, write: false}, // exact, length 3, deny-all
		{user: "phil", topic: "foo%", read: true, write: true},  // wildcard, length 4
	})
	read, write, found := c.Lookup("phil", "foo")
	require.True(t, found)
	require.True(t, read)
	require.True(t, write)
}

func TestACLCache_WriteBeatsReadAtEqualLength(t *testing.T) {
	// Two wildcard rules of identical length for the same user. The write rule
	// should win the tie-break. The two-rows-with-same-topic shape is
	// impossible via real upsert (pkey would conflict), so we inject the entries
	// directly into the cache's wildcard slice.
	c := newAccessCache()
	c.mu.Lock()
	c.exact = map[string]map[string]aclEntry{}
	c.pattern = map[string][]aclEntry{
		Everyone: {
			{length: len("ab%"), read: true, write: false, pattern: mustCompileLikeToRegex(t, "ab%")},
			{length: len("ab%"), read: false, write: true, pattern: mustCompileLikeToRegex(t, "ab%")},
		},
	}
	c.mu.Unlock()
	_, write, found := c.Lookup(Everyone, "abc")
	require.True(t, found)
	require.True(t, write)
}

func TestACLCache_ConcurrentLookupAndReload(t *testing.T) {
	// Lock-based swap must be safe under concurrent reads. The race detector
	// catches any unsafe shared mutation.
	c := newAccessCache()
	loadCache(t, c, []rawACLRow{
		{user: Everyone, topic: "mytopic", read: true, write: true},
	})

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			_, _, _ = c.Lookup(Everyone, "mytopic")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			loadCache(t, c, []rawACLRow{
				{user: Everyone, topic: "mytopic", read: i%2 == 0, write: i%2 == 1},
			})
		}
		stop.Store(true)
	}()
	wg.Wait()
}

// rawACLRow models the rows that reload would Scan from the DB but avoids
// actually opening a DB for these unit tests.
type rawACLRow struct {
	user  string
	topic string
	read  bool
	write bool
}

// loadCache writes the given rows into the cache under its write lock,
// preserving the same exact/wildcard partitioning that reload would produce.
func loadCache(t *testing.T, c *accessCache, rows []rawACLRow) {
	t.Helper()
	exact := make(map[string]map[string]aclEntry)
	wildcards := make(map[string][]aclEntry)
	for _, r := range rows {
		e := aclEntry{length: len(r.topic), read: r.read, write: r.write}
		if strings.Contains(r.topic, "%") {
			e.pattern = mustCompileLikeToRegex(t, r.topic)
			wildcards[r.user] = append(wildcards[r.user], e)
		} else {
			if exact[r.user] == nil {
				exact[r.user] = make(map[string]aclEntry)
			}
			exact[r.user][r.topic] = e
		}
	}
	c.mu.Lock()
	c.exact = exact
	c.pattern = wildcards
	c.mu.Unlock()
}

func mustCompileLikeToRegex(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	r, err := compileLikeToRegex(pattern)
	require.NoError(t, err)
	return r
}

package attachment

import (
	"context"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/s3"
	"heckel.io/ntfy/v2/util"
)

const (
	// testRoot is the shared root under which every test's key space lives, so that one listing
	// can find what previous runs left behind
	testRoot = "testpkg-attachment"

	// staleTestObjectAge is how old a test object has to be before it is assumed to belong to a
	// run that died: a test takes seconds, a CI job minutes, so nothing live is this old
	staleTestObjectAge = 2 * time.Hour
)

var (
	// testRunID separates concurrent runs that share the test bucket
	testRunID = util.RandomString(10)

	// sweepOnce runs the stale-object sweep once per test binary
	sweepOnce sync.Once
)

// testKeyPrefix returns a key space belonging to this test alone. The bucket is shared with
// every other CI run, and deleteAllObjects wipes everything under the prefix it is given, so
// tests sharing one prefix delete each other's objects mid-run (seen as NoSuchKey 404s on an
// unrelated pull request).
func testKeyPrefix(t *testing.T, configuredPrefix string) string {
	return path.Join(configuredPrefix, testRoot, testRunID, t.Name())
}

// sweepStaleTestObjects deletes objects left behind by runs that were killed before their
// cleanup ran. Per-run key spaces keep concurrent runs from deleting each other's objects, but
// they also mean nothing ever revisits a dead run's keys, so something has to collect them.
// Anything under the shared test root older than staleTestObjectAge cannot belong to a live run.
//
// This is a backstop for a bucket that has no expiration rule. Prefer a lifecycle rule on the
// bucket (expire the test root after a day): that also covers the case where nobody runs these
// tests for a while, and it cannot be skipped by a crash.
func sweepStaleTestObjects(t *testing.T, config *s3.Config) {
	t.Helper()
	rootConfig := *config // The sweep looks at every run's keys, not just this test's
	rootConfig.Prefix = path.Join(config.Prefix, testRoot)
	client := s3.New(&rootConfig)
	objects, err := client.ListObjectsV2(context.Background())
	if err != nil {
		t.Logf("cannot list stale test objects, skipping sweep: %s", err) // Hygiene, not the test
		return
	}
	stale := staleKeys(objects, time.Now())
	if len(stale) == 0 {
		return
	}
	t.Logf("deleting %d object(s) left behind by earlier runs", len(stale))
	if err := client.DeleteObjects(context.Background(), stale); err != nil {
		t.Logf("cannot delete stale test objects: %s", err)
	}
}

// staleKeys returns the keys of objects old enough to belong to a run that died. An object whose
// modification time could not be read is left alone: a zero time would otherwise look infinitely
// old and take every live run's objects with it.
func staleKeys(objects []*s3.Object, now time.Time) []string {
	stale := make([]string, 0)
	for _, obj := range objects {
		if !obj.LastModified.IsZero() && now.Sub(obj.LastModified) > staleTestObjectAge {
			stale = append(stale, obj.Key)
		}
	}
	return stale
}

func TestStaleKeys_OnlyCollectsWhatNoLiveRunCouldOwn(t *testing.T) {
	// The sweep runs while other CI jobs are mid-test, so the age cutoff is the only thing
	// keeping it from deleting a live run's objects
	now := time.Now()
	objects := []*s3.Object{
		{Key: "live/just-written", LastModified: now},
		{Key: "live/minutes-old", LastModified: now.Add(-10 * time.Minute)},
		{Key: "dead/hours-old", LastModified: now.Add(-3 * time.Hour)},
		{Key: "dead/days-old", LastModified: now.Add(-72 * time.Hour)},
	}
	require.Equal(t, []string{"dead/hours-old", "dead/days-old"}, staleKeys(objects, now))
}

func TestStaleKeys_IgnoresObjectsWithoutAModTime(t *testing.T) {
	// LastModified is parsed from the listing and left zero if it could not be read; deleting on
	// a zero time would delete everything
	objects := []*s3.Object{{Key: "unknown-age", LastModified: time.Time{}}}
	require.Empty(t, staleKeys(objects, time.Now()))
}

func TestS3Store_WriteWithPrefix(t *testing.T) {
	s3URL := os.Getenv("NTFY_TEST_S3_URL")
	if s3URL == "" {
		t.Skip("NTFY_TEST_S3_URL not set")
	}
	cfg, err := s3.ParseURL(s3URL)
	require.Nil(t, err)
	sweepOnce.Do(func() { sweepStaleTestObjects(t, cfg) })
	cfg.Prefix = testKeyPrefix(t, cfg.Prefix) // The key space under test IS a prefix
	client := s3.New(cfg)
	deleteAllObjects(t, client)
	backend := newS3Backend(client)
	cache, err := newStore(backend, 10*1024, time.Hour, nil)
	require.Nil(t, err)
	t.Cleanup(func() {
		deleteAllObjects(t, client)
		cache.Close()
	})

	size, err := cache.Write("abcdefghijkl", strings.NewReader("test"), 0)
	require.Nil(t, err)
	require.Equal(t, int64(4), size)

	reader, _, err := cache.Read("abcdefghijkl")
	require.Nil(t, err)
	data, err := io.ReadAll(reader)
	reader.Close()
	require.Nil(t, err)
	require.Equal(t, "test", string(data))
}

// --- Helpers ---

func newTestRealS3Store(t *testing.T, totalSizeLimit int64) (*Store, *modTimeOverrideBackend) {
	t.Helper()
	s3URL := os.Getenv("NTFY_TEST_S3_URL")
	if s3URL == "" {
		t.Skip("NTFY_TEST_S3_URL not set")
	}
	cfg, err := s3.ParseURL(s3URL)
	require.Nil(t, err)
	sweepOnce.Do(func() { sweepStaleTestObjects(t, cfg) })
	cfg.Prefix = testKeyPrefix(t, cfg.Prefix)
	client := s3.New(cfg)
	inner := newS3Backend(client)
	wrapper := &modTimeOverrideBackend{backend: inner, modTimes: make(map[string]time.Time)}
	deleteAllObjects(t, client)
	store, err := newStore(wrapper, totalSizeLimit, time.Hour, nil)
	require.Nil(t, err)
	t.Cleanup(func() {
		deleteAllObjects(t, client)
		store.Close()
	})
	return store, wrapper
}

func deleteAllObjects(t *testing.T, client *s3.Client) {
	t.Helper()
	for i := 0; i < 20; i++ {
		objects, err := client.ListObjectsV2(context.Background())
		require.Nil(t, err)
		if len(objects) == 0 {
			return
		}
		keys := make([]string, len(objects))
		for j, obj := range objects {
			keys[j] = obj.Key
		}
		require.Nil(t, client.DeleteObjects(context.Background(), keys))
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("timed out waiting for bucket to be empty")
}

// modTimeOverrideBackend wraps a backend and allows overriding LastModified times returned by List().
// This is used in tests to simulate old objects on backends (like real S3) where
// LastModified cannot be set directly.
type modTimeOverrideBackend struct {
	backend
	mu       sync.Mutex
	modTimes map[string]time.Time // object ID -> override time
}

func (b *modTimeOverrideBackend) List() ([]object, error) {
	objects, err := b.backend.List()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, obj := range objects {
		if t, ok := b.modTimes[obj.ID]; ok {
			objects[i].LastModified = t
		}
	}
	return objects, nil
}

func (b *modTimeOverrideBackend) setModTime(id string, t time.Time) {
	b.mu.Lock()
	b.modTimes[id] = t
	b.mu.Unlock()
}

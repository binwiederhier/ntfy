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

// testRunID separates concurrent runs that share the test bucket
var testRunID = util.RandomString(10)

// testKeyPrefix returns a key space belonging to this test alone. The bucket is shared with
// every other CI run, and deleteAllObjects wipes everything under the prefix it is given, so
// tests sharing one prefix delete each other's objects mid-run (seen as NoSuchKey 404s on an
// unrelated pull request).
func testKeyPrefix(t *testing.T, configuredPrefix string) string {
	return path.Join(configuredPrefix, "testpkg-attachment", testRunID, t.Name())
}

func TestS3Store_WriteWithPrefix(t *testing.T) {
	s3URL := os.Getenv("NTFY_TEST_S3_URL")
	if s3URL == "" {
		t.Skip("NTFY_TEST_S3_URL not set")
	}
	cfg, err := s3.ParseURL(s3URL)
	require.Nil(t, err)
	cfg.Prefix = testKeyPrefix(t, "test-prefix")
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

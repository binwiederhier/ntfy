package attachment

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/s3"
)

func TestS3Store_WriteWithPrefix(t *testing.T) {
	s3URL := os.Getenv("NTFY_TEST_S3_URL")
	if s3URL == "" {
		t.Skip("NTFY_TEST_S3_URL not set")
	}
	cfg, err := s3.ParseURL(s3URL)
	require.Nil(t, err)
	cfg.Prefix = "test-prefix"
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
	if cfg.Prefix != "" {
		cfg.Prefix = cfg.Prefix + "/testpkg-attachment"
	} else {
		cfg.Prefix = "testpkg-attachment"
	}
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

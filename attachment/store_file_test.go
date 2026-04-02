package attachment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestFileStore(t *testing.T, totalSizeLimit int64) (dir string, cache *Store) {
	t.Helper()
	dir = t.TempDir()
	cache, err := NewFileStore(dir, totalSizeLimit, time.Hour, nil)
	require.Nil(t, err)
	t.Cleanup(func() { cache.Close() })
	return dir, cache
}

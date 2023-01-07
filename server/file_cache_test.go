package server

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/util"
	"os"
	"strings"
	"testing"
)

var (
	oneKilobyteArray = make([]byte, 1024)
)

func TestFileCache_Write_Success(t *testing.T) {
	dir, c := newTestFileCache(t)
	size, err := c.Write("abcdefghijkl", strings.NewReader("normal file"), util.NewFixedLimiter(999))
	require.Nil(t, err)
	require.Equal(t, int64(11), size)
	require.Equal(t, "normal file", readFile(t, dir+"/abcdefghijkl"))
	require.Equal(t, int64(11), c.Size())
	require.Equal(t, int64(10229), c.Remaining())
}

func TestFileCache_Write_Remove_Success(t *testing.T) {
	dir, c := newTestFileCache(t) // max = 10k (10240), each = 1k (1024)
	for i := 0; i < 10; i++ {     // 10x999 = 9990
		size, err := c.Write(fmt.Sprintf("abcdefghijk%d", i), bytes.NewReader(make([]byte, 999)))
		require.Nil(t, err)
		require.Equal(t, int64(999), size)
	}
	require.Equal(t, int64(9990), c.Size())
	require.Equal(t, int64(250), c.Remaining())
	require.FileExists(t, dir+"/abcdefghijk1")
	require.FileExists(t, dir+"/abcdefghijk5")

	require.Nil(t, c.Remove("abcdefghijk1", "abcdefghijk5"))
	require.NoFileExists(t, dir+"/abcdefghijk1")
	require.NoFileExists(t, dir+"/abcdefghijk5")
	require.Equal(t, int64(7992), c.Size())
	require.Equal(t, int64(2248), c.Remaining())
}

func TestFileCache_Write_FailedTotalSizeLimit(t *testing.T) {
	dir, c := newTestFileCache(t)
	for i := 0; i < 10; i++ {
		size, err := c.Write(fmt.Sprintf("abcdefghijk%d", i), bytes.NewReader(oneKilobyteArray))
		require.Nil(t, err)
		require.Equal(t, int64(1024), size)
	}
	_, err := c.Write("abcdefghijkX", bytes.NewReader(oneKilobyteArray))
	require.Equal(t, util.ErrLimitReached, err)
	require.NoFileExists(t, dir+"/abcdefghijkX")
}

func TestFileCache_Write_FailedAdditionalLimiter(t *testing.T) {
	dir, c := newTestFileCache(t)
	_, err := c.Write("abcdefghijkl", bytes.NewReader(make([]byte, 1001)), util.NewFixedLimiter(1000))
	require.Equal(t, util.ErrLimitReached, err)
	require.NoFileExists(t, dir+"/abcdefghijkl")
}

func newTestFileCache(t *testing.T) (dir string, cache *fileCache) {
	dir = t.TempDir()
	cache, err := newFileCache(dir, 10*1024)
	require.Nil(t, err)
	return dir, cache
}

func readFile(t *testing.T, f string) string {
	b, err := os.ReadFile(f)
	require.Nil(t, err)
	return string(b)
}

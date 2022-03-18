package util

import (
	"embed"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var (
	modTime = time.Now()

	//go:embed embedfs
	testFs       embed.FS
	testFsCached = &CachingEmbedFS{ModTime: modTime, FS: testFs}
)

func TestCachingEmbedFS(t *testing.T) {
	s := http.FileServer(http.FS(testFsCached))

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/embedfs/test.txt", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)
	lastModified := rr.Header().Get("Last-Modified")

	rr = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/embedfs/test.txt", nil)
	req.Header.Set("If-Modified-Since", lastModified)
	s.ServeHTTP(rr, req)
	require.Equal(t, 304, rr.Code) // Huzzah!
}

func TestCachingEmbedFS_Range(t *testing.T) {
	s := http.FileServer(http.FS(testFsCached))
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/embedfs/test.txt", nil)
	req.Header.Set("Range", "bytes=1-20")
	s.ServeHTTP(rr, req)
	require.Equal(t, 206, rr.Code)
	require.Equal(t, "his is a test file f", rr.Body.String())
}

package util

import (
	"compress/gzip"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGzipHandler(t *testing.T) {
	s := Gzip(http.FileServer(http.FS(testFs)))

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/embedfs/test.txt", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	s.ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "gzip", rr.Header().Get("Content-Encoding"))
	require.Equal(t, "", rr.Header().Get("Content-Length"))

	gz, _ := gzip.NewReader(rr.Body)
	b, _ := io.ReadAll(gz)
	require.Equal(t, "This is a test file for embedfs_test.go\n", string(b))
}

func TestGzipHandler_NoGzip(t *testing.T) {
	s := Gzip(http.FileServer(http.FS(testFs)))

	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/embedfs/test.txt", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, 200, rr.Code)
	require.Equal(t, "", rr.Header().Get("Content-Encoding"))
	require.Equal(t, "40", rr.Header().Get("Content-Length"))

	b, _ := io.ReadAll(rr.Body)
	require.Equal(t, "This is a test file for embedfs_test.go\n", string(b))
}

package util

import (
	"crypto/rand"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestSniffWriter_WriteHTML(t *testing.T) {
	rr := httptest.NewRecorder()
	sw := NewContentTypeWriter(rr, "")
	sw.Write([]byte("<script>alert('hi')</script>"))
	require.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
}

func TestSniffWriter_WriteTwoWriteCalls(t *testing.T) {
	rr := httptest.NewRecorder()
	sw := NewContentTypeWriter(rr, "")
	sw.Write([]byte{0x25, 0x50, 0x44, 0x46, 0x2d, 0x11, 0x22, 0x33})
	sw.Write([]byte("<script>alert('hi')</script>"))
	require.Equal(t, "application/pdf", rr.Header().Get("Content-Type"))
}

func TestSniffWriter_NoSniffWriterWriteHTML(t *testing.T) {
	// This test just makes sure that without the sniff-w, we would get text/html

	rr := httptest.NewRecorder()
	rr.Write([]byte("<script>alert('hi')</script>"))
	require.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
}

func TestSniffWriter_WriteHTMLSplitIntoTwoWrites(t *testing.T) {
	// This test shows how splitting the HTML into two Write() calls will still yield text/plain

	rr := httptest.NewRecorder()
	sw := NewContentTypeWriter(rr, "")
	sw.Write([]byte("<scr"))
	sw.Write([]byte("ipt>alert('hi')</script>"))
	require.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
}

func TestSniffWriter_WriteUnknownMimeType(t *testing.T) {
	rr := httptest.NewRecorder()
	sw := NewContentTypeWriter(rr, "")
	randomBytes := make([]byte, 199)
	rand.Read(randomBytes[5:]) // Start at an offset; the test kept failing randomly because it hit random magic strings
	sw.Write(randomBytes)
	require.Equal(t, "application/octet-stream", rr.Header().Get("Content-Type"))
}

func TestSniffWriter_WriteWithFilenameAPK(t *testing.T) {
	rr := httptest.NewRecorder()
	sw := NewContentTypeWriter(rr, "https://example.com/ntfy.apk")
	sw.Write([]byte{0x50, 0x4B, 0x03, 0x04})
	require.Equal(t, "application/vnd.android.package-archive", rr.Header().Get("Content-Type"))
}

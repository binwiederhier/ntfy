package util

import (
	"net/http"
	"strings"
)

// ContentTypeWriter is an implementation of http.ResponseWriter that will detect the content type and set the
// Content-Type and (optionally) Content-Disposition headers accordingly.
//
// It will always set a Content-Type based on http.DetectContentType, but will never send the "text/html"
// content type.
type ContentTypeWriter struct {
	w        http.ResponseWriter
	filename string
	sniffed  bool
}

// NewContentTypeWriter creates a new ContentTypeWriter
func NewContentTypeWriter(w http.ResponseWriter, filename string) *ContentTypeWriter {
	return &ContentTypeWriter{w, filename, false}
}

func (w *ContentTypeWriter) Write(p []byte) (n int, err error) {
	if w.sniffed {
		return w.w.Write(p)
	}
	// Detect and set Content-Type header
	// Fix content types that we don't want to inline-render in the browser. In particular,
	// we don't want to render HTML in the browser for security reasons.
	contentType, _ := DetectContentType(p, w.filename)
	if strings.HasPrefix(contentType, "text/html") {
		contentType = strings.ReplaceAll(contentType, "text/html", "text/plain")
	} else if contentType == "application/octet-stream" {
		contentType = "" // Reset to let downstream http.ResponseWriter take care of it
	}
	if contentType != "" {
		w.w.Header().Set("Content-Type", contentType)
	}
	w.sniffed = true
	return w.w.Write(p)
}

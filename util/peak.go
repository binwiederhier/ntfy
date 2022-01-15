package util

import (
	"bytes"
	"io"
	"strings"
)

// PeakedReadCloser is a ReadCloser that allows peaking into a stream and buffering it in memory.
// It can be instantiated using the Peak function. After a stream has been peaked, it can still be fully
// read by reading the PeakedReadCloser. It first drained from the memory buffer, and then from the remaining
// underlying reader.
type PeakedReadCloser struct {
	PeakedBytes  []byte
	LimitReached bool
	peaked       io.Reader
	underlying   io.ReadCloser
	closed       bool
}

// Peak reads the underlying ReadCloser into memory up until the limit and returns a PeakedReadCloser
func Peak(underlying io.ReadCloser, limit int) (*PeakedReadCloser, error) {
	if underlying == nil {
		underlying = io.NopCloser(strings.NewReader(""))
	}
	peaked := make([]byte, limit)
	read, err := io.ReadFull(underlying, peaked)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return &PeakedReadCloser{
		PeakedBytes:  peaked[:read],
		LimitReached: read == limit,
		underlying:   underlying,
		peaked:       bytes.NewReader(peaked[:read]),
		closed:       false,
	}, nil
}

// Read reads from the peaked bytes and then from the underlying stream
func (r *PeakedReadCloser) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}
	n, err = r.peaked.Read(p)
	if err == io.EOF {
		return r.underlying.Read(p)
	} else if err != nil {
		return 0, err
	}
	return
}

// Close closes the underlying stream
func (r *PeakedReadCloser) Close() error {
	if r.closed {
		return io.EOF
	}
	r.closed = true
	return r.underlying.Close()
}

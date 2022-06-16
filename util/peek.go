package util

import (
	"bytes"
	"io"
	"strings"
)

// PeekedReadCloser is a ReadCloser that allows peeking into a stream and buffering it in memory.
// It can be instantiated using the Peek function. After a stream has been peeked, it can still be fully
// read by reading the PeekedReadCloser. It first drained from the memory buffer, and then from the remaining
// underlying reader.
type PeekedReadCloser struct {
	PeekedBytes  []byte
	LimitReached bool
	peeked       io.Reader
	underlying   io.ReadCloser
	closed       bool
}

// Peek reads the underlying ReadCloser into memory up until the limit and returns a PeekedReadCloser.
// It does not return an error if limit is reached. Instead, LimitReached will be set to true.
func Peek(underlying io.ReadCloser, limit int) (*PeekedReadCloser, error) {
	if underlying == nil {
		underlying = io.NopCloser(strings.NewReader(""))
	}
	peeked := make([]byte, limit)
	read, err := io.ReadFull(underlying, peeked)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return &PeekedReadCloser{
		PeekedBytes:  peeked[:read],
		LimitReached: read == limit,
		underlying:   underlying,
		peeked:       bytes.NewReader(peeked[:read]),
		closed:       false,
	}, nil
}

// Read reads from the peeked bytes and then from the underlying stream
func (r *PeekedReadCloser) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}
	n, err = r.peeked.Read(p)
	if err == io.EOF {
		return r.underlying.Read(p)
	} else if err != nil {
		return 0, err
	}
	return
}

// Close closes the underlying stream
func (r *PeekedReadCloser) Close() error {
	if r.closed {
		return io.EOF
	}
	r.closed = true
	return r.underlying.Close()
}

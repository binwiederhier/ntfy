package s3

import (
	"bytes"
	"errors"
	"io"
)

const (
	// partChunkSize is the allocation unit for buffering a part. Parts are buffered in chunks of this
	// size so that memory grows with the bytes actually received, not with partSize. Must divide partSize.
	partChunkSize = 64 * 1024
)

// partBuffer buffers up to partSize bytes of an upload body in partChunkSize chunks. Chunks are
// allocated only as data arrives (so a slow or stalled client pins only what it actually sent) and
// are reused across fill calls, so a multipart upload holds at most one part in memory.
type partBuffer struct {
	chunks [][]byte
	size   int // Number of buffered bytes from the last fill
}

// fill replaces the buffer contents with up to partSize bytes read from r. It returns nil if the
// buffer is full (more data may follow), io.EOF if r ended before that, or the read error.
func (b *partBuffer) fill(r io.Reader) error {
	b.size = 0
	for b.size < partSize {
		index, offset := b.size/partChunkSize, b.size%partChunkSize
		if index == len(b.chunks) {
			b.chunks = append(b.chunks, make([]byte, partChunkSize))
		}
		n, err := r.Read(b.chunks[index][offset:])
		b.size += n
		if errors.Is(err, io.EOF) {
			return io.EOF
		} else if err != nil {
			return err
		}
	}
	return nil
}

// reader returns a reader over the buffered bytes. It is invalidated by the next fill.
//
// Small bodies are returned as a *bytes.Reader, since http.NewRequest only recognizes that type
// as a zero-length body; any other empty reader would be sent with chunked transfer encoding.
func (b *partBuffer) reader() io.Reader {
	if b.size <= partChunkSize {
		if b.size == 0 {
			return bytes.NewReader(nil)
		}
		return bytes.NewReader(b.chunks[0][:b.size])
	}
	readers := make([]io.Reader, 0, len(b.chunks))
	for remaining, i := b.size, 0; remaining > 0; i++ {
		n := min(remaining, partChunkSize)
		readers = append(readers, bytes.NewReader(b.chunks[i][:n]))
		remaining -= n
	}
	return io.MultiReader(readers...)
}

// allocated returns the number of bytes currently allocated for chunks
func (b *partBuffer) allocated() int {
	return len(b.chunks) * partChunkSize
}

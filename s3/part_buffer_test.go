package s3

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartBuffer_FillAllocatesOnlyWhatWasRead(t *testing.T) {
	// A slow client that sends 4 KB and then stops must not pin a full 5 MB part buffer
	buf := &partBuffer{}
	err := buf.fill(&trickleReader{data: make([]byte, 4096), chunk: 512})
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 4096, buf.size)
	require.LessOrEqual(t, buf.allocated(), partChunkSize)
}

func TestPartBuffer_FillReadErrorAllocatesOnlyWhatWasRead(t *testing.T) {
	// A client that sends a few KB and then disconnects (or times out)
	buf := &partBuffer{}
	readErr := errors.New("connection reset")
	err := buf.fill(io.MultiReader(bytes.NewReader(make([]byte, 3000)), &errReader{err: readErr}))
	require.ErrorIs(t, err, readErr)
	require.Equal(t, 3000, buf.size)
	require.LessOrEqual(t, buf.allocated(), partChunkSize)
}

func TestPartBuffer_FillCapsAtPartSizeAndReuses(t *testing.T) {
	data := make([]byte, partSize+partChunkSize+10)
	for i := range data {
		data[i] = byte(i % 251)
	}
	r := bytes.NewReader(data)
	buf := &partBuffer{}

	// First fill reads exactly one part and allocates no more than that
	require.Nil(t, buf.fill(r))
	require.Equal(t, partSize, buf.size)
	require.Equal(t, partSize, buf.allocated())
	got, err := io.ReadAll(buf.reader())
	require.Nil(t, err)
	require.Equal(t, data[:partSize], got)

	// Second fill reuses the existing chunks instead of allocating new ones
	first := &buf.chunks[0][0]
	require.ErrorIs(t, buf.fill(r), io.EOF)
	require.True(t, first == &buf.chunks[0][0], "first chunk was reallocated")
	require.Equal(t, partChunkSize+10, buf.size)
	require.Equal(t, partSize, buf.allocated())
	got, err = io.ReadAll(buf.reader())
	require.Nil(t, err)
	require.Equal(t, data[partSize:], got)
}

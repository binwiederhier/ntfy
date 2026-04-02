package util

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFixedLimiter_AllowValueReset(t *testing.T) {
	l := NewFixedLimiter(10)
	require.True(t, l.AllowN(5))
	require.Equal(t, int64(5), l.Value())

	require.True(t, l.AllowN(5))
	require.Equal(t, int64(10), l.Value())

	require.False(t, l.Allow())
	require.Equal(t, int64(10), l.Value())

	l.Reset()
	require.Equal(t, int64(0), l.Value())
	require.True(t, l.Allow())
	require.True(t, l.AllowN(9))
	require.False(t, l.Allow())
}

func TestFixedLimiter_AddSub(t *testing.T) {
	l := NewFixedLimiter(10)
	l.AllowN(5)
	if l.value != 5 {
		t.Fatalf("expected value to be %d, got %d", 5, l.value)
	}
	l.AllowN(-2)
	if l.value != 3 {
		t.Fatalf("expected value to be %d, got %d", 7, l.value)
	}
}

func TestBytesLimiter_Add_Simple(t *testing.T) {
	l := NewBytesLimiter(250*1024*1024, 24*time.Hour) // 250 MB per 24h
	require.True(t, l.AllowN(100*1024*1024))
	require.Equal(t, int64(100*1024*1024), l.Value())

	require.True(t, l.AllowN(100*1024*1024))
	require.Equal(t, int64(200*1024*1024), l.Value())

	require.False(t, l.AllowN(300*1024*1024))
	require.Equal(t, int64(200*1024*1024), l.Value())
}

func TestBytesLimiter_Add_Wait(t *testing.T) {
	l := NewBytesLimiter(250*1024*1024, 24*time.Hour) // 250 MB per 24h (~ 303 bytes per 100ms)
	require.True(t, l.AllowN(250*1024*1024))
	require.False(t, l.AllowN(400))
	time.Sleep(200 * time.Millisecond)
	require.True(t, l.AllowN(400))
}

func TestLimitWriter_WriteNoLimiter(t *testing.T) {
	var buf bytes.Buffer
	lw := NewLimitWriter(&buf)
	if _, err := lw.Write(make([]byte, 10)); err != nil {
		t.Fatal(err)
	}
	if _, err := lw.Write(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 11 {
		t.Fatalf("expected buffer length to be %d, got %d", 11, buf.Len())
	}
}

func TestLimitWriter_WriteOneLimiter(t *testing.T) {
	var buf bytes.Buffer
	l := NewFixedLimiter(10)
	lw := NewLimitWriter(&buf, l)
	if _, err := lw.Write(make([]byte, 10)); err != nil {
		t.Fatal(err)
	}
	if _, err := lw.Write(make([]byte, 1)); err != ErrLimitReached {
		t.Fatalf("expected ErrLimitReached, got %#v", err)
	}
	if buf.Len() != 10 {
		t.Fatalf("expected buffer length to be %d, got %d", 10, buf.Len())
	}
	if l.value != 10 {
		t.Fatalf("expected limiter value to be %d, got %d", 10, l.value)
	}
}

func TestLimitWriter_WriteTwoLimiters(t *testing.T) {
	var buf bytes.Buffer
	l1 := NewFixedLimiter(11)
	l2 := NewFixedLimiter(9)
	lw := NewLimitWriter(&buf, l1, l2)
	if _, err := lw.Write(make([]byte, 8)); err != nil {
		t.Fatal(err)
	}
	if _, err := lw.Write(make([]byte, 2)); err != ErrLimitReached {
		t.Fatalf("expected ErrLimitReached, got %#v", err)
	}
	if buf.Len() != 8 {
		t.Fatalf("expected buffer length to be %d, got %d", 8, buf.Len())
	}
	if l1.value != 8 {
		t.Fatalf("expected limiter 1 value to be %d, got %d", 8, l1.value)
	}
	if l2.value != 8 {
		t.Fatalf("expected limiter 2 value to be %d, got %d", 8, l2.value)
	}
}

func TestLimitWriter_WriteTwoDifferentLimiters(t *testing.T) {
	var buf bytes.Buffer
	l1 := NewFixedLimiter(32)
	l2 := NewBytesLimiter(8, 200*time.Millisecond)
	lw := NewLimitWriter(&buf, l1, l2)
	_, err := lw.Write(make([]byte, 8))
	require.Nil(t, err)
	_, err = lw.Write(make([]byte, 4))
	require.Equal(t, ErrLimitReached, err)
}

func TestLimitWriter_WriteTwoDifferentLimiters_Wait(t *testing.T) {
	var buf bytes.Buffer
	l1 := NewFixedLimiter(32)
	l2 := NewBytesLimiter(8, 200*time.Millisecond)
	lw := NewLimitWriter(&buf, l1, l2)
	_, err := lw.Write(make([]byte, 8))
	require.Nil(t, err)
	time.Sleep(250 * time.Millisecond)
	_, err = lw.Write(make([]byte, 8))
	require.Nil(t, err)
	_, err = lw.Write(make([]byte, 4))
	require.Equal(t, ErrLimitReached, err)
}

func TestLimitWriter_WriteTwoDifferentLimiters_Wait_FixedLimiterFail(t *testing.T) {
	var buf bytes.Buffer
	l1 := NewFixedLimiter(11) // <<< This fails below
	l2 := NewBytesLimiter(8, 200*time.Millisecond)
	lw := NewLimitWriter(&buf, l1, l2)
	_, err := lw.Write(make([]byte, 8))
	require.Nil(t, err)
	time.Sleep(250 * time.Millisecond)
	_, err = lw.Write(make([]byte, 8)) // <<< FixedLimiter fails
	require.Equal(t, ErrLimitReached, err)
}

func TestCountingReader_Total(t *testing.T) {
	cr := NewCountingReader(strings.NewReader("hello world"))
	buf := make([]byte, 5)

	n, err := cr.Read(buf)
	require.Nil(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, int64(5), cr.Total())

	n, err = cr.Read(buf)
	require.Nil(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, int64(10), cr.Total())

	n, err = cr.Read(buf)
	require.Nil(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, int64(11), cr.Total())

	_, err = cr.Read(buf)
	require.Equal(t, io.EOF, err)
	require.Equal(t, int64(11), cr.Total())
}

func TestCountingReader_Empty(t *testing.T) {
	cr := NewCountingReader(strings.NewReader(""))
	require.Equal(t, int64(0), cr.Total())

	_, err := cr.Read(make([]byte, 10))
	require.Equal(t, io.EOF, err)
	require.Equal(t, int64(0), cr.Total())
}

func TestLimitReader_ReadNoLimiter(t *testing.T) {
	lr := NewLimitReader(strings.NewReader("hello"))
	data, err := io.ReadAll(lr)
	require.Nil(t, err)
	require.Equal(t, "hello", string(data))
}

func TestLimitReader_ReadOneLimiter(t *testing.T) {
	l := NewFixedLimiter(10)
	lr := NewLimitReader(strings.NewReader("hello world!"), l)

	buf := make([]byte, 5)
	n, err := lr.Read(buf)
	require.Nil(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, int64(5), l.Value())

	n, err = lr.Read(buf)
	require.Nil(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, int64(10), l.Value())

	_, err = lr.Read(buf)
	require.Equal(t, ErrLimitReached, err)
}

func TestLimitReader_ReadTwoLimiters(t *testing.T) {
	l1 := NewFixedLimiter(11)
	l2 := NewFixedLimiter(8)
	lr := NewLimitReader(strings.NewReader("hello world!"), l1, l2)

	buf := make([]byte, 5)
	n, err := lr.Read(buf)
	require.Nil(t, err)
	require.Equal(t, 5, n)

	// Second read: l2 (limit 8) should reject 5 more bytes
	_, err = lr.Read(buf)
	require.Equal(t, ErrLimitReached, err)
	// l1 should have been reverted
	require.Equal(t, int64(5), l1.Value())
	require.Equal(t, int64(5), l2.Value())
}

func TestLimitReader_ReadAll(t *testing.T) {
	l := NewFixedLimiter(100)
	lr := NewLimitReader(strings.NewReader("hello"), l)
	data, err := io.ReadAll(lr)
	require.Nil(t, err)
	require.Equal(t, "hello", string(data))
	require.Equal(t, int64(5), l.Value())
}

func TestLimitReader_ReadExactLimit(t *testing.T) {
	l := NewFixedLimiter(5)
	lr := NewLimitReader(bytes.NewReader(make([]byte, 5)), l)
	data, err := io.ReadAll(lr)
	require.Nil(t, err)
	require.Equal(t, 5, len(data))
	require.Equal(t, int64(5), l.Value())
}

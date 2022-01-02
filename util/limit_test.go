package util

import (
	"bytes"
	"testing"
)

func TestLimiter_Add(t *testing.T) {
	l := NewLimiter(10)
	if err := l.Add(5); err != nil {
		t.Fatal(err)
	}
	if err := l.Add(5); err != nil {
		t.Fatal(err)
	}
	if err := l.Add(5); err != ErrLimitReached {
		t.Fatalf("expected ErrLimitReached, got %#v", err)
	}
}

func TestLimiter_AddSet(t *testing.T) {
	l := NewLimiter(10)
	l.Add(5)
	if l.Value() != 5 {
		t.Fatalf("expected value to be %d, got %d", 5, l.Value())
	}
	l.Set(7)
	if l.Value() != 7 {
		t.Fatalf("expected value to be %d, got %d", 7, l.Value())
	}
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
	l := NewLimiter(10)
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
	if l.Value() != 10 {
		t.Fatalf("expected limiter value to be %d, got %d", 10, l.Value())
	}
}

func TestLimitWriter_WriteTwoLimiters(t *testing.T) {
	var buf bytes.Buffer
	l1 := NewLimiter(11)
	l2 := NewLimiter(9)
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
	if l1.Value() != 8 {
		t.Fatalf("expected limiter 1 value to be %d, got %d", 8, l1.Value())
	}
	if l2.Value() != 8 {
		t.Fatalf("expected limiter 2 value to be %d, got %d", 8, l2.Value())
	}
}

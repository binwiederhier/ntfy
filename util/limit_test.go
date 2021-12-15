package util

import (
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

func TestLimiter_AddSub(t *testing.T) {
	l := NewLimiter(10)
	l.Add(5)
	if l.Value() != 5 {
		t.Fatalf("expected value to be %d, got %d", 5, l.Value())
	}
	l.Sub(2)
	if l.Value() != 3 {
		t.Fatalf("expected value to be %d, got %d", 3, l.Value())
	}
}

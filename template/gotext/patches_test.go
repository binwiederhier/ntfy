package gotext

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// These tests are ntfy's, not upstream's: they cover the ctx-ex patch (patches/), and are kept
// across `make update-template` runs (which only replaces the non-test *.go files).

const (
	testSlowCallCount = 300
	testSlowCallSleep = time.Millisecond
	testSlowTimeout   = 20 * time.Millisecond
	testSlowMaxCalls  = 100
)

func TestExecuteContext_LongPipelineInSingleAction(t *testing.T) {
	// A single action node is one walk step, so the per-node check alone never fires inside it
	calls, slow := newSlowFunc()
	text := "{{0" + strings.Repeat(" | slow", testSlowCallCount) + "}}"
	err := executeSlow(t, text, slow)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v (calls: %d)", err, calls.Load())
	}
	if n := calls.Load(); n >= testSlowMaxCalls {
		t.Fatalf("expected execution to stop early, but slow was called %d times", n)
	}
}

func TestExecuteContext_NestedCallsInSingleAction(t *testing.T) {
	calls, slow := newSlowFunc()
	text := "{{" + strings.Repeat("slow (", testSlowCallCount) + "0" + strings.Repeat(")", testSlowCallCount) + "}}"
	err := executeSlow(t, text, slow)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v (calls: %d)", err, calls.Load())
	}
	if n := calls.Load(); n >= testSlowMaxCalls {
		t.Fatalf("expected execution to stop early, but slow was called %d times", n)
	}
}

func TestExecuteContext_NoDeadlineRunsToCompletion(t *testing.T) {
	calls, slow := newSlowFunc()
	text := "{{0" + strings.Repeat(" | slow", 5) + "}}"
	tmpl := Must(New("t").Funcs(FuncMap{"slow": slow}).Parse(text))
	var out strings.Builder
	if err := tmpl.ExecuteContext(context.Background(), &out, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != "5" || calls.Load() != 5 {
		t.Fatalf("expected output 5 after 5 calls, got %q after %d calls", out.String(), calls.Load())
	}
}

func newSlowFunc() (*atomic.Int64, func(int) int) {
	calls := new(atomic.Int64)
	slow := func(x int) int {
		calls.Add(1)
		time.Sleep(testSlowCallSleep)
		return x + 1
	}
	return calls, slow
}

func executeSlow(t *testing.T, text string, slow func(int) int) error {
	t.Helper()
	tmpl := Must(New("t").Funcs(FuncMap{"slow": slow}).Parse(text))
	ctx, cancel := context.WithTimeout(context.Background(), testSlowTimeout)
	defer cancel()
	return tmpl.ExecuteContext(ctx, io.Discard, nil)
}

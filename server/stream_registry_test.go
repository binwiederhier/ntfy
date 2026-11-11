package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamRegistry_CancelSomeClosesAndForgets(t *testing.T) {
	r := newStreamRegistry()
	ctxs := make([]context.Context, 0)
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs = append(ctxs, ctx)
		r.Add(cancel)
	}
	require.Equal(t, 5, r.Len())

	// Closed streams are forgotten at once, so the next call picks others
	require.Equal(t, 2, r.CancelSome(2))
	require.Equal(t, 3, r.Len())
	require.Equal(t, 3, r.CancelSome(10))
	require.Equal(t, 0, r.Len())
	for _, ctx := range ctxs {
		require.Error(t, ctx.Err())
	}
}

func TestStreamRegistry_RemoveAfterCancelIsANop(t *testing.T) {
	// A canceled stream's handler still calls Remove when it returns
	r := newStreamRegistry()
	_, cancel := context.WithCancel(context.Background())
	id := r.Add(cancel)
	require.Equal(t, 1, r.CancelSome(1))
	r.Remove(id)
	require.Equal(t, 0, r.Len())
}

package server

import (
	"github.com/stretchr/testify/require"
	"sync/atomic"
	"testing"
	"time"
)

func TestTopic_CancelSubscribers(t *testing.T) {
	t.Parallel()

	subFn := func(v *visitor, msg *message) error {
		return nil
	}
	canceled1 := atomic.Bool{}
	cancelFn1 := func() {
		canceled1.Store(true)
	}
	canceled2 := atomic.Bool{}
	cancelFn2 := func() {
		canceled2.Store(true)
	}
	to := newTopic("mytopic")
	to.Subscribe(subFn, "", cancelFn1)
	to.Subscribe(subFn, "u_phil", cancelFn2)

	to.CancelSubscribers("u_phil")
	require.True(t, canceled1.Load())
	require.False(t, canceled2.Load())
}

func TestTopic_Keepalive(t *testing.T) {
	t.Parallel()

	to := newTopic("mytopic")
	to.lastAccess = time.Now().Add(-1 * time.Hour)
	to.Keepalive()
	require.True(t, to.LastAccess().Unix() >= time.Now().Unix()-2)
	require.True(t, to.LastAccess().Unix() <= time.Now().Unix()+2)
}

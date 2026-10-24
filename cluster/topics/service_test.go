package topics

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func newTestStore(t *testing.T) *Store {
	host, err := pg.Open(dbtest.CreateTestPostgresSchema(t))
	require.Nil(t, err)
	d := db.New(host, nil)
	t.Cleanup(func() { d.Close() })
	store, err := New(d)
	require.Nil(t, err)
	return store
}

func TestStore_RecordAndGet(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("mytopic")
	require.ErrorIs(t, err, ErrNotFound)

	require.Nil(t, s.RecordPublish("mytopic", "ip:1.2.3.4"))
	require.Nil(t, s.RecordSubscriber("mytopic", "ip:5.6.7.8"))
	info, err := s.Get("mytopic")
	require.Nil(t, err)
	require.Equal(t, "ip:1.2.3.4", info.LastPublisherKey)
	require.Equal(t, "ip:5.6.7.8", info.LastSubscriberKey)
	require.False(t, info.CreatedAt.IsZero())
	require.Equal(t, "", info.RateVisitorKey)
}

func TestStore_RateVisitor_SetGetReplace(t *testing.T) {
	s := newTestStore(t)
	_, _, err := s.RateVisitor("up123456789012")
	require.ErrorIs(t, err, ErrNotFound)

	require.Nil(t, s.SetRateVisitor("up123456789012", "ip:1.2.3.4", ""))
	key, uid, err := s.RateVisitor("up123456789012")
	require.Nil(t, err)
	require.Equal(t, "ip:1.2.3.4", key)
	require.Equal(t, "", uid)

	// Replacement (subscriber moved or logged in)
	require.Nil(t, s.SetRateVisitor("up123456789012", "user:u_abc", "u_abc"))
	key, uid, err = s.RateVisitor("up123456789012")
	require.Nil(t, err)
	require.Equal(t, "user:u_abc", key)
	require.Equal(t, "u_abc", uid)
}

func TestStore_RateVisitor_KeepaliveRefreshBeatsTTL(t *testing.T) {
	// The wrongful-Matrix-reject scenario: a subscriber holds one connection longer than the
	// TTL. Its (throttled) keepalive RecordSubscriber calls must refresh the assignment's
	// liveness, so it never expires while the subscriber is around.
	s := newTestStore(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	require.Nil(t, s.SetRateVisitor("up123456789012", "ip:1.2.3.4", ""))

	// 20h later: a keepalive from the rate visitor refreshes liveness
	now = now.Add(20 * time.Hour)
	require.Nil(t, s.RecordSubscriber("up123456789012", "ip:1.2.3.4"))

	// 20h after that (40h after Set): still valid, because the keepalive refreshed it
	now = now.Add(20 * time.Hour)
	_, _, err := s.RateVisitor("up123456789012")
	require.Nil(t, err)

	// A keepalive from a DIFFERENT visitor must NOT refresh the assignment
	require.Nil(t, s.RecordSubscriber("up123456789012", "ip:9.9.9.9"))
	now = now.Add(25 * time.Hour)
	_, _, err = s.RateVisitor("up123456789012")
	require.ErrorIs(t, err, ErrNotFound)
	info, err := s.Get("up123456789012")
	require.Nil(t, err)
	require.Equal(t, "ip:1.2.3.4", info.RateVisitorKey) // Assignment still recorded, just stale
}

func TestStore_Prune(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	require.Nil(t, s.RecordPublish("oldtopic", "ip:1.2.3.4"))
	now = now.Add(31 * 24 * time.Hour)
	require.Nil(t, s.RecordPublish("newtopic", "ip:1.2.3.4"))
	require.Nil(t, s.Prune())
	_, err := s.Get("oldtopic")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.Get("newtopic")
	require.Nil(t, err)
}

func TestStore_LastActivity(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	require.Nil(t, s.RecordPublish("published", "ip:1.2.3.4"))
	now = now.Add(time.Hour)
	require.Nil(t, s.RecordSubscriber("subscribed", "ip:1.2.3.4"))
	now = now.Add(time.Hour)
	require.Nil(t, s.RecordPublish("both", "ip:1.2.3.4"))
	now = now.Add(time.Hour)
	require.Nil(t, s.RecordSubscriber("both", "ip:1.2.3.4"))

	// Unknown topics are absent from the result; lookups span multiple query chunks
	ids := []string{"published", "subscribed", "both"}
	for i := 0; i < 2*lastActivityChunkSize; i++ {
		ids = append(ids, fmt.Sprintf("unknown%d", i))
	}
	activity, err := s.LastActivity(ids)
	require.Nil(t, err)
	require.Equal(t, 3, len(activity))
	require.Equal(t, now.Add(-3*time.Hour).Unix(), activity["published"].Unix())
	require.Equal(t, now.Add(-2*time.Hour).Unix(), activity["subscribed"].Unix())
	require.Equal(t, now.Unix(), activity["both"].Unix())
}

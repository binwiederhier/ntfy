package registry

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func openTestPool(t *testing.T, dsn string) *db.DB {
	t.Helper()
	host, err := pg.Open(dsn)
	require.Nil(t, err)
	d := db.New(host, nil)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestRegistry_NewDoesNotRegister(t *testing.T) {
	// New only sets up the schema and the identity handle; joining the cluster is an explicit
	// Register call, owned by the caller (the mesh registers synchronously at construction).
	// This keeps read-only uses (ops tooling, future admin endpoints) side-effect free.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Equal(t, 0, countRows(t, pool, "node-1"))
	require.Nil(t, r1.Register())
	require.Equal(t, 1, countRows(t, pool, "node-1"))
}

func TestRegistry_RegisterAndPeers(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r1.Register())
	r2, err := New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r2.Register())
	// Each node sees the other, never itself
	peers, err := r1.Peers()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "node-2", peers[0].NodeID)
	require.Equal(t, "http://10.0.0.2:2587", peers[0].AdvertiseURL)
	peers, err = r2.Peers()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "node-1", peers[0].NodeID)
}

func TestRegistry_ReRegisterUpdatesAdvertiseURL(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	// The same node comes back under a new address; the upsert replaces the row
	old, err := New(pool, "node-2", "http://old:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, old.Register())
	renewed, err := New(pool, "node-2", "http://new:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, renewed.Register())
	expireCache(r1)
	peers, err := r1.Peers()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "http://new:2587", peers[0].AdvertiseURL)
}

func TestRegistry_PeersCachedForTTL(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	peers, err := r1.Peers()
	require.Nil(t, err)
	require.Empty(t, peers)
	// A node joining after the cache was populated is invisible until the cache expires
	r2, err := New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r2.Register())
	peers, err = r1.Peers()
	require.Nil(t, err)
	require.Empty(t, peers)
	expireCache(r1)
	peers, err = r1.Peers()
	require.Nil(t, err)
	require.Len(t, peers, 1)
}

func TestRegistry_TTLExcludesSilentNodes(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	// A node whose heartbeat is older than the TTL does not count as live
	_, err = pool.Exec(upsertNodeQuery, "node-silent", "http://10.0.0.9:2587", time.Now().Add(-2*time.Minute).Unix())
	require.Nil(t, err)
	peers, err := r1.Peers()
	require.Nil(t, err)
	require.Empty(t, peers)
}

func TestRegistry_PruneDeletesLongDeadOnly(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	// One node beyond the 3x TTL grace period, one merely stale
	_, err = pool.Exec(upsertNodeQuery, "node-long-dead", "http://10.0.0.8:2587", time.Now().Add(-4*time.Minute).Unix())
	require.Nil(t, err)
	_, err = pool.Exec(upsertNodeQuery, "node-slow", "http://10.0.0.9:2587", time.Now().Add(-2*time.Minute).Unix())
	require.Nil(t, err)
	require.Nil(t, r1.Prune())
	require.Equal(t, 0, countRows(t, pool, "node-long-dead"))
	require.Equal(t, 1, countRows(t, pool, "node-slow")) // Slow, not dead: kept
}

func TestRegistry_Deregister(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r1.Register())
	require.Equal(t, 1, countRows(t, pool, "node-1"))
	require.Nil(t, r1.Deregister())
	require.Equal(t, 0, countRows(t, pool, "node-1"))
}

func TestRegistry_PeersStaleCacheOnError(t *testing.T) {
	// During a database hiccup, Peers serves the last-known peer list instead of erroring:
	// fan-out keeps flowing to known peers, and the publish path does not log a warning per
	// message for the duration of the outage.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	r1, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	r2, err := New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
	require.Nil(t, r2.Register())
	peers, err := r1.Peers()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	// Expire the cache and break the database; the stale list must still be served
	expireCache(r1)
	require.Nil(t, pool.Close())
	peers, err = r1.Peers()
	require.Nil(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "node-2", peers[0].NodeID)
}

func TestRegistry_ConcurrentCreate(t *testing.T) {
	// Multiple nodes cold-booting on a fresh database must not race on table creation: CREATE
	// TABLE IF NOT EXISTS is not atomic in PostgreSQL, so creation is serialized via an advisory
	// lock. Without it, this test fails sporadically with a duplicate-key error on pg_class.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			pool, err := pg.Open(schemaDSN)
			if err != nil {
				errs <- err
				return
			}
			defer pool.DB.Close()
			_, err = New(db.New(pool, nil), fmt.Sprintf("node-%d", i), "http://127.0.0.1:1", time.Second)
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		require.Nil(t, <-errs)
	}
}

func TestRegistry_SchemaVersionWritten(t *testing.T) {
	// The registry participates in the shared schema_version framework like every other store,
	// so future table changes can be applied as migrations.
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	_, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	var version int
	require.Nil(t, pool.QueryRow(`SELECT version FROM schema_version WHERE store = $1`, schemaStoreKey).Scan(&version))
	require.Equal(t, schemaVersion, version)
	// Setup is idempotent: a second node boots against the migrated schema
	_, err = New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Nil(t, err)
}

func TestRegistry_SchemaVersionFromTheFuture(t *testing.T) {
	// A node running older code must refuse to touch a schema migrated by newer code
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	_, err := New(pool, "node-1", "http://10.0.0.1:2587", time.Minute)
	require.Nil(t, err)
	_, err = pool.Exec(`UPDATE schema_version SET version = 99 WHERE store = $1`, schemaStoreKey)
	require.Nil(t, err)
	_, err = New(pool, "node-2", "http://10.0.0.2:2587", time.Minute)
	require.Error(t, err)
}

// expireCache forces the next Peers() call to re-read the registry table.
func expireCache(r *Registry) {
	r.mu.Lock()
	r.peersFetched = time.Time{}
	r.mu.Unlock()
}

func countRows(t *testing.T, pool *db.DB, nodeID string) int {
	t.Helper()
	var count int
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE node_id = $1`, nodeID).Scan(&count))
	return count
}

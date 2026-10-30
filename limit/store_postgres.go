package limit

import (
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

// Timestamps come from the database clock, never from a node's: the read-back watermark compares
// them, and node clocks neither agree with each other nor with the database. The upsert is one
// statement for a whole chunk (unnest), because per-call SQL bloats pgx's statement cache.
const (
	postgresSelectClockQuery = `SELECT EXTRACT(EPOCH FROM now())::BIGINT`
	postgresSelectSinceQuery = `SELECT key, kind, count FROM usage WHERE day = $1 AND updated_at >= $2`
	postgresSelectKeysQuery  = `SELECT key, kind, count FROM usage WHERE day = $1 AND key = ANY($2)`
	postgresUpsertQuery      = `
		INSERT INTO usage (key, day, kind, count, updated_at)
		SELECT k, $1, kd, c, EXTRACT(EPOCH FROM now())::BIGINT FROM unnest($2::TEXT[], $3::TEXT[], $4::BIGINT[]) AS t(k, kd, c)
		ON CONFLICT (key, day, kind) DO UPDATE SET count = usage.count + EXCLUDED.count, updated_at = EXCLUDED.updated_at
	`
	postgresPruneQuery = `DELETE FROM usage WHERE day < $1`
)

// NewPostgresStore returns the store on the shared pool; the usage table is the user schema's (see
// package user, which creates and migrates it)
func NewPostgresStore(d *db.DB) *Store {
	return &Store{
		db:      d,
		dialect: schema.Postgres,
		queries: queries{
			selectClock: postgresSelectClockQuery,
			selectSince: postgresSelectSinceQuery,
			selectKeys:  postgresSelectKeysQuery,
			upsert:      postgresUpsertQuery,
			prune:       postgresPruneQuery,
		},
	}
}

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

// Schema
var (
	postgresCreateTables = schema.AsMigrateFunc(`
		CREATE TABLE IF NOT EXISTS usage (
			key TEXT NOT NULL,
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			count BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (key, day, kind)
		);
		CREATE INDEX IF NOT EXISTS idx_usage_day_updated_at ON usage (day, updated_at);
	`)
)

// NewPostgresStore sets up the usage table on the shared pool
func NewPostgresStore(d *db.DB) (*Store, error) {
	if err := schema.Migrate(d.Primary(), schema.Postgres, schemaStore, postgresCreateTables, nil); err != nil {
		return nil, err
	}
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
	}, nil
}

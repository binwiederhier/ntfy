package user

import (
	"database/sql"
	"fmt"
)

// Initial PostgreSQL schema
const (
	postgresCreateTablesQueries = `
		CREATE TABLE IF NOT EXISTS tier (
			id TEXT PRIMARY KEY,
			code TEXT NOT NULL,
			name TEXT NOT NULL,
			messages_limit BIGINT NOT NULL,
			messages_expiry_duration BIGINT NOT NULL,
			emails_limit BIGINT NOT NULL,
			calls_limit BIGINT NOT NULL,
			reservations_limit BIGINT NOT NULL,
			attachment_file_size_limit BIGINT NOT NULL,
			attachment_total_size_limit BIGINT NOT NULL,
			attachment_expiry_duration BIGINT NOT NULL,
			attachment_bandwidth_limit BIGINT NOT NULL,
			stripe_monthly_price_id TEXT,
			stripe_yearly_price_id TEXT,
			UNIQUE(code),
			UNIQUE(stripe_monthly_price_id),
			UNIQUE(stripe_yearly_price_id)
		);
		CREATE TABLE IF NOT EXISTS "user" (
		    id TEXT PRIMARY KEY,
			tier_id TEXT REFERENCES tier(id),
			user_name TEXT NOT NULL UNIQUE,
			pass TEXT NOT NULL,
			role TEXT NOT NULL CHECK (role IN ('anonymous', 'admin', 'user')),
			prefs JSONB NOT NULL DEFAULT '{}',
			sync_topic TEXT NOT NULL,
			provisioned BOOLEAN NOT NULL,
			stats_messages BIGINT NOT NULL DEFAULT 0,
			stats_emails BIGINT NOT NULL DEFAULT 0,
			stats_calls BIGINT NOT NULL DEFAULT 0,
			stripe_customer_id TEXT UNIQUE,
			stripe_subscription_id TEXT UNIQUE,
			stripe_subscription_status TEXT,
			stripe_subscription_interval TEXT,
			stripe_subscription_paid_until BIGINT,
			stripe_subscription_cancel_at BIGINT,
			created BIGINT NOT NULL,
			deleted BIGINT
		);
		CREATE TABLE IF NOT EXISTS user_access (
			user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
			topic TEXT NOT NULL,
			read BOOLEAN NOT NULL,
			write BOOLEAN NOT NULL,
			owner_user_id TEXT REFERENCES "user"(id) ON DELETE CASCADE,
			provisioned BOOLEAN NOT NULL,
			PRIMARY KEY (user_id, topic)
		);
		CREATE TABLE IF NOT EXISTS user_token (
			user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
			token TEXT NOT NULL UNIQUE,
			label TEXT NOT NULL,
			last_access BIGINT NOT NULL,
			last_origin TEXT NOT NULL,
			expires BIGINT NOT NULL,
			provisioned BOOLEAN NOT NULL,
			PRIMARY KEY (user_id, token)
		);
		CREATE TABLE IF NOT EXISTS user_phone (
			user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
			phone_number TEXT NOT NULL,
			PRIMARY KEY (user_id, phone_number)
		);
		CREATE TABLE IF NOT EXISTS user_email (
			user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
			email TEXT NOT NULL,
			PRIMARY KEY (user_id, email)
		);
		CREATE TABLE IF NOT EXISTS schema_version (
			store TEXT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO "user" (id, user_name, pass, role, sync_topic, provisioned, created)
		VALUES ('` + everyoneID + `', '*', '', 'anonymous', '', false, EXTRACT(EPOCH FROM NOW())::BIGINT)
		ON CONFLICT (id) DO NOTHING;
	`
)

// Schema table management queries for Postgres
const (
	postgresCurrentSchemaVersion     = 7
	postgresSelectSchemaVersionQuery = `SELECT version FROM schema_version WHERE store = 'user'`
	postgresInsertSchemaVersionQuery = `INSERT INTO schema_version (store, version) VALUES ('user', $1)`
)

const (
	postgresMigrate6To7UpdateQueries = `
		CREATE TABLE IF NOT EXISTS user_email (
			user_id TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
			email TEXT NOT NULL,
			PRIMARY KEY (user_id, email)
		);
	`
	postgresUpdateSchemaVersionQuery = `UPDATE schema_version SET version = $1 WHERE store = 'user'`
)

var postgresMigrations = map[int]func(db *sql.DB) error{
	6: postgresMigrateFrom6,
}

func setupPostgres(db *sql.DB) error {
	var schemaVersion int
	err := db.QueryRow(postgresSelectSchemaVersionQuery).Scan(&schemaVersion)
	if err != nil {
		return setupNewPostgres(db)
	}
	if schemaVersion == postgresCurrentSchemaVersion {
		return nil
	} else if schemaVersion > postgresCurrentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, postgresCurrentSchemaVersion)
	}
	for i := schemaVersion; i < postgresCurrentSchemaVersion; i++ {
		fn, ok := postgresMigrations[i]
		if !ok {
			return fmt.Errorf("cannot find migration step from schema version %d to %d", i, i+1)
		} else if err := fn(db); err != nil {
			return err
		}
	}
	return nil
}

func postgresMigrateFrom6(db *sql.DB) error {
	if _, err := db.Exec(postgresMigrate6To7UpdateQueries); err != nil {
		return err
	}
	if _, err := db.Exec(postgresUpdateSchemaVersionQuery, 7); err != nil {
		return err
	}
	return nil
}

func setupNewPostgres(db *sql.DB) error {
	if _, err := db.Exec(postgresCreateTablesQueries); err != nil {
		return err
	}
	if _, err := db.Exec(postgresInsertSchemaVersionQuery, postgresCurrentSchemaVersion); err != nil {
		return err
	}
	return nil
}

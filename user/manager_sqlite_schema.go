package user

import (
	"database/sql"

	"heckel.io/ntfy/v2/db/schema"
	"heckel.io/ntfy/v2/util"
)

// Initial SQLite schema
const (
	sqliteCreateTablesQueries = `
		CREATE TABLE IF NOT EXISTS tier (
			id TEXT PRIMARY KEY,
			code TEXT NOT NULL,
			name TEXT NOT NULL,
			messages_limit INT NOT NULL,
			messages_expiry_duration INT NOT NULL,
			emails_limit INT NOT NULL,
			calls_limit INT NOT NULL,
			reservations_limit INT NOT NULL,
			attachment_file_size_limit INT NOT NULL,
			attachment_total_size_limit INT NOT NULL,
			attachment_expiry_duration INT NOT NULL,
			attachment_bandwidth_limit INT NOT NULL,
			stripe_monthly_price_id TEXT,
			stripe_yearly_price_id TEXT
		);
		CREATE UNIQUE INDEX idx_tier_code ON tier (code);
		CREATE UNIQUE INDEX idx_tier_stripe_monthly_price_id ON tier (stripe_monthly_price_id);
		CREATE UNIQUE INDEX idx_tier_stripe_yearly_price_id ON tier (stripe_yearly_price_id);
		CREATE TABLE IF NOT EXISTS user (
		    id TEXT PRIMARY KEY,
			tier_id TEXT,
			user TEXT NOT NULL,
			pass TEXT NOT NULL,
			role TEXT CHECK (role IN ('anonymous', 'admin', 'user')) NOT NULL,
			prefs JSON NOT NULL DEFAULT '{}',
			sync_topic TEXT NOT NULL,
			provisioned INT NOT NULL,
			stats_messages INT NOT NULL DEFAULT (0),
			stats_emails INT NOT NULL DEFAULT (0),
			stats_calls INT NOT NULL DEFAULT (0),
			stripe_customer_id TEXT,
			stripe_subscription_id TEXT,
			stripe_subscription_status TEXT,
			stripe_subscription_interval TEXT,
			stripe_subscription_paid_until INT,
			stripe_subscription_cancel_at INT,
			created INT NOT NULL,
			deleted INT,
		    FOREIGN KEY (tier_id) REFERENCES tier (id)
		);
		CREATE UNIQUE INDEX idx_user ON user (user);
		CREATE UNIQUE INDEX idx_user_stripe_customer_id ON user (stripe_customer_id);
		CREATE UNIQUE INDEX idx_user_stripe_subscription_id ON user (stripe_subscription_id);
		CREATE TABLE IF NOT EXISTS user_access (
			user_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			read INT NOT NULL,
			write INT NOT NULL,
			owner_user_id INT,
			provisioned INT NOT NULL,
			PRIMARY KEY (user_id, topic),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE,
		    FOREIGN KEY (owner_user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS user_token (
			user_id TEXT NOT NULL,
			token TEXT NOT NULL,
			label TEXT NOT NULL,
			last_access INT NOT NULL,
			last_origin TEXT NOT NULL,
			expires INT NOT NULL,
			provisioned INT NOT NULL,
			PRIMARY KEY (user_id, token),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE UNIQUE INDEX idx_user_token ON user_token (token);
		CREATE TABLE IF NOT EXISTS user_phone (
			user_id TEXT NOT NULL,
			phone_number TEXT NOT NULL,
			PRIMARY KEY (user_id, phone_number),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS user_email (
			user_id TEXT NOT NULL,
			email TEXT NOT NULL,
			is_primary INT NOT NULL DEFAULT (0),
			PRIMARY KEY (user_id, email),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE UNIQUE INDEX idx_user_email_primary_user ON user_email (user_id) WHERE is_primary = 1;
		CREATE UNIQUE INDEX idx_user_email_primary_addr ON user_email (email) WHERE is_primary = 1;
		CREATE TABLE IF NOT EXISTS user_magic_link (
			token_hash TEXT NOT NULL,
			kind TEXT NOT NULL,
			user_id TEXT NOT NULL,
			email TEXT,
			expires INT NOT NULL,
			created INT NOT NULL,
			PRIMARY KEY (token_hash),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE INDEX idx_magic_link_user_kind ON user_magic_link (user_id, kind);
		INSERT INTO user (id, user, pass, role, sync_topic, provisioned, created)
		VALUES ('` + everyoneID + `', '*', '', 'anonymous', '', false, UNIXEPOCH())
		ON CONFLICT (id) DO NOTHING;
	`
)

const (
	sqliteBuiltinStartupQueries = `PRAGMA foreign_keys = ON;`
)

const (
	sqliteCurrentSchemaVersion = 9
)

// Schema migrations for SQLite
const (
	// 1 -> 2 (complex migration!)
	sqliteMigrate1To2CreateTablesQueries = `
		ALTER TABLE user RENAME TO user_old;
		CREATE TABLE IF NOT EXISTS tier (
			id TEXT PRIMARY KEY,
			code TEXT NOT NULL,
			name TEXT NOT NULL,
			messages_limit INT NOT NULL,
			messages_expiry_duration INT NOT NULL,
			emails_limit INT NOT NULL,
			reservations_limit INT NOT NULL,
			attachment_file_size_limit INT NOT NULL,
			attachment_total_size_limit INT NOT NULL,
			attachment_expiry_duration INT NOT NULL,
			attachment_bandwidth_limit INT NOT NULL,
			stripe_price_id TEXT
		);
		CREATE UNIQUE INDEX idx_tier_code ON tier (code);
		CREATE UNIQUE INDEX idx_tier_price_id ON tier (stripe_price_id);
		CREATE TABLE IF NOT EXISTS user (
		    id TEXT PRIMARY KEY,
			tier_id TEXT,
			user TEXT NOT NULL,
			pass TEXT NOT NULL,
			role TEXT CHECK (role IN ('anonymous', 'admin', 'user')) NOT NULL,
			prefs JSON NOT NULL DEFAULT '{}',
			sync_topic TEXT NOT NULL,
			stats_messages INT NOT NULL DEFAULT (0),
			stats_emails INT NOT NULL DEFAULT (0),
			stripe_customer_id TEXT,
			stripe_subscription_id TEXT,
			stripe_subscription_status TEXT,
			stripe_subscription_paid_until INT,
			stripe_subscription_cancel_at INT,
			created INT NOT NULL,
			deleted INT,
		    FOREIGN KEY (tier_id) REFERENCES tier (id)
		);
		CREATE UNIQUE INDEX idx_user ON user (user);
		CREATE UNIQUE INDEX idx_user_stripe_customer_id ON user (stripe_customer_id);
		CREATE UNIQUE INDEX idx_user_stripe_subscription_id ON user (stripe_subscription_id);
		CREATE TABLE IF NOT EXISTS user_access (
			user_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			read INT NOT NULL,
			write INT NOT NULL,
			owner_user_id INT,
			PRIMARY KEY (user_id, topic),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE,
		    FOREIGN KEY (owner_user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS user_token (
			user_id TEXT NOT NULL,
			token TEXT NOT NULL,
			label TEXT NOT NULL,
			last_access INT NOT NULL,
			last_origin TEXT NOT NULL,
			expires INT NOT NULL,
			PRIMARY KEY (user_id, token),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		INSERT INTO user (id, user, pass, role, sync_topic, created)
		VALUES ('u_everyone', '*', '', 'anonymous', '', UNIXEPOCH())
		ON CONFLICT (id) DO NOTHING;
	`
	sqliteMigrate1To2SelectAllOldUsernamesNoTxQuery = `SELECT user FROM user_old`
	sqliteMigrate1To2InsertUserNoTxQuery            = `
		INSERT INTO user (id, user, pass, role, sync_topic, created)
		SELECT ?, user, pass, role, ?, UNIXEPOCH() FROM user_old WHERE user = ?
	`
	sqliteMigrate1To2InsertFromOldTablesAndDropNoTxQuery = `
		INSERT INTO user_access (user_id, topic, read, write)
		SELECT u.id, a.topic, a.read, a.write
		FROM user u
	 	JOIN access a ON u.user = a.user;

		DROP TABLE access;
		DROP TABLE user_old;
	`

	// 2 -> 3
	sqliteMigrate2To3UpdateQueries = `
		ALTER TABLE user ADD COLUMN stripe_subscription_interval TEXT;
		ALTER TABLE tier RENAME COLUMN stripe_price_id TO stripe_monthly_price_id;
		ALTER TABLE tier ADD COLUMN stripe_yearly_price_id TEXT;
		DROP INDEX IF EXISTS idx_tier_price_id;
		CREATE UNIQUE INDEX idx_tier_stripe_monthly_price_id ON tier (stripe_monthly_price_id);
		CREATE UNIQUE INDEX idx_tier_stripe_yearly_price_id ON tier (stripe_yearly_price_id);
	`

	// 3 -> 4
	sqliteMigrate3To4UpdateQueries = `
		ALTER TABLE tier ADD COLUMN calls_limit INT NOT NULL DEFAULT (0);
		ALTER TABLE user ADD COLUMN stats_calls INT NOT NULL DEFAULT (0);
		CREATE TABLE IF NOT EXISTS user_phone (
			user_id TEXT NOT NULL,
			phone_number TEXT NOT NULL,
			PRIMARY KEY (user_id, phone_number),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
	`

	// 4 -> 5
	sqliteMigrate4To5UpdateQueries = `
		UPDATE user_access SET topic = REPLACE(topic, '_', '\_');
	`

	// 6 -> 7
	sqliteMigrate6To7UpdateQueries = `
		CREATE TABLE IF NOT EXISTS user_email (
			user_id TEXT NOT NULL,
			email TEXT NOT NULL,
			PRIMARY KEY (user_id, email),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
	`

	// 7 -> 8: primary (recovery) email + magic-link table for verification/reset.
	// No backfill -- existing verified emails stay non-primary, so the ALTER cannot
	// conflict and no old notification address becomes a recovery channel.
	sqliteMigrate7To8UpdateQueries = `
		ALTER TABLE user_email ADD COLUMN is_primary INT NOT NULL DEFAULT (0);
		CREATE UNIQUE INDEX idx_user_email_primary_user ON user_email (user_id) WHERE is_primary = 1;
		CREATE UNIQUE INDEX idx_user_email_primary_addr ON user_email (email) WHERE is_primary = 1;
		CREATE TABLE IF NOT EXISTS user_magic_link (
			token_hash TEXT NOT NULL,
			kind TEXT NOT NULL,
			user_id TEXT NOT NULL,
			email TEXT,
			expires INT NOT NULL,
			created INT NOT NULL,
			PRIMARY KEY (token_hash),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE INDEX idx_magic_link_user_kind ON user_magic_link (user_id, kind);
	`

	// 5 -> 6. The table rebuilds below rely on foreign keys being OFF (otherwise RENAME
	// would rewrite the childrens' REFERENCES clauses to point at the _old tables). This is
	// guaranteed because migrations run on fresh connections, before the startup queries
	// enable the foreign_keys pragma; see NewSQLiteManager.
	sqliteMigrate5To6UpdateQueries = `
		-- Alter user table: Add provisioned column
		ALTER TABLE user RENAME TO user_old;
		CREATE TABLE IF NOT EXISTS user (
		    id TEXT PRIMARY KEY,
			tier_id TEXT,
			user TEXT NOT NULL,
			pass TEXT NOT NULL,
			role TEXT CHECK (role IN ('anonymous', 'admin', 'user')) NOT NULL,
			prefs JSON NOT NULL DEFAULT '{}',
			sync_topic TEXT NOT NULL,
			provisioned INT NOT NULL,
			stats_messages INT NOT NULL DEFAULT (0),
			stats_emails INT NOT NULL DEFAULT (0),
			stats_calls INT NOT NULL DEFAULT (0),
			stripe_customer_id TEXT,
			stripe_subscription_id TEXT,
			stripe_subscription_status TEXT,
			stripe_subscription_interval TEXT,
			stripe_subscription_paid_until INT,
			stripe_subscription_cancel_at INT,
			created INT NOT NULL,
			deleted INT,
		    FOREIGN KEY (tier_id) REFERENCES tier (id)
		);
		INSERT INTO user
		SELECT
		    id,
		    tier_id,
		    user,
		    pass,
		    role,
		    prefs,
		    sync_topic,
		    0, -- provisioned
		    stats_messages,
		    stats_emails,
		    stats_calls,
		    stripe_customer_id,
		    stripe_subscription_id,
		    stripe_subscription_status,
		    stripe_subscription_interval,
		    stripe_subscription_paid_until,
		    stripe_subscription_cancel_at,
		    created,
		    deleted
		FROM user_old;
		DROP TABLE user_old;

		-- Alter user_access table: Add provisioned column
		ALTER TABLE user_access RENAME TO user_access_old;
		CREATE TABLE user_access (
			user_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			read INT NOT NULL,
			write INT NOT NULL,
			owner_user_id INT,
			provisioned INTEGER NOT NULL,
			PRIMARY KEY (user_id, topic),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE,
			FOREIGN KEY (owner_user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		INSERT INTO user_access SELECT *, 0 FROM user_access_old;
		DROP TABLE user_access_old;

		-- Alter user_token table: Add provisioned column
		ALTER TABLE user_token RENAME TO user_token_old;
		CREATE TABLE IF NOT EXISTS user_token (
			user_id TEXT NOT NULL,
			token TEXT NOT NULL,
			label TEXT NOT NULL,
			last_access INT NOT NULL,
			last_origin TEXT NOT NULL,
			expires INT NOT NULL,
			provisioned INT NOT NULL,
			PRIMARY KEY (user_id, token),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		INSERT INTO user_token SELECT *, 0 FROM user_token_old;
		DROP TABLE user_token_old;

		-- Recreate indices
		CREATE UNIQUE INDEX idx_user ON user (user);
		CREATE UNIQUE INDEX idx_user_stripe_customer_id ON user (stripe_customer_id);
		CREATE UNIQUE INDEX idx_user_stripe_subscription_id ON user (stripe_subscription_id);
		CREATE UNIQUE INDEX idx_user_token ON user_token (token);
	`

	// 8 -> 9: Repair the user_phone foreign key. The 5 -> 6 migration renamed user to
	// user_old, which rewrote user_phone's REFERENCES clause to user_old -- a table that was
	// then dropped (the rebuilt tables got correct fresh foreign keys; user_phone was the only
	// child table not rebuilt). Rebuilding user_phone re-points the foreign key at user; on
	// healthy databases the rebuild is a harmless no-op schema-wise.
	sqliteMigrate8To9UpdateQueries = `
		ALTER TABLE user_phone RENAME TO user_phone_old;
		CREATE TABLE user_phone (
			user_id TEXT NOT NULL,
			phone_number TEXT NOT NULL,
			PRIMARY KEY (user_id, phone_number),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		INSERT INTO user_phone (user_id, phone_number)
		SELECT user_id, phone_number FROM user_phone_old
		WHERE user_id IN (SELECT id FROM user); -- Drop orphaned rows that the broken foreign key failed to cascade-delete
		DROP TABLE user_phone_old;
	`
)

var (
	sqliteCreateTables = schema.AsMigrateFunc(sqliteCreateTablesQueries)

	// sqliteMigrations maps a schema version to the migration upgrading it to the next
	// version. Always append migrations at the end, never insert in the middle.
	sqliteMigrations = map[int]schema.MigrateFunc{
		1: sqliteMigrateFrom1,
		2: schema.AsMigrateFunc(sqliteMigrate2To3UpdateQueries),
		3: schema.AsMigrateFunc(sqliteMigrate3To4UpdateQueries),
		4: schema.AsMigrateFunc(sqliteMigrate4To5UpdateQueries),
		5: schema.AsMigrateFunc(sqliteMigrate5To6UpdateQueries),
		6: schema.AsMigrateFunc(sqliteMigrate6To7UpdateQueries),
		7: schema.AsMigrateFunc(sqliteMigrate7To8UpdateQueries),
		8: schema.AsMigrateFunc(sqliteMigrate8To9UpdateQueries),
	}
)

func runSQLiteStartupQueries(db *sql.DB, startupQueries string) error {
	if _, err := db.Exec(sqliteBuiltinStartupQueries); err != nil {
		return err
	}
	if startupQueries != "" {
		if _, err := db.Exec(startupQueries); err != nil {
			return err
		}
	}
	return nil
}

func sqliteMigrateFrom1(tx *sql.Tx) error {
	// Rename user -> user_old, and create new tables
	if _, err := tx.Exec(sqliteMigrate1To2CreateTablesQueries); err != nil {
		return err
	}
	// Insert users from user_old into new user table, with ID and sync_topic
	rows, err := tx.Query(sqliteMigrate1To2SelectAllOldUsernamesNoTxQuery)
	if err != nil {
		return err
	}
	defer rows.Close()
	usernames := make([]string, 0)
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return err
		}
		usernames = append(usernames, username)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, username := range usernames {
		userID := util.RandomStringPrefix(userIDPrefix, userIDLength)
		syncTopic := util.RandomStringPrefix(syncTopicPrefix, syncTopicLength)
		if _, err := tx.Exec(sqliteMigrate1To2InsertUserNoTxQuery, userID, syncTopic, username); err != nil {
			return err
		}
	}
	// Migrate old "access" table to "user_access" and drop "access" and "user_old"
	_, err = tx.Exec(sqliteMigrate1To2InsertFromOldTablesAndDropNoTxQuery)
	return err
}

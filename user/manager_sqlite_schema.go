package user

import (
	"database/sql"
	"fmt"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
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
			PRIMARY KEY (user_id, email),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO user (id, user, pass, role, sync_topic, provisioned, created)
		VALUES ('` + everyoneID + `', '*', '', 'anonymous', '', false, UNIXEPOCH())
		ON CONFLICT (id) DO NOTHING;
	`
)

const (
	sqliteBuiltinStartupQueries = `PRAGMA foreign_keys = ON;`
)

// Schema version table management for SQLite
const (
	sqliteCurrentSchemaVersion     = 7
	sqliteInsertSchemaVersionQuery = `INSERT INTO schemaVersion VALUES (1, ?)`
	sqliteUpdateSchemaVersionQuery = `UPDATE schemaVersion SET version = ? WHERE id = 1`
	sqliteSelectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`
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
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
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

	// 5 -> 6
	sqliteMigrate5To6UpdateQueries = `
		PRAGMA foreign_keys=off;

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

		-- Re-enable foreign keys
		PRAGMA foreign_keys=on;
	`
)

var (
	sqliteMigrations = map[int]func(db *sql.DB) error{
		1: sqliteMigrateFrom1,
		2: sqliteMigrateFrom2,
		3: sqliteMigrateFrom3,
		4: sqliteMigrateFrom4,
		5: sqliteMigrateFrom5,
		6: sqliteMigrateFrom6,
	}
)

func setupSQLite(db *sql.DB) error {
	var schemaVersion int
	if err := db.QueryRow(sqliteSelectSchemaVersionQuery).Scan(&schemaVersion); err != nil {
		return setupNewSQLite(db)
	}
	if schemaVersion == sqliteCurrentSchemaVersion {
		return nil
	} else if schemaVersion > sqliteCurrentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, sqliteCurrentSchemaVersion)
	}
	for i := schemaVersion; i < sqliteCurrentSchemaVersion; i++ {
		fn, ok := sqliteMigrations[i]
		if !ok {
			return fmt.Errorf("cannot find migration step from schema version %d to %d", i, i+1)
		} else if err := fn(db); err != nil {
			return err
		}
	}
	return nil
}

func setupNewSQLite(sqlDB *sql.DB) error {
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteCreateTablesQueries); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteInsertSchemaVersionQuery, sqliteCurrentSchemaVersion); err != nil {
			return err
		}
		return nil
	})
}

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

func sqliteMigrateFrom1(sqlDB *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 1 to 2")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
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
		if _, err := tx.Exec(sqliteMigrate1To2InsertFromOldTablesAndDropNoTxQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 2); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom2(sqlDB *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 2 to 3")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate2To3UpdateQueries); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 3); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom3(sqlDB *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 3 to 4")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate3To4UpdateQueries); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 4); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom4(sqlDB *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 4 to 5")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate4To5UpdateQueries); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 5); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom5(sqlDB *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 5 to 6")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate5To6UpdateQueries); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 6); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom6(sqlDB *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 6 to 7")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate6To7UpdateQueries); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 7); err != nil {
			return err
		}
		return nil
	})
}

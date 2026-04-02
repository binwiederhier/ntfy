package message

import (
	"database/sql"
	"fmt"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
)

// Initial SQLite schema
const (
	sqliteCreateTablesQuery = `
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mid TEXT NOT NULL,
			sequence_id TEXT NOT NULL,
			time INT NOT NULL,
			event TEXT NOT NULL,
			expires INT NOT NULL,
			topic TEXT NOT NULL,
			message TEXT NOT NULL,
			title TEXT NOT NULL,
			priority INT NOT NULL,
			tags TEXT NOT NULL,
			click TEXT NOT NULL,
			icon TEXT NOT NULL,
			actions TEXT NOT NULL,
			attachment_name TEXT NOT NULL,
			attachment_type TEXT NOT NULL,
			attachment_size INT NOT NULL,
			attachment_expires INT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_deleted INT NOT NULL,
			sender TEXT NOT NULL,
			user TEXT NOT NULL,
			content_type TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages (mid);
		CREATE INDEX IF NOT EXISTS idx_sequence_id ON messages (sequence_id);
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
		CREATE INDEX IF NOT EXISTS idx_expires ON messages (expires);
		CREATE INDEX IF NOT EXISTS idx_sender ON messages (sender);
		CREATE INDEX IF NOT EXISTS idx_user ON messages (user);
		CREATE INDEX IF NOT EXISTS idx_attachment_expires ON messages (attachment_expires);
		CREATE TABLE IF NOT EXISTS stats (
			key TEXT PRIMARY KEY,
			value INT
		);
		INSERT INTO stats (key, value) VALUES ('messages', 0);
	`
)

// Schema version management for SQLite
const (
	sqliteCurrentSchemaVersion          = 15
	sqliteCreateSchemaVersionTableQuery = `
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
	`
	sqliteInsertSchemaVersionQuery = `INSERT INTO schemaVersion VALUES (1, ?)`
	sqliteUpdateSchemaVersionQuery = `UPDATE schemaVersion SET version = ? WHERE id = 1`
	sqliteSelectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`
)

// Schema migrations for SQLite
const (
	// 0 -> 1
	sqliteMigrate0To1AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN title TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN priority INT NOT NULL DEFAULT(0);
		ALTER TABLE messages ADD COLUMN tags TEXT NOT NULL DEFAULT('');
	`

	// 1 -> 2
	sqliteMigrate1To2AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN published INT NOT NULL DEFAULT(1);
	`

	// 2 -> 3
	sqliteMigrate2To3AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN click TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_name TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_type TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_size INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN attachment_expires INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN attachment_owner TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_url TEXT NOT NULL DEFAULT('');
	`
	// 3 -> 4
	sqliteMigrate3To4AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN encoding TEXT NOT NULL DEFAULT('');
	`

	// 4 -> 5
	sqliteMigrate4To5AlterMessagesTableQuery = `
		CREATE TABLE IF NOT EXISTS messages_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mid TEXT NOT NULL,
			time INT NOT NULL,
			topic TEXT NOT NULL,
			message TEXT NOT NULL,
			title TEXT NOT NULL,
			priority INT NOT NULL,
			tags TEXT NOT NULL,
			click TEXT NOT NULL,
			attachment_name TEXT NOT NULL,
			attachment_type TEXT NOT NULL,
			attachment_size INT NOT NULL,
			attachment_expires INT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_owner TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published INT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_mid ON messages_new (mid);
		CREATE INDEX IF NOT EXISTS idx_topic ON messages_new (topic);
		INSERT
			INTO messages_new (
				mid, time, topic, message, title, priority, tags, click, attachment_name, attachment_type,
				attachment_size, attachment_expires, attachment_url, attachment_owner, encoding, published)
			SELECT
				id, time, topic, message, title, priority, tags, click, attachment_name, attachment_type,
				attachment_size, attachment_expires, attachment_url, attachment_owner, encoding, published
			FROM messages;
		DROP TABLE messages;
		ALTER TABLE messages_new RENAME TO messages;
	`

	// 5 -> 6
	sqliteMigrate5To6AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN actions TEXT NOT NULL DEFAULT('');
	`

	// 6 -> 7
	sqliteMigrate6To7AlterMessagesTableQuery = `
		ALTER TABLE messages RENAME COLUMN attachment_owner TO sender;
	`

	// 7 -> 8
	sqliteMigrate7To8AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN icon TEXT NOT NULL DEFAULT('');
	`

	// 8 -> 9
	sqliteMigrate8To9AlterMessagesTableQuery = `
		CREATE INDEX IF NOT EXISTS idx_time ON messages (time);
	`

	// 9 -> 10
	sqliteMigrate9To10AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN user TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN attachment_deleted INT NOT NULL DEFAULT('0');
		ALTER TABLE messages ADD COLUMN expires INT NOT NULL DEFAULT('0');
		CREATE INDEX IF NOT EXISTS idx_expires ON messages (expires);
		CREATE INDEX IF NOT EXISTS idx_sender ON messages (sender);
		CREATE INDEX IF NOT EXISTS idx_user ON messages (user);
		CREATE INDEX IF NOT EXISTS idx_attachment_expires ON messages (attachment_expires);
	`
	sqliteMigrate9To10UpdateMessageExpiryQuery = `UPDATE messages SET expires = time + ?`

	// 10 -> 11
	sqliteMigrate10To11AlterMessagesTableQuery = `
		CREATE TABLE IF NOT EXISTS stats (
			key TEXT PRIMARY KEY,
			value INT
		);
		INSERT INTO stats (key, value) VALUES ('messages', 0);
	`

	// 11 -> 12
	sqliteMigrate11To12AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN content_type TEXT NOT NULL DEFAULT('');
	`

	// 12 -> 13
	sqliteMigrate12To13AlterMessagesTableQuery = `
		CREATE INDEX IF NOT EXISTS idx_topic ON messages (topic);
	`

	// 13 -> 14
	sqliteMigrate13To14AlterMessagesTableQuery = `
		ALTER TABLE messages ADD COLUMN sequence_id TEXT NOT NULL DEFAULT('');
		ALTER TABLE messages ADD COLUMN event TEXT NOT NULL DEFAULT('message');
		CREATE INDEX IF NOT EXISTS idx_sequence_id ON messages (sequence_id);
	`
)

var (
	sqliteMigrations = map[int]func(db *sql.DB, cacheDuration time.Duration) error{
		0:  sqliteMigrateFrom0,
		1:  sqliteMigrateFrom1,
		2:  sqliteMigrateFrom2,
		3:  sqliteMigrateFrom3,
		4:  sqliteMigrateFrom4,
		5:  sqliteMigrateFrom5,
		6:  sqliteMigrateFrom6,
		7:  sqliteMigrateFrom7,
		8:  sqliteMigrateFrom8,
		9:  sqliteMigrateFrom9,
		10: sqliteMigrateFrom10,
		11: sqliteMigrateFrom11,
		12: sqliteMigrateFrom12,
		13: sqliteMigrateFrom13,
		14: sqliteMigrateFrom14,
	}
)

func setupSQLite(db *sql.DB, startupQueries string, cacheDuration time.Duration) error {
	if err := runSQLiteStartupQueries(db, startupQueries); err != nil {
		return err
	}
	// If 'messages' table does not exist, this must be a new database
	var messagesCount int
	if err := db.QueryRow(sqliteSelectMessagesCountQuery).Scan(&messagesCount); err != nil {
		return setupNewSQLite(db)
	}
	// If 'messages' table exists (schema >= 0), check 'schemaVersion' table
	var schemaVersion int
	db.QueryRow(sqliteSelectSchemaVersionQuery).Scan(&schemaVersion) // Error means schema version is zero!
	// Do migrations
	if schemaVersion == sqliteCurrentSchemaVersion {
		return nil
	} else if schemaVersion > sqliteCurrentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, sqliteCurrentSchemaVersion)
	}
	for i := schemaVersion; i < sqliteCurrentSchemaVersion; i++ {
		fn, ok := sqliteMigrations[i]
		if !ok {
			return fmt.Errorf("cannot find migration step from schema version %d to %d", i, i+1)
		} else if err := fn(db, cacheDuration); err != nil {
			return err
		}
	}
	return nil
}

func setupNewSQLite(sqlDB *sql.DB) error {
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteCreateTablesQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteCreateSchemaVersionTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteInsertSchemaVersionQuery, sqliteCurrentSchemaVersion); err != nil {
			return err
		}
		return nil
	})
}

func runSQLiteStartupQueries(db *sql.DB, startupQueries string) error {
	if startupQueries != "" {
		if _, err := db.Exec(startupQueries); err != nil {
			return err
		}
	}
	return nil
}

func sqliteMigrateFrom0(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 0 to 1")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate0To1AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteCreateSchemaVersionTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteInsertSchemaVersionQuery, 1); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom1(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 1 to 2")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate1To2AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 2); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom2(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 2 to 3")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate2To3AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 3); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom3(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 3 to 4")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate3To4AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 4); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom4(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 4 to 5")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate4To5AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 5); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom5(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 5 to 6")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate5To6AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 6); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom6(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 6 to 7")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate6To7AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 7); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom7(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 7 to 8")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate7To8AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 8); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom8(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 8 to 9")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate8To9AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 9); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom9(sqlDB *sql.DB, cacheDuration time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 9 to 10")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate9To10AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteMigrate9To10UpdateMessageExpiryQuery, int64(cacheDuration.Seconds())); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 10); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom10(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 10 to 11")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate10To11AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 11); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom11(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 11 to 12")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate11To12AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 12); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom12(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 12 to 13")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate12To13AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 13); err != nil {
			return err
		}
		return nil
	})
}

func sqliteMigrateFrom13(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 13 to 14")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteMigrate13To14AlterMessagesTableQuery); err != nil {
			return err
		}
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 14); err != nil {
			return err
		}
		return nil
	})
}

// sqliteMigrateFrom14 is a no-op; the corresponding Postgres migration adds
// idx_message_attachment_expires, which SQLite already has from the initial schema.
func sqliteMigrateFrom14(sqlDB *sql.DB, _ time.Duration) error {
	log.Tag(tagMessageCache).Info("Migrating cache database schema: from 14 to 15")
	return db.ExecTx(sqlDB, func(tx *sql.Tx) error {
		if _, err := tx.Exec(sqliteUpdateSchemaVersionQuery, 15); err != nil {
			return err
		}
		return nil
	})
}

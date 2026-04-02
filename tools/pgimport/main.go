// pgimport is a one-off migration script to import ntfy data from SQLite to PostgreSQL.
// It is not a generic migration tool. It expects specific schema versions for each database
// (message cache v14, user db v6, web push v1) and will refuse to run if versions don't match.
package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"gopkg.in/yaml.v2"
	"heckel.io/ntfy/v2/db/pg"
)

const (
	batchSize = 1000

	expectedMessageSchemaVersion = 14
	expectedUserSchemaVersion    = 6
	expectedWebPushSchemaVersion = 1

	everyoneID = "u_everyone"

	// Initial PostgreSQL schema for message store (from message/cache_postgres_schema.go)
	createMessageSchemaQuery = `
		CREATE TABLE IF NOT EXISTS message (
			id BIGSERIAL PRIMARY KEY,
			mid TEXT NOT NULL,
			sequence_id TEXT NOT NULL,
			time BIGINT NOT NULL,
			event TEXT NOT NULL,
			expires BIGINT NOT NULL,
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
			attachment_size BIGINT NOT NULL,
			attachment_expires BIGINT NOT NULL,
			attachment_url TEXT NOT NULL,
			attachment_deleted BOOLEAN NOT NULL DEFAULT FALSE,
			sender TEXT NOT NULL,
			user_id TEXT NOT NULL,
			content_type TEXT NOT NULL,
			encoding TEXT NOT NULL,
			published BOOLEAN NOT NULL DEFAULT FALSE
		);
		CREATE INDEX IF NOT EXISTS idx_message_mid ON message (mid);
		CREATE INDEX IF NOT EXISTS idx_message_sequence_id ON message (sequence_id);
		CREATE INDEX IF NOT EXISTS idx_message_topic_published_time ON message (topic, published, time, id);
		CREATE INDEX IF NOT EXISTS idx_message_published_expires ON message (published, expires);
		CREATE INDEX IF NOT EXISTS idx_message_sender_attachment_expires ON message (sender, attachment_expires) WHERE user_id = '';
		CREATE INDEX IF NOT EXISTS idx_message_user_id_attachment_expires ON message (user_id, attachment_expires);
		CREATE TABLE IF NOT EXISTS message_stats (
			key TEXT PRIMARY KEY,
			value BIGINT
		);
		INSERT INTO message_stats (key, value) VALUES ('messages', 0) ON CONFLICT (key) DO NOTHING;
		CREATE TABLE IF NOT EXISTS schema_version (
			store TEXT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO schema_version (store, version) VALUES ('message', 14) ON CONFLICT (store) DO NOTHING;
	`

	// Initial PostgreSQL schema for user store (from user/manager_postgres_schema.go)
	createUserSchemaQuery = `
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
		CREATE TABLE IF NOT EXISTS schema_version (
			store TEXT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO "user" (id, user_name, pass, role, sync_topic, provisioned, created)
		VALUES ('` + everyoneID + `', '*', '', 'anonymous', '', false, EXTRACT(EPOCH FROM NOW())::BIGINT)
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO schema_version (store, version) VALUES ('user', 6) ON CONFLICT (store) DO NOTHING;
	`

	// Initial PostgreSQL schema for web push store (from webpush/store_postgres.go)
	createWebPushSchemaQuery = `
		CREATE TABLE IF NOT EXISTS webpush_subscription (
			id TEXT PRIMARY KEY,
			endpoint TEXT NOT NULL UNIQUE,
			key_auth TEXT NOT NULL,
			key_p256dh TEXT NOT NULL,
			user_id TEXT NOT NULL,
			subscriber_ip TEXT NOT NULL,
			updated_at BIGINT NOT NULL,
			warned_at BIGINT NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_webpush_subscriber_ip ON webpush_subscription (subscriber_ip);
		CREATE INDEX IF NOT EXISTS idx_webpush_updated_at ON webpush_subscription (updated_at);
		CREATE INDEX IF NOT EXISTS idx_webpush_user_id ON webpush_subscription (user_id);
		CREATE TABLE IF NOT EXISTS webpush_subscription_topic (
			subscription_id TEXT NOT NULL REFERENCES webpush_subscription (id) ON DELETE CASCADE,
			topic TEXT NOT NULL,
			PRIMARY KEY (subscription_id, topic)
		);
		CREATE INDEX IF NOT EXISTS idx_webpush_topic ON webpush_subscription_topic (topic);
		CREATE TABLE IF NOT EXISTS schema_version (
			store TEXT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO schema_version (store, version) VALUES ('webpush', 1) ON CONFLICT (store) DO NOTHING;
	`
)

var flags = []cli.Flag{
	&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "path to server.yml config file"},
	altsrc.NewStringFlag(&cli.StringFlag{Name: "database-url", Aliases: []string{"database_url"}, Usage: "PostgreSQL connection string"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "cache-file", Aliases: []string{"cache_file"}, Usage: "SQLite message cache file path"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "auth-file", Aliases: []string{"auth_file"}, Usage: "SQLite user/auth database file path"}),
	altsrc.NewStringFlag(&cli.StringFlag{Name: "web-push-file", Aliases: []string{"web_push_file"}, Usage: "SQLite web push database file path"}),
	&cli.BoolFlag{Name: "create-schema", Usage: "create initial PostgreSQL schema before importing"},
	&cli.BoolFlag{Name: "pre-import", Usage: "pre-import messages while ntfy is still running (only imports messages)"},
}

func main() {
	app := &cli.App{
		Name:      "pgimport",
		Usage:     "One-off SQLite to PostgreSQL migration script for ntfy",
		UsageText: "pgimport [OPTIONS]",
		Flags:     flags,
		Before:    loadConfigFile("config", flags),
		Action:    execImport,
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execImport(c *cli.Context) error {
	databaseURL := c.String("database-url")
	cacheFile := c.String("cache-file")
	authFile := c.String("auth-file")
	webPushFile := c.String("web-push-file")
	preImport := c.Bool("pre-import")

	if databaseURL == "" {
		return fmt.Errorf("database-url must be set (via --database-url or config file)")
	}
	if preImport {
		if cacheFile == "" {
			return fmt.Errorf("--cache-file must be set when using --pre-import")
		}
		return execPreImport(c, databaseURL, cacheFile)
	}
	if cacheFile == "" && authFile == "" && webPushFile == "" {
		return fmt.Errorf("at least one of --cache-file, --auth-file, or --web-push-file must be set")
	}

	fmt.Println("pgimport - SQLite to PostgreSQL migration tool for ntfy")
	fmt.Println()
	fmt.Println("Sources:")
	printSource("  Cache file:    ", cacheFile)
	printSource("  Auth file:     ", authFile)
	printSource("  Web push file: ", webPushFile)
	fmt.Println()
	fmt.Println("Target:")
	fmt.Printf("  Database URL:  %s\n", maskPassword(databaseURL))
	fmt.Println()
	fmt.Println("This will import data from the SQLite databases into PostgreSQL.")
	fmt.Print("Make sure ntfy is not running. Continue? (y/n): ")

	var answer string
	fmt.Scanln(&answer)
	if strings.TrimSpace(strings.ToLower(answer)) != "y" {
		fmt.Println("Aborted.")
		return nil
	}
	fmt.Println()

	pgHost, err := pg.Open(databaseURL)
	if err != nil {
		return fmt.Errorf("cannot connect to PostgreSQL: %w", err)
	}
	pgDB := pgHost.DB
	defer pgDB.Close()

	if c.Bool("create-schema") {
		if err := createSchema(pgDB, cacheFile, authFile, webPushFile); err != nil {
			return fmt.Errorf("cannot create schema: %w", err)
		}
	}

	if authFile != "" {
		if err := verifySchemaVersion(pgDB, "user", expectedUserSchemaVersion); err != nil {
			return err
		}
		if err := importUsers(authFile, pgDB); err != nil {
			return fmt.Errorf("cannot import users: %w", err)
		}
	}
	if cacheFile != "" {
		if err := verifySchemaVersion(pgDB, "message", expectedMessageSchemaVersion); err != nil {
			return err
		}
		sinceTime := maxMessageTime(pgDB)
		if err := importMessages(cacheFile, pgDB, sinceTime); err != nil {
			return fmt.Errorf("cannot import messages: %w", err)
		}
	}
	if webPushFile != "" {
		if err := verifySchemaVersion(pgDB, "webpush", expectedWebPushSchemaVersion); err != nil {
			return err
		}
		if err := importWebPush(webPushFile, pgDB); err != nil {
			return fmt.Errorf("cannot import web push subscriptions: %w", err)
		}
	}

	fmt.Println()
	fmt.Println("Verifying migration ...")
	failed := false
	if authFile != "" {
		if err := verifyUsers(authFile, pgDB, &failed); err != nil {
			return fmt.Errorf("cannot verify users: %w", err)
		}
	}
	if cacheFile != "" {
		if err := verifyMessages(cacheFile, pgDB, &failed); err != nil {
			return fmt.Errorf("cannot verify messages: %w", err)
		}
	}
	if webPushFile != "" {
		if err := verifyWebPush(webPushFile, pgDB, &failed); err != nil {
			return fmt.Errorf("cannot verify web push: %w", err)
		}
	}
	fmt.Println()
	if failed {
		return fmt.Errorf("verification FAILED, see above for details")
	}
	fmt.Println("Verification successful. Migration complete.")
	return nil
}

func execPreImport(c *cli.Context, databaseURL, cacheFile string) error {
	fmt.Println("pgimport - PRE-IMPORT mode (ntfy can keep running)")
	fmt.Println()
	fmt.Println("Source:")
	printSource("  Cache file:    ", cacheFile)
	fmt.Println()
	fmt.Println("Target:")
	fmt.Printf("  Database URL:  %s\n", maskPassword(databaseURL))
	fmt.Println()
	fmt.Println("This will pre-import messages into PostgreSQL while ntfy is still running.")
	fmt.Println("After this completes, stop ntfy and run pgimport again without --pre-import")
	fmt.Println("to import remaining messages, users, and web push subscriptions.")
	fmt.Print("Continue? (y/n): ")

	var answer string
	fmt.Scanln(&answer)
	if strings.TrimSpace(strings.ToLower(answer)) != "y" {
		fmt.Println("Aborted.")
		return nil
	}
	fmt.Println()

	pgHost, err := pg.Open(databaseURL)
	if err != nil {
		return fmt.Errorf("cannot connect to PostgreSQL: %w", err)
	}
	pgDB := pgHost.DB
	defer pgDB.Close()

	if c.Bool("create-schema") {
		if err := createSchema(pgDB, cacheFile, "", ""); err != nil {
			return fmt.Errorf("cannot create schema: %w", err)
		}
	}

	if err := verifySchemaVersion(pgDB, "message", expectedMessageSchemaVersion); err != nil {
		return err
	}
	if err := importMessages(cacheFile, pgDB, 0); err != nil {
		return fmt.Errorf("cannot import messages: %w", err)
	}

	fmt.Println()
	fmt.Println("Pre-import complete. Now stop ntfy and run pgimport again without --pre-import")
	fmt.Println("to import any remaining messages, users, and web push subscriptions.")
	return nil
}

func createSchema(pgDB *sql.DB, cacheFile, authFile, webPushFile string) error {
	fmt.Println("Creating initial PostgreSQL schema ...")
	// User schema must be created before message schema, because message_stats and
	// schema_version use "INSERT INTO" without "ON CONFLICT", so user schema (which
	// also creates the schema_version table) must come first.
	if authFile != "" {
		fmt.Println("  Creating user schema ...")
		if _, err := pgDB.Exec(createUserSchemaQuery); err != nil {
			return fmt.Errorf("creating user schema: %w", err)
		}
	}
	if cacheFile != "" {
		fmt.Println("  Creating message schema ...")
		if _, err := pgDB.Exec(createMessageSchemaQuery); err != nil {
			return fmt.Errorf("creating message schema: %w", err)
		}
	}
	if webPushFile != "" {
		fmt.Println("  Creating web push schema ...")
		if _, err := pgDB.Exec(createWebPushSchemaQuery); err != nil {
			return fmt.Errorf("creating web push schema: %w", err)
		}
	}
	fmt.Println("  Schema creation complete.")
	fmt.Println()
	return nil
}

func loadConfigFile(configFlag string, flags []cli.Flag) cli.BeforeFunc {
	return func(c *cli.Context) error {
		configFile := c.String(configFlag)
		if configFile == "" {
			return nil
		}
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return fmt.Errorf("config file %s does not exist", configFile)
		}
		inputSource, err := newYamlSourceFromFile(configFile, flags)
		if err != nil {
			return err
		}
		return altsrc.ApplyInputSourceValues(c, inputSource, flags)
	}
}

func newYamlSourceFromFile(file string, flags []cli.Flag) (altsrc.InputSourceContext, error) {
	var rawConfig map[any]any
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &rawConfig); err != nil {
		return nil, err
	}
	for _, f := range flags {
		flagName := f.Names()[0]
		for _, flagAlias := range f.Names()[1:] {
			if _, ok := rawConfig[flagAlias]; ok {
				rawConfig[flagName] = rawConfig[flagAlias]
			}
		}
	}
	return altsrc.NewMapInputSource(file, rawConfig), nil
}

func verifySchemaVersion(pgDB *sql.DB, store string, expected int) error {
	var version int
	err := pgDB.QueryRow(`SELECT version FROM schema_version WHERE store = $1`, store).Scan(&version)
	if err != nil {
		return fmt.Errorf("cannot read %s schema version from PostgreSQL (is the schema set up?): %w", store, err)
	}
	if version != expected {
		return fmt.Errorf("%s schema version mismatch: expected %d, got %d", store, expected, version)
	}
	return nil
}

func printSource(label, path string) {
	if path == "" {
		fmt.Printf("%s(not set, skipping)\n", label)
	} else if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("%s%s (NOT FOUND, skipping)\n", label, path)
	} else {
		fmt.Printf("%s%s\n", label, path)
	}
}

func maskPassword(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return databaseURL
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			masked := u.Scheme + "://" + u.User.Username() + ":****@" + u.Host + u.Path
			if u.RawQuery != "" {
				masked += "?" + u.RawQuery
			}
			return masked
		}
	}
	return u.String()
}

func openSQLite(filename string) (*sql.DB, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, fmt.Errorf("file %s does not exist", filename)
	}
	return sql.Open("sqlite3", filename+"?mode=ro")
}

// User import

func importUsers(sqliteFile string, pgDB *sql.DB) error {
	sqlDB, err := openSQLite(sqliteFile)
	if err != nil {
		fmt.Printf("Skipping user import: %s\n", err)
		return nil
	}
	defer sqlDB.Close()
	fmt.Printf("Importing users from %s ...\n", sqliteFile)

	count, err := importTiers(sqlDB, pgDB)
	if err != nil {
		return fmt.Errorf("importing tiers: %w", err)
	}
	fmt.Printf("  Imported %d tiers\n", count)

	count, err = importUserRows(sqlDB, pgDB)
	if err != nil {
		return fmt.Errorf("importing users: %w", err)
	}
	fmt.Printf("  Imported %d users\n", count)

	count, err = importUserAccess(sqlDB, pgDB)
	if err != nil {
		return fmt.Errorf("importing user access: %w", err)
	}
	fmt.Printf("  Imported %d access entries\n", count)

	count, err = importUserTokens(sqlDB, pgDB)
	if err != nil {
		return fmt.Errorf("importing user tokens: %w", err)
	}
	fmt.Printf("  Imported %d tokens\n", count)

	count, err = importUserPhones(sqlDB, pgDB)
	if err != nil {
		return fmt.Errorf("importing user phones: %w", err)
	}
	fmt.Printf("  Imported %d phone numbers\n", count)

	return nil
}

func importTiers(sqlDB, pgDB *sql.DB) (int, error) {
	rows, err := sqlDB.Query(`SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id FROM tier`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := pgDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO tier (id, code, name, messages_limit, messages_expiry_duration, emails_limit, calls_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var id, code, name string
		var messagesLimit, messagesExpiryDuration, emailsLimit, callsLimit, reservationsLimit int64
		var attachmentFileSizeLimit, attachmentTotalSizeLimit, attachmentExpiryDuration, attachmentBandwidthLimit int64
		var stripeMonthlyPriceID, stripeYearlyPriceID sql.NullString
		if err := rows.Scan(&id, &code, &name, &messagesLimit, &messagesExpiryDuration, &emailsLimit, &callsLimit, &reservationsLimit, &attachmentFileSizeLimit, &attachmentTotalSizeLimit, &attachmentExpiryDuration, &attachmentBandwidthLimit, &stripeMonthlyPriceID, &stripeYearlyPriceID); err != nil {
			return 0, err
		}
		if _, err := stmt.Exec(id, code, name, messagesLimit, messagesExpiryDuration, emailsLimit, callsLimit, reservationsLimit, attachmentFileSizeLimit, attachmentTotalSizeLimit, attachmentExpiryDuration, attachmentBandwidthLimit, stripeMonthlyPriceID, stripeYearlyPriceID); err != nil {
			return 0, err
		}
		count++
	}
	return count, tx.Commit()
}

func importUserRows(sqlDB, pgDB *sql.DB) (int, error) {
	rows, err := sqlDB.Query(`SELECT id, user, pass, role, prefs, sync_topic, provisioned, stats_messages, stats_emails, stats_calls, stripe_customer_id, stripe_subscription_id, stripe_subscription_status, stripe_subscription_interval, stripe_subscription_paid_until, stripe_subscription_cancel_at, created, deleted, tier_id FROM user`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := pgDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO "user" (id, user_name, pass, role, prefs, sync_topic, provisioned, stats_messages, stats_emails, stats_calls, stripe_customer_id, stripe_subscription_id, stripe_subscription_status, stripe_subscription_interval, stripe_subscription_paid_until, stripe_subscription_cancel_at, created, deleted, tier_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var id, userName, pass, role, prefs, syncTopic string
		var provisioned int
		var statsMessages, statsEmails, statsCalls int64
		var stripeCustomerID, stripeSubscriptionID, stripeSubscriptionStatus, stripeSubscriptionInterval sql.NullString
		var stripeSubscriptionPaidUntil, stripeSubscriptionCancelAt sql.NullInt64
		var created int64
		var deleted sql.NullInt64
		var tierID sql.NullString
		if err := rows.Scan(&id, &userName, &pass, &role, &prefs, &syncTopic, &provisioned, &statsMessages, &statsEmails, &statsCalls, &stripeCustomerID, &stripeSubscriptionID, &stripeSubscriptionStatus, &stripeSubscriptionInterval, &stripeSubscriptionPaidUntil, &stripeSubscriptionCancelAt, &created, &deleted, &tierID); err != nil {
			return 0, err
		}
		provisionedBool := provisioned != 0
		if _, err := stmt.Exec(id, userName, pass, role, prefs, syncTopic, provisionedBool, statsMessages, statsEmails, statsCalls, stripeCustomerID, stripeSubscriptionID, stripeSubscriptionStatus, stripeSubscriptionInterval, stripeSubscriptionPaidUntil, stripeSubscriptionCancelAt, created, deleted, tierID); err != nil {
			return 0, err
		}
		count++
	}
	return count, tx.Commit()
}

func importUserAccess(sqlDB, pgDB *sql.DB) (int, error) {
	rows, err := sqlDB.Query(`SELECT a.user_id, a.topic, a.read, a.write, a.owner_user_id, a.provisioned FROM user_access a JOIN user u ON u.id = a.user_id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := pgDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO user_access (user_id, topic, read, write, owner_user_id, provisioned) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (user_id, topic) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var userID, topic string
		var read, write, provisioned int
		var ownerUserID sql.NullString
		if err := rows.Scan(&userID, &topic, &read, &write, &ownerUserID, &provisioned); err != nil {
			return 0, err
		}
		readBool := read != 0
		writeBool := write != 0
		provisionedBool := provisioned != 0
		if _, err := stmt.Exec(userID, topic, readBool, writeBool, ownerUserID, provisionedBool); err != nil {
			return 0, err
		}
		count++
	}
	return count, tx.Commit()
}

func importUserTokens(sqlDB, pgDB *sql.DB) (int, error) {
	rows, err := sqlDB.Query(`SELECT t.user_id, t.token, t.label, t.last_access, t.last_origin, t.expires, t.provisioned FROM user_token t JOIN user u ON u.id = t.user_id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := pgDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO user_token (user_id, token, label, last_access, last_origin, expires, provisioned) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (user_id, token) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var userID, token, label, lastOrigin string
		var lastAccess, expires int64
		var provisioned int
		if err := rows.Scan(&userID, &token, &label, &lastAccess, &lastOrigin, &expires, &provisioned); err != nil {
			return 0, err
		}
		provisionedBool := provisioned != 0
		if _, err := stmt.Exec(userID, token, label, lastAccess, lastOrigin, expires, provisionedBool); err != nil {
			return 0, err
		}
		count++
	}
	return count, tx.Commit()
}

func importUserPhones(sqlDB, pgDB *sql.DB) (int, error) {
	rows, err := sqlDB.Query(`SELECT p.user_id, p.phone_number FROM user_phone p JOIN user u ON u.id = p.user_id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := pgDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO user_phone (user_id, phone_number) VALUES ($1, $2) ON CONFLICT (user_id, phone_number) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var userID, phoneNumber string
		if err := rows.Scan(&userID, &phoneNumber); err != nil {
			return 0, err
		}
		if _, err := stmt.Exec(userID, phoneNumber); err != nil {
			return 0, err
		}
		count++
	}
	return count, tx.Commit()
}

// Message import

const preImportTimeDelta = 30 // seconds to subtract from max time to account for in-flight messages

// maxMessageTime returns the maximum message time in PostgreSQL minus a small buffer,
// or 0 if there are no messages yet. This is used after a --pre-import run to only
// import messages that arrived since the pre-import.
func maxMessageTime(pgDB *sql.DB) int64 {
	var maxTime sql.NullInt64
	if err := pgDB.QueryRow(`SELECT MAX(time) FROM message`).Scan(&maxTime); err != nil || !maxTime.Valid || maxTime.Int64 == 0 {
		return 0
	}
	sinceTime := maxTime.Int64 - preImportTimeDelta
	if sinceTime < 0 {
		return 0
	}
	fmt.Printf("Pre-imported messages detected (max time: %d), importing delta (since time %d) ...\n", maxTime.Int64, sinceTime)
	return sinceTime
}

func importMessages(sqliteFile string, pgDB *sql.DB, sinceTime int64) error {
	sqlDB, err := openSQLite(sqliteFile)
	if err != nil {
		fmt.Printf("Skipping message import: %s\n", err)
		return nil
	}
	defer sqlDB.Close()

	query := `SELECT mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, attachment_deleted, sender, user, content_type, encoding, published FROM messages`
	var rows *sql.Rows
	if sinceTime > 0 {
		fmt.Printf("Importing messages from %s (since time %d) ...\n", sqliteFile, sinceTime)
		rows, err = sqlDB.Query(query+` WHERE time >= ?`, sinceTime)
	} else {
		fmt.Printf("Importing messages from %s ...\n", sqliteFile)
		rows, err = sqlDB.Query(query)
	}
	if err != nil {
		return fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	if _, err := pgDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_message_mid_unique ON message (mid)`); err != nil {
		return fmt.Errorf("creating unique index on mid: %w", err)
	}

	insertQuery := `INSERT INTO message (mid, sequence_id, time, event, expires, topic, message, title, priority, tags, click, icon, actions, attachment_name, attachment_type, attachment_size, attachment_expires, attachment_url, attachment_deleted, sender, user_id, content_type, encoding, published) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24) ON CONFLICT (mid) DO NOTHING`

	count := 0
	batchCount := 0
	tx, err := pgDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertQuery)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var mid, sequenceID, event, topic, message, title, tags, click, icon, actions string
		var attachmentName, attachmentType, attachmentURL, sender, userID, contentType, encoding string
		var msgTime, expires, attachmentExpires int64
		var priority int
		var attachmentSize int64
		var attachmentDeleted, published int
		if err := rows.Scan(&mid, &sequenceID, &msgTime, &event, &expires, &topic, &message, &title, &priority, &tags, &click, &icon, &actions, &attachmentName, &attachmentType, &attachmentSize, &attachmentExpires, &attachmentURL, &attachmentDeleted, &sender, &userID, &contentType, &encoding, &published); err != nil {
			return fmt.Errorf("scanning message: %w", err)
		}
		mid = toUTF8(mid)
		sequenceID = toUTF8(sequenceID)
		event = toUTF8(event)
		topic = toUTF8(topic)
		message = toUTF8(message)
		title = toUTF8(title)
		tags = toUTF8(tags)
		click = toUTF8(click)
		icon = toUTF8(icon)
		actions = toUTF8(actions)
		attachmentName = toUTF8(attachmentName)
		attachmentType = toUTF8(attachmentType)
		attachmentURL = toUTF8(attachmentURL)
		sender = toUTF8(sender)
		userID = toUTF8(userID)
		contentType = toUTF8(contentType)
		encoding = toUTF8(encoding)
		attachmentDeletedBool := attachmentDeleted != 0
		publishedBool := published != 0
		if _, err := stmt.Exec(mid, sequenceID, msgTime, event, expires, topic, message, title, priority, tags, click, icon, actions, attachmentName, attachmentType, attachmentSize, attachmentExpires, attachmentURL, attachmentDeletedBool, sender, userID, contentType, encoding, publishedBool); err != nil {
			return fmt.Errorf("inserting message: %w", err)
		}
		count++
		batchCount++
		if batchCount >= batchSize {
			stmt.Close()
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("committing message batch: %w", err)
			}
			fmt.Printf("  ... %d messages\n", count)
			tx, err = pgDB.Begin()
			if err != nil {
				return err
			}
			stmt, err = tx.Prepare(insertQuery)
			if err != nil {
				return err
			}
			batchCount = 0
		}
	}
	if batchCount > 0 {
		stmt.Close()
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing final message batch: %w", err)
		}
	}
	fmt.Printf("  Imported %d messages\n", count)

	var statsValue int64
	err = sqlDB.QueryRow(`SELECT value FROM stats WHERE key = 'messages'`).Scan(&statsValue)
	if err == nil {
		if _, err := pgDB.Exec(`UPDATE message_stats SET value = $1 WHERE key = 'messages'`, statsValue); err != nil {
			return fmt.Errorf("updating message stats: %w", err)
		}
		fmt.Printf("  Updated message stats (count: %d)\n", statsValue)
	}

	return nil
}

// Web push import

func importWebPush(sqliteFile string, pgDB *sql.DB) error {
	sqlDB, err := openSQLite(sqliteFile)
	if err != nil {
		fmt.Printf("Skipping web push import: %s\n", err)
		return nil
	}
	defer sqlDB.Close()
	fmt.Printf("Importing web push subscriptions from %s ...\n", sqliteFile)

	rows, err := sqlDB.Query(`SELECT id, endpoint, key_auth, key_p256dh, user_id, subscriber_ip, updated_at, warned_at FROM subscription`)
	if err != nil {
		return fmt.Errorf("querying subscriptions: %w", err)
	}
	defer rows.Close()

	tx, err := pgDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO webpush_subscription (id, endpoint, key_auth, key_p256dh, user_id, subscriber_ip, updated_at, warned_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		var id, endpoint, keyAuth, keyP256dh, userID, subscriberIP string
		var updatedAt, warnedAt int64
		if err := rows.Scan(&id, &endpoint, &keyAuth, &keyP256dh, &userID, &subscriberIP, &updatedAt, &warnedAt); err != nil {
			return fmt.Errorf("scanning subscription: %w", err)
		}
		if _, err := stmt.Exec(id, endpoint, keyAuth, keyP256dh, userID, subscriberIP, updatedAt, warnedAt); err != nil {
			return fmt.Errorf("inserting subscription: %w", err)
		}
		count++
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing subscriptions: %w", err)
	}
	fmt.Printf("  Imported %d subscriptions\n", count)

	topicRows, err := sqlDB.Query(`SELECT subscription_id, topic FROM subscription_topic`)
	if err != nil {
		return fmt.Errorf("querying subscription topics: %w", err)
	}
	defer topicRows.Close()

	tx, err = pgDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err = tx.Prepare(`INSERT INTO webpush_subscription_topic (subscription_id, topic) VALUES ($1, $2) ON CONFLICT (subscription_id, topic) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	topicCount := 0
	for topicRows.Next() {
		var subscriptionID, topic string
		if err := topicRows.Scan(&subscriptionID, &topic); err != nil {
			return fmt.Errorf("scanning subscription topic: %w", err)
		}
		if _, err := stmt.Exec(subscriptionID, topic); err != nil {
			return fmt.Errorf("inserting subscription topic: %w", err)
		}
		topicCount++
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing subscription topics: %w", err)
	}
	fmt.Printf("  Imported %d subscription topics\n", topicCount)

	return nil
}

func toUTF8(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = strings.ReplaceAll(s, "\x00", "")
	return s
}

// Verification

func verifyUsers(sqliteFile string, pgDB *sql.DB, failed *bool) error {
	sqlDB, err := openSQLite(sqliteFile)
	if err != nil {
		return nil
	}
	defer sqlDB.Close()

	verifyCount(sqlDB, pgDB, "tier", `SELECT COUNT(*) FROM tier`, `SELECT COUNT(*) FROM tier`, failed)
	verifyContent(sqlDB, pgDB, "tier",
		`SELECT id, code, name FROM tier ORDER BY id`,
		`SELECT id, code, name FROM tier ORDER BY id COLLATE "C"`,
		failed)

	verifyCount(sqlDB, pgDB, "user", `SELECT COUNT(*) FROM user`, `SELECT COUNT(*) FROM "user"`, failed)
	verifyContent(sqlDB, pgDB, "user",
		`SELECT id, user, role, sync_topic FROM user ORDER BY id`,
		`SELECT id, user_name, role, sync_topic FROM "user" ORDER BY id COLLATE "C"`,
		failed)

	verifyCount(sqlDB, pgDB, "user_access", `SELECT COUNT(*) FROM user_access a JOIN user u ON u.id = a.user_id`, `SELECT COUNT(*) FROM user_access`, failed)
	verifyContent(sqlDB, pgDB, "user_access",
		`SELECT a.user_id, a.topic FROM user_access a JOIN user u ON u.id = a.user_id ORDER BY a.user_id, a.topic`,
		`SELECT user_id, topic FROM user_access ORDER BY user_id COLLATE "C", topic COLLATE "C"`,
		failed)

	verifyCount(sqlDB, pgDB, "user_token", `SELECT COUNT(*) FROM user_token t JOIN user u ON u.id = t.user_id`, `SELECT COUNT(*) FROM user_token`, failed)
	verifyContent(sqlDB, pgDB, "user_token",
		`SELECT t.user_id, t.token, t.label FROM user_token t JOIN user u ON u.id = t.user_id ORDER BY t.user_id, t.token`,
		`SELECT user_id, token, label FROM user_token ORDER BY user_id COLLATE "C", token COLLATE "C"`,
		failed)

	verifyCount(sqlDB, pgDB, "user_phone", `SELECT COUNT(*) FROM user_phone p JOIN user u ON u.id = p.user_id`, `SELECT COUNT(*) FROM user_phone`, failed)
	verifyContent(sqlDB, pgDB, "user_phone",
		`SELECT p.user_id, p.phone_number FROM user_phone p JOIN user u ON u.id = p.user_id ORDER BY p.user_id, p.phone_number`,
		`SELECT user_id, phone_number FROM user_phone ORDER BY user_id COLLATE "C", phone_number COLLATE "C"`,
		failed)

	return nil
}

func verifyMessages(sqliteFile string, pgDB *sql.DB, failed *bool) error {
	sqlDB, err := openSQLite(sqliteFile)
	if err != nil {
		return nil
	}
	defer sqlDB.Close()

	verifyCount(sqlDB, pgDB, "messages", `SELECT COUNT(*) FROM messages`, `SELECT COUNT(*) FROM message`, failed)
	verifySampledMessages(sqlDB, pgDB, failed)
	return nil
}

func verifyWebPush(sqliteFile string, pgDB *sql.DB, failed *bool) error {
	sqlDB, err := openSQLite(sqliteFile)
	if err != nil {
		return nil
	}
	defer sqlDB.Close()

	verifyCount(sqlDB, pgDB, "subscription", `SELECT COUNT(*) FROM subscription`, `SELECT COUNT(*) FROM webpush_subscription`, failed)
	verifyContent(sqlDB, pgDB, "subscription",
		`SELECT id, endpoint, key_auth, key_p256dh, user_id FROM subscription ORDER BY id`,
		`SELECT id, endpoint, key_auth, key_p256dh, user_id FROM webpush_subscription ORDER BY id COLLATE "C"`,
		failed)

	verifyCount(sqlDB, pgDB, "subscription_topic", `SELECT COUNT(*) FROM subscription_topic`, `SELECT COUNT(*) FROM webpush_subscription_topic`, failed)
	verifyContent(sqlDB, pgDB, "subscription_topic",
		`SELECT subscription_id, topic FROM subscription_topic ORDER BY subscription_id, topic`,
		`SELECT subscription_id, topic FROM webpush_subscription_topic ORDER BY subscription_id COLLATE "C", topic COLLATE "C"`,
		failed)

	return nil
}

func verifyCount(sqlDB, pgDB *sql.DB, table, sqliteQuery, pgQuery string, failed *bool) {
	var sqliteCount, pgCount int64
	if err := sqlDB.QueryRow(sqliteQuery).Scan(&sqliteCount); err != nil {
		fmt.Printf("  %-25s count ERROR reading SQLite: %s\n", table, err)
		*failed = true
		return
	}
	if err := pgDB.QueryRow(pgQuery).Scan(&pgCount); err != nil {
		fmt.Printf("  %-25s count ERROR reading PostgreSQL: %s\n", table, err)
		*failed = true
		return
	}
	if sqliteCount == pgCount {
		fmt.Printf("  %-25s count OK (%d rows)\n", table, pgCount)
	} else {
		fmt.Printf("  %-25s count MISMATCH: SQLite=%d, PostgreSQL=%d\n", table, sqliteCount, pgCount)
		*failed = true
	}
}

func verifyContent(sqlDB, pgDB *sql.DB, table, sqliteQuery, pgQuery string, failed *bool) {
	sqliteRows, err := sqlDB.Query(sqliteQuery)
	if err != nil {
		fmt.Printf("  %-25s content ERROR reading SQLite: %s\n", table, err)
		*failed = true
		return
	}
	defer sqliteRows.Close()

	pgRows, err := pgDB.Query(pgQuery)
	if err != nil {
		fmt.Printf("  %-25s content ERROR reading PostgreSQL: %s\n", table, err)
		*failed = true
		return
	}
	defer pgRows.Close()

	cols, err := sqliteRows.Columns()
	if err != nil {
		fmt.Printf("  %-25s content ERROR reading columns: %s\n", table, err)
		*failed = true
		return
	}
	numCols := len(cols)

	rowNum := 0
	mismatches := 0
	for sqliteRows.Next() {
		rowNum++
		if !pgRows.Next() {
			fmt.Printf("  %-25s content MISMATCH: PostgreSQL has fewer rows (at row %d)\n", table, rowNum)
			*failed = true
			return
		}
		sqliteVals := makeStringSlice(numCols)
		pgVals := makeStringSlice(numCols)
		if err := sqliteRows.Scan(sqliteVals...); err != nil {
			fmt.Printf("  %-25s content ERROR scanning SQLite row %d: %s\n", table, rowNum, err)
			*failed = true
			return
		}
		if err := pgRows.Scan(pgVals...); err != nil {
			fmt.Printf("  %-25s content ERROR scanning PostgreSQL row %d: %s\n", table, rowNum, err)
			*failed = true
			return
		}
		for i := 0; i < numCols; i++ {
			sv := *(sqliteVals[i].(*sql.NullString))
			pv := *(pgVals[i].(*sql.NullString))
			if sv != pv {
				mismatches++
				if mismatches <= 3 {
					fmt.Printf("  %-25s content MISMATCH at row %d, col %s: SQLite=%q, PostgreSQL=%q\n", table, rowNum, cols[i], sv.String, pv.String)
				}
			}
		}
	}
	if pgRows.Next() {
		fmt.Printf("  %-25s content MISMATCH: PostgreSQL has more rows than SQLite\n", table)
		*failed = true
		return
	}
	if mismatches > 0 {
		if mismatches > 3 {
			fmt.Printf("  %-25s content ... and %d more mismatches\n", table, mismatches-3)
		}
		*failed = true
	} else {
		fmt.Printf("  %-25s content OK\n", table)
	}
}

func verifySampledMessages(sqlDB, pgDB *sql.DB, failed *bool) {
	rows, err := sqlDB.Query(`SELECT mid, topic, time, message, title, tags, priority FROM messages ORDER BY mid`)
	if err != nil {
		fmt.Printf("  %-25s content ERROR reading SQLite: %s\n", "messages (sampled)", err)
		*failed = true
		return
	}
	defer rows.Close()

	rowNum := 0
	checked := 0
	mismatches := 0
	for rows.Next() {
		rowNum++
		var mid, topic, message, title, tags string
		var msgTime int64
		var priority int
		if err := rows.Scan(&mid, &topic, &msgTime, &message, &title, &tags, &priority); err != nil {
			fmt.Printf("  %-25s content ERROR scanning SQLite row %d: %s\n", "messages (sampled)", rowNum, err)
			*failed = true
			return
		}
		if rowNum%100 != 1 {
			continue
		}
		checked++
		var pgTopic, pgMessage, pgTitle, pgTags string
		var pgTime int64
		var pgPriority int
		err := pgDB.QueryRow(`SELECT topic, time, message, title, tags, priority FROM message WHERE mid = $1`, mid).
			Scan(&pgTopic, &pgTime, &pgMessage, &pgTitle, &pgTags, &pgPriority)
		if err == sql.ErrNoRows {
			mismatches++
			if mismatches <= 3 {
				fmt.Printf("  %-25s content MISMATCH: mid=%s not found in PostgreSQL\n", "messages (sampled)", mid)
			}
			continue
		} else if err != nil {
			fmt.Printf("  %-25s content ERROR querying PostgreSQL for mid=%s: %s\n", "messages (sampled)", mid, err)
			*failed = true
			return
		}
		topic = toUTF8(topic)
		message = toUTF8(message)
		title = toUTF8(title)
		tags = toUTF8(tags)
		if topic != pgTopic || msgTime != pgTime || message != pgMessage || title != pgTitle || tags != pgTags || priority != pgPriority {
			mismatches++
			if mismatches <= 3 {
				fmt.Printf("  %-25s content MISMATCH at mid=%s\n", "messages (sampled)", mid)
			}
		}
	}
	if mismatches > 0 {
		if mismatches > 3 {
			fmt.Printf("  %-25s content ... and %d more mismatches\n", "messages (sampled)", mismatches-3)
		}
		*failed = true
	} else {
		fmt.Printf("  %-25s content OK (%d samples checked)\n", "messages (sampled)", checked)
	}
}

func makeStringSlice(n int) []any {
	vals := make([]any, n)
	for i := range vals {
		vals[i] = &sql.NullString{}
	}
	return vals
}

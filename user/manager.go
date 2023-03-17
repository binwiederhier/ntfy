// Package user deals with authentication and authorization against topics
package user

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"github.com/stripe/stripe-go/v74"
	"golang.org/x/crypto/bcrypt"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	tierIDPrefix                    = "ti_"
	tierIDLength                    = 8
	syncTopicPrefix                 = "st_"
	syncTopicLength                 = 16
	userIDPrefix                    = "u_"
	userIDLength                    = 12
	userAuthIntentionalSlowDownHash = "$2a$10$YFCQvqQDwIIwnJM1xkAYOeih0dg17UVGanaTStnrSzC8NCWxcLDwy" // Cost should match DefaultUserPasswordBcryptCost
	userHardDeleteAfterDuration     = 7 * 24 * time.Hour
	tokenPrefix                     = "tk_"
	tokenLength                     = 32
	tokenMaxCount                   = 20 // Only keep this many tokens in the table per user
	tag                             = "user_manager"
)

// Default constants that may be overridden by configs
const (
	DefaultUserStatsQueueWriterInterval = 33 * time.Second
	DefaultUserPasswordBcryptCost       = 10
)

var (
	errNoTokenProvided    = errors.New("no token provided")
	errTopicOwnedByOthers = errors.New("topic owned by others")
	errNoRows             = errors.New("no rows found")
)

// Manager-related queries
const (
	createTablesQueries = `
		BEGIN;
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
			stats_messages INT NOT NULL DEFAULT (0),
			stats_emails INT NOT NULL DEFAULT (0),
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
		VALUES ('` + everyoneID + `', '*', '', 'anonymous', '', UNIXEPOCH())
		ON CONFLICT (id) DO NOTHING;
		COMMIT;
	`
	builtinStartupQueries = `
		PRAGMA foreign_keys = ON;
	`

	selectUserByIDQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.stats_messages, u.stats_emails, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE u.id = ?
	`
	selectUserByNameQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.stats_messages, u.stats_emails, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE user = ?
	`
	selectUserByTokenQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.stats_messages, u.stats_emails, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		JOIN user_token tk on u.id = tk.user_id
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE tk.token = ? AND (tk.expires = 0 OR tk.expires >= ?)
	`
	selectUserByStripeCustomerIDQuery = `
		SELECT u.id, u.user, u.pass, u.role, u.prefs, u.sync_topic, u.stats_messages, u.stats_emails, u.stripe_customer_id, u.stripe_subscription_id, u.stripe_subscription_status, u.stripe_subscription_interval, u.stripe_subscription_paid_until, u.stripe_subscription_cancel_at, deleted, t.id, t.code, t.name, t.messages_limit, t.messages_expiry_duration, t.emails_limit, t.reservations_limit, t.attachment_file_size_limit, t.attachment_total_size_limit, t.attachment_expiry_duration, t.attachment_bandwidth_limit, t.stripe_monthly_price_id, t.stripe_yearly_price_id
		FROM user u
		LEFT JOIN tier t on t.id = u.tier_id
		WHERE u.stripe_customer_id = ?
	`
	selectTopicPermsQuery = `
		SELECT read, write
		FROM user_access a
		JOIN user u ON u.id = a.user_id
		WHERE (u.user = ? OR u.user = ?) AND ? LIKE a.topic
		ORDER BY u.user DESC
	`

	insertUserQuery = `
		INSERT INTO user (id, user, pass, role, sync_topic, created)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	selectUsernamesQuery = `
		SELECT user
		FROM user
		ORDER BY
			CASE role
				WHEN 'admin' THEN 1
				WHEN 'anonymous' THEN 3
				ELSE 2
			END, user
	`
	selectUserCountQuery         = `SELECT COUNT(*) FROM user`
	updateUserPassQuery          = `UPDATE user SET pass = ? WHERE user = ?`
	updateUserRoleQuery          = `UPDATE user SET role = ? WHERE user = ?`
	updateUserPrefsQuery         = `UPDATE user SET prefs = ? WHERE id = ?`
	updateUserStatsQuery         = `UPDATE user SET stats_messages = ?, stats_emails = ? WHERE id = ?`
	updateUserStatsResetAllQuery = `UPDATE user SET stats_messages = 0, stats_emails = 0`
	updateUserDeletedQuery       = `UPDATE user SET deleted = ? WHERE id = ?`
	deleteUsersMarkedQuery       = `DELETE FROM user WHERE deleted < ?`
	deleteUserQuery              = `DELETE FROM user WHERE user = ?`

	upsertUserAccessQuery = `
		INSERT INTO user_access (user_id, topic, read, write, owner_user_id)
		VALUES ((SELECT id FROM user WHERE user = ?), ?, ?, ?, (SELECT IIF(?='',NULL,(SELECT id FROM user WHERE user=?))))
		ON CONFLICT (user_id, topic)
		DO UPDATE SET read=excluded.read, write=excluded.write, owner_user_id=excluded.owner_user_id
	`
	selectUserAccessQuery = `
		SELECT topic, read, write
		FROM user_access
		WHERE user_id = (SELECT id FROM user WHERE user = ?)
		ORDER BY write DESC, read DESC, topic
	`
	selectUserReservationsQuery = `
		SELECT a_user.topic, a_user.read, a_user.write, a_everyone.read AS everyone_read, a_everyone.write AS everyone_write
		FROM user_access a_user
		LEFT JOIN  user_access a_everyone ON a_user.topic = a_everyone.topic AND a_everyone.user_id = (SELECT id FROM user WHERE user = ?)
		WHERE a_user.user_id = a_user.owner_user_id
		  AND a_user.owner_user_id = (SELECT id FROM user WHERE user = ?)
		ORDER BY a_user.topic
	`
	selectUserReservationsCountQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE user_id = owner_user_id 
		  AND owner_user_id = (SELECT id FROM user WHERE user = ?)
	`
	selectUserReservationsOwnerQuery = `
		SELECT owner_user_id
		FROM user_access
		WHERE topic = ?
		  AND user_id = owner_user_id
	`
	selectUserHasReservationQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE user_id = owner_user_id
		  AND owner_user_id = (SELECT id FROM user WHERE user = ?)
		  AND topic = ?
	`
	selectOtherAccessCountQuery = `
		SELECT COUNT(*)
		FROM user_access
		WHERE (topic = ? OR ? LIKE topic)
		  AND (owner_user_id IS NULL OR owner_user_id != (SELECT id FROM user WHERE user = ?))
	`
	deleteAllAccessQuery  = `DELETE FROM user_access`
	deleteUserAccessQuery = `
		DELETE FROM user_access
		WHERE user_id = (SELECT id FROM user WHERE user = ?)
		   OR owner_user_id = (SELECT id FROM user WHERE user = ?)
	`
	deleteTopicAccessQuery = `
		DELETE FROM user_access
	   	WHERE (user_id = (SELECT id FROM user WHERE user = ?) OR owner_user_id = (SELECT id FROM user WHERE user = ?))
	   	  AND topic = ?
  	`

	selectTokenCountQuery      = `SELECT COUNT(*) FROM user_token WHERE user_id = ?`
	selectTokensQuery          = `SELECT token, label, last_access, last_origin, expires FROM user_token WHERE user_id = ?`
	selectTokenQuery           = `SELECT token, label, last_access, last_origin, expires FROM user_token WHERE user_id = ? AND token = ?`
	insertTokenQuery           = `INSERT INTO user_token (user_id, token, label, last_access, last_origin, expires) VALUES (?, ?, ?, ?, ?, ?)`
	updateTokenExpiryQuery     = `UPDATE user_token SET expires = ? WHERE user_id = ? AND token = ?`
	updateTokenLabelQuery      = `UPDATE user_token SET label = ? WHERE user_id = ? AND token = ?`
	updateTokenLastAccessQuery = `UPDATE user_token SET last_access = ?, last_origin = ? WHERE token = ?`
	deleteTokenQuery           = `DELETE FROM user_token WHERE user_id = ? AND token = ?`
	deleteAllTokenQuery        = `DELETE FROM user_token WHERE user_id = ?`
	deleteExpiredTokensQuery   = `DELETE FROM user_token WHERE expires > 0 AND expires < ?`
	deleteExcessTokensQuery    = `
		DELETE FROM user_token
		WHERE (user_id, token) NOT IN (
			SELECT user_id, token
			FROM user_token
			WHERE user_id = ?
			ORDER BY expires DESC
			LIMIT ?
		)
	`

	insertTierQuery = `
		INSERT INTO tier (id, code, name, messages_limit, messages_expiry_duration, emails_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	updateTierQuery = `
		UPDATE tier
		SET name = ?, messages_limit = ?, messages_expiry_duration = ?, emails_limit = ?, reservations_limit = ?, attachment_file_size_limit = ?, attachment_total_size_limit = ?, attachment_expiry_duration = ?, attachment_bandwidth_limit = ?, stripe_monthly_price_id = ?, stripe_yearly_price_id = ?
		WHERE code = ?
	`
	selectTiersQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
	`
	selectTierByCodeQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
		WHERE code = ?
	`
	selectTierByPriceIDQuery = `
		SELECT id, code, name, messages_limit, messages_expiry_duration, emails_limit, reservations_limit, attachment_file_size_limit, attachment_total_size_limit, attachment_expiry_duration, attachment_bandwidth_limit, stripe_monthly_price_id, stripe_yearly_price_id
		FROM tier
		WHERE (stripe_monthly_price_id = ? OR stripe_yearly_price_id = ?)
	`
	updateUserTierQuery = `UPDATE user SET tier_id = (SELECT id FROM tier WHERE code = ?) WHERE user = ?`
	deleteUserTierQuery = `UPDATE user SET tier_id = null WHERE user = ?`
	deleteTierQuery     = `DELETE FROM tier WHERE code = ?`

	updateBillingQuery = `
		UPDATE user
		SET stripe_customer_id = ?, stripe_subscription_id = ?, stripe_subscription_status = ?, stripe_subscription_interval = ?, stripe_subscription_paid_until = ?, stripe_subscription_cancel_at = ?
		WHERE user = ?
	`
)

// Schema management queries
const (
	currentSchemaVersion     = 3
	insertSchemaVersion      = `INSERT INTO schemaVersion VALUES (1, ?)`
	updateSchemaVersion      = `UPDATE schemaVersion SET version = ? WHERE id = 1`
	selectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`

	// 1 -> 2 (complex migration!)
	migrate1To2CreateTablesQueries = `
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
	migrate1To2SelectAllOldUsernamesNoTx = `SELECT user FROM user_old`
	migrate1To2InsertUserNoTx            = `
		INSERT INTO user (id, user, pass, role, sync_topic, created)
		SELECT ?, user, pass, role, ?, UNIXEPOCH() FROM user_old WHERE user = ?
	`
	migrate1To2InsertFromOldTablesAndDropNoTx = `
		INSERT INTO user_access (user_id, topic, read, write)
		SELECT u.id, a.topic, a.read, a.write
		FROM user u
	 	JOIN access a ON u.user = a.user;

		DROP TABLE access;
		DROP TABLE user_old;
	`

	// 2 -> 3
	migrate2To3UpdateQueries = `
		ALTER TABLE user ADD COLUMN stripe_subscription_interval TEXT;
		ALTER TABLE tier RENAME COLUMN stripe_price_id TO stripe_monthly_price_id;
		ALTER TABLE tier ADD COLUMN stripe_yearly_price_id TEXT;
		DROP INDEX IF EXISTS idx_tier_price_id;
		CREATE UNIQUE INDEX idx_tier_stripe_monthly_price_id ON tier (stripe_monthly_price_id);
		CREATE UNIQUE INDEX idx_tier_stripe_yearly_price_id ON tier (stripe_yearly_price_id);
	`
)

var (
	migrations = map[int]func(db *sql.DB) error{
		1: migrateFrom1,
		2: migrateFrom2,
	}
)

// Manager is an implementation of Manager. It stores users and access control list
// in a SQLite database.
type Manager struct {
	db            *sql.DB
	defaultAccess Permission              // Default permission if no ACL matches
	statsQueue    map[string]*Stats       // "Queue" to asynchronously write user stats to the database (UserID -> Stats)
	tokenQueue    map[string]*TokenUpdate // "Queue" to asynchronously write token access stats to the database (Token ID -> TokenUpdate)
	bcryptCost    int                     // Makes testing easier
	mu            sync.Mutex
}

var _ Auther = (*Manager)(nil)

// NewManager creates a new Manager instance
func NewManager(filename, startupQueries string, defaultAccess Permission, bcryptCost int, queueWriterInterval time.Duration) (*Manager, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupDB(db); err != nil {
		return nil, err
	}
	if err := runStartupQueries(db, startupQueries); err != nil {
		return nil, err
	}
	manager := &Manager{
		db:            db,
		defaultAccess: defaultAccess,
		statsQueue:    make(map[string]*Stats),
		tokenQueue:    make(map[string]*TokenUpdate),
		bcryptCost:    bcryptCost,
	}
	go manager.asyncQueueWriter(queueWriterInterval)
	return manager, nil
}

// Authenticate checks username and password and returns a User if correct, and the user has not been
// marked as deleted. The method returns in constant-ish time, regardless of whether the user exists or
// the password is correct or incorrect.
func (a *Manager) Authenticate(username, password string) (*User, error) {
	if username == Everyone {
		return nil, ErrUnauthenticated
	}
	user, err := a.User(username)
	if err != nil {
		log.Tag(tag).Field("user_name", username).Err(err).Trace("Authentication of user failed (1)")
		bcrypt.CompareHashAndPassword([]byte(userAuthIntentionalSlowDownHash), []byte("intentional slow-down to avoid timing attacks"))
		return nil, ErrUnauthenticated
	} else if user.Deleted {
		log.Tag(tag).Field("user_name", username).Trace("Authentication of user failed (2): user marked deleted")
		bcrypt.CompareHashAndPassword([]byte(userAuthIntentionalSlowDownHash), []byte("intentional slow-down to avoid timing attacks"))
		return nil, ErrUnauthenticated
	} else if err := bcrypt.CompareHashAndPassword([]byte(user.Hash), []byte(password)); err != nil {
		log.Tag(tag).Field("user_name", username).Err(err).Trace("Authentication of user failed (3)")
		return nil, ErrUnauthenticated
	}
	return user, nil
}

// AuthenticateToken checks if the token exists and returns the associated User if it does.
// The method sets the User.Token value to the token that was used for authentication.
func (a *Manager) AuthenticateToken(token string) (*User, error) {
	if len(token) != tokenLength {
		return nil, ErrUnauthenticated
	}
	user, err := a.userByToken(token)
	if err != nil {
		log.Tag(tag).Field("token", token).Err(err).Trace("Authentication of token failed")
		return nil, ErrUnauthenticated
	}
	user.Token = token
	return user, nil
}

// CreateToken generates a random token for the given user and returns it. The token expires
// after a fixed duration unless ChangeToken is called. This function also prunes tokens for the
// given user, if there are too many of them.
func (a *Manager) CreateToken(userID, label string, expires time.Time, origin netip.Addr) (*Token, error) {
	token := util.RandomStringPrefix(tokenPrefix, tokenLength)
	tx, err := a.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access := time.Now()
	if _, err := tx.Exec(insertTokenQuery, userID, token, label, access.Unix(), origin.String(), expires.Unix()); err != nil {
		return nil, err
	}
	rows, err := tx.Query(selectTokenCountQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errNoRows
	}
	var tokenCount int
	if err := rows.Scan(&tokenCount); err != nil {
		return nil, err
	}
	if tokenCount >= tokenMaxCount {
		// This pruning logic is done in two queries for efficiency. The SELECT above is a lookup
		// on two indices, whereas the query below is a full table scan.
		if _, err := tx.Exec(deleteExcessTokensQuery, userID, tokenMaxCount); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Token{
		Value:      token,
		Label:      label,
		LastAccess: access,
		LastOrigin: origin,
		Expires:    expires,
	}, nil
}

// Tokens returns all existing tokens for the user with the given user ID
func (a *Manager) Tokens(userID string) ([]*Token, error) {
	rows, err := a.db.Query(selectTokensQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := make([]*Token, 0)
	for {
		token, err := a.readToken(rows)
		if err == ErrTokenNotFound {
			break
		} else if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// Token returns a specific token for a user
func (a *Manager) Token(userID, token string) (*Token, error) {
	rows, err := a.db.Query(selectTokenQuery, userID, token)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return a.readToken(rows)
}

func (a *Manager) readToken(rows *sql.Rows) (*Token, error) {
	var token, label, lastOrigin string
	var lastAccess, expires int64
	if !rows.Next() {
		return nil, ErrTokenNotFound
	}
	if err := rows.Scan(&token, &label, &lastAccess, &lastOrigin, &expires); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	lastOriginIP, err := netip.ParseAddr(lastOrigin)
	if err != nil {
		lastOriginIP = netip.IPv4Unspecified()
	}
	return &Token{
		Value:      token,
		Label:      label,
		LastAccess: time.Unix(lastAccess, 0),
		LastOrigin: lastOriginIP,
		Expires:    time.Unix(expires, 0),
	}, nil
}

// ChangeToken updates a token's label and/or expiry date
func (a *Manager) ChangeToken(userID, token string, label *string, expires *time.Time) (*Token, error) {
	if token == "" {
		return nil, errNoTokenProvided
	}
	tx, err := a.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if label != nil {
		if _, err := tx.Exec(updateTokenLabelQuery, *label, userID, token); err != nil {
			return nil, err
		}
	}
	if expires != nil {
		if _, err := tx.Exec(updateTokenExpiryQuery, expires.Unix(), userID, token); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a.Token(userID, token)
}

// RemoveToken deletes the token defined in User.Token
func (a *Manager) RemoveToken(userID, token string) error {
	if token == "" {
		return errNoTokenProvided
	}
	if _, err := a.db.Exec(deleteTokenQuery, userID, token); err != nil {
		return err
	}
	return nil
}

// RemoveExpiredTokens deletes all expired tokens from the database
func (a *Manager) RemoveExpiredTokens() error {
	if _, err := a.db.Exec(deleteExpiredTokensQuery, time.Now().Unix()); err != nil {
		return err
	}
	return nil
}

// RemoveDeletedUsers deletes all users that have been marked deleted for
func (a *Manager) RemoveDeletedUsers() error {
	if _, err := a.db.Exec(deleteUsersMarkedQuery, time.Now().Unix()); err != nil {
		return err
	}
	return nil
}

// ChangeSettings persists the user settings
func (a *Manager) ChangeSettings(userID string, prefs *Prefs) error {
	b, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	if _, err := a.db.Exec(updateUserPrefsQuery, string(b), userID); err != nil {
		return err
	}
	return nil
}

// ResetStats resets all user stats in the user database. This touches all users.
func (a *Manager) ResetStats() error {
	a.mu.Lock() // Includes database query to avoid races!
	defer a.mu.Unlock()
	if _, err := a.db.Exec(updateUserStatsResetAllQuery); err != nil {
		return err
	}
	a.statsQueue = make(map[string]*Stats)
	return nil
}

// EnqueueUserStats adds the user to a queue which writes out user stats (messages, emails, ..) in
// batches at a regular interval
func (a *Manager) EnqueueUserStats(userID string, stats *Stats) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statsQueue[userID] = stats
}

// EnqueueTokenUpdate adds the token update to  a queue which writes out token access times
// in batches at a regular interval
func (a *Manager) EnqueueTokenUpdate(tokenID string, update *TokenUpdate) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokenQueue[tokenID] = update
}

func (a *Manager) asyncQueueWriter(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := a.writeUserStatsQueue(); err != nil {
			log.Tag(tag).Err(err).Warn("Writing user stats queue failed")
		}
		if err := a.writeTokenUpdateQueue(); err != nil {
			log.Tag(tag).Err(err).Warn("Writing token update queue failed")
		}
	}
}

func (a *Manager) writeUserStatsQueue() error {
	a.mu.Lock()
	if len(a.statsQueue) == 0 {
		a.mu.Unlock()
		log.Tag(tag).Trace("No user stats updates to commit")
		return nil
	}
	statsQueue := a.statsQueue
	a.statsQueue = make(map[string]*Stats)
	a.mu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	log.Tag(tag).Debug("Writing user stats queue for %d user(s)", len(statsQueue))
	for userID, update := range statsQueue {
		log.
			Tag(tag).
			Fields(log.Context{
				"user_id":        userID,
				"messages_count": update.Messages,
				"emails_count":   update.Emails,
			}).
			Trace("Updating stats for user %s", userID)
		if _, err := tx.Exec(updateUserStatsQuery, update.Messages, update.Emails, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *Manager) writeTokenUpdateQueue() error {
	a.mu.Lock()
	if len(a.tokenQueue) == 0 {
		a.mu.Unlock()
		log.Tag(tag).Trace("No token updates to commit")
		return nil
	}
	tokenQueue := a.tokenQueue
	a.tokenQueue = make(map[string]*TokenUpdate)
	a.mu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	log.Tag(tag).Debug("Writing token update queue for %d token(s)", len(tokenQueue))
	for tokenID, update := range tokenQueue {
		log.Tag(tag).Trace("Updating token %s with last access time %v", tokenID, update.LastAccess.Unix())
		if _, err := tx.Exec(updateTokenLastAccessQuery, update.LastAccess.Unix(), update.LastOrigin.String(), tokenID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Authorize returns nil if the given user has access to the given topic using the desired
// permission. The user param may be nil to signal an anonymous user.
func (a *Manager) Authorize(user *User, topic string, perm Permission) error {
	if user != nil && user.Role == RoleAdmin {
		return nil // Admin can do everything
	}
	username := Everyone
	if user != nil {
		username = user.Name
	}
	// Select the read/write permissions for this user/topic combo. The query may return two
	// rows (one for everyone, and one for the user), but prioritizes the user.
	rows, err := a.db.Query(selectTopicPermsQuery, Everyone, username, topic)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return a.resolvePerms(a.defaultAccess, perm)
	}
	var read, write bool
	if err := rows.Scan(&read, &write); err != nil {
		return err
	} else if err := rows.Err(); err != nil {
		return err
	}
	return a.resolvePerms(NewPermission(read, write), perm)
}

func (a *Manager) resolvePerms(base, perm Permission) error {
	if perm == PermissionRead && base.IsRead() {
		return nil
	} else if perm == PermissionWrite && base.IsWrite() {
		return nil
	}
	return ErrUnauthorized
}

// AddUser adds a user with the given username, password and role
func (a *Manager) AddUser(username, password string, role Role) error {
	if !AllowedUsername(username) || !AllowedRole(role) {
		return ErrInvalidArgument
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), a.bcryptCost)
	if err != nil {
		return err
	}
	userID := util.RandomStringPrefix(userIDPrefix, userIDLength)
	syncTopic, now := util.RandomStringPrefix(syncTopicPrefix, syncTopicLength), time.Now().Unix()
	if _, err = a.db.Exec(insertUserQuery, userID, username, hash, role, syncTopic, now); err != nil {
		return err
	}
	return nil
}

// RemoveUser deletes the user with the given username. The function returns nil on success, even
// if the user did not exist in the first place.
func (a *Manager) RemoveUser(username string) error {
	if !AllowedUsername(username) {
		return ErrInvalidArgument
	}
	// Rows in user_access, user_token, etc. are deleted via foreign keys
	if _, err := a.db.Exec(deleteUserQuery, username); err != nil {
		return err
	}
	return nil
}

// MarkUserRemoved sets the deleted flag on the user, and deletes all access tokens. This prevents
// successful auth via Authenticate. A background process will delete the user at a later date.
func (a *Manager) MarkUserRemoved(user *User) error {
	if !AllowedUsername(user.Name) {
		return ErrInvalidArgument
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(deleteUserAccessQuery, user.Name, user.Name); err != nil {
		return err
	}
	if _, err := tx.Exec(deleteAllTokenQuery, user.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(updateUserDeletedQuery, time.Now().Add(userHardDeleteAfterDuration).Unix(), user.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Users returns a list of users. It always also returns the Everyone user ("*").
func (a *Manager) Users() ([]*User, error) {
	rows, err := a.db.Query(selectUsernamesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	usernames := make([]string, 0)
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
		usernames = append(usernames, username)
	}
	rows.Close()
	users := make([]*User, 0)
	for _, username := range usernames {
		user, err := a.User(username)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

// UsersCount returns the number of users in the databsae
func (a *Manager) UsersCount() (int64, error) {
	rows, err := a.db.Query(selectUserCountQuery)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errNoRows
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// User returns the user with the given username if it exists, or ErrUserNotFound otherwise.
// You may also pass Everyone to retrieve the anonymous user and its Grant list.
func (a *Manager) User(username string) (*User, error) {
	rows, err := a.db.Query(selectUserByNameQuery, username)
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

// UserByID returns the user with the given ID if it exists, or ErrUserNotFound otherwise
func (a *Manager) UserByID(id string) (*User, error) {
	rows, err := a.db.Query(selectUserByIDQuery, id)
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

// UserByStripeCustomer returns the user with the given Stripe customer ID if it exists, or ErrUserNotFound otherwise.
func (a *Manager) UserByStripeCustomer(stripeCustomerID string) (*User, error) {
	rows, err := a.db.Query(selectUserByStripeCustomerIDQuery, stripeCustomerID)
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

func (a *Manager) userByToken(token string) (*User, error) {
	rows, err := a.db.Query(selectUserByTokenQuery, token, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return a.readUser(rows)
}

func (a *Manager) readUser(rows *sql.Rows) (*User, error) {
	defer rows.Close()
	var id, username, hash, role, prefs, syncTopic string
	var stripeCustomerID, stripeSubscriptionID, stripeSubscriptionStatus, stripeSubscriptionInterval, stripeMonthlyPriceID, stripeYearlyPriceID, tierID, tierCode, tierName sql.NullString
	var messages, emails int64
	var messagesLimit, messagesExpiryDuration, emailsLimit, reservationsLimit, attachmentFileSizeLimit, attachmentTotalSizeLimit, attachmentExpiryDuration, attachmentBandwidthLimit, stripeSubscriptionPaidUntil, stripeSubscriptionCancelAt, deleted sql.NullInt64
	if !rows.Next() {
		return nil, ErrUserNotFound
	}
	if err := rows.Scan(&id, &username, &hash, &role, &prefs, &syncTopic, &messages, &emails, &stripeCustomerID, &stripeSubscriptionID, &stripeSubscriptionStatus, &stripeSubscriptionInterval, &stripeSubscriptionPaidUntil, &stripeSubscriptionCancelAt, &deleted, &tierID, &tierCode, &tierName, &messagesLimit, &messagesExpiryDuration, &emailsLimit, &reservationsLimit, &attachmentFileSizeLimit, &attachmentTotalSizeLimit, &attachmentExpiryDuration, &attachmentBandwidthLimit, &stripeMonthlyPriceID, &stripeYearlyPriceID); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	user := &User{
		ID:        id,
		Name:      username,
		Hash:      hash,
		Role:      Role(role),
		Prefs:     &Prefs{},
		SyncTopic: syncTopic,
		Stats: &Stats{
			Messages: messages,
			Emails:   emails,
		},
		Billing: &Billing{
			StripeCustomerID:            stripeCustomerID.String,                                          // May be empty
			StripeSubscriptionID:        stripeSubscriptionID.String,                                      // May be empty
			StripeSubscriptionStatus:    stripe.SubscriptionStatus(stripeSubscriptionStatus.String),       // May be empty
			StripeSubscriptionInterval:  stripe.PriceRecurringInterval(stripeSubscriptionInterval.String), // May be empty
			StripeSubscriptionPaidUntil: time.Unix(stripeSubscriptionPaidUntil.Int64, 0),                  // May be zero
			StripeSubscriptionCancelAt:  time.Unix(stripeSubscriptionCancelAt.Int64, 0),                   // May be zero
		},
		Deleted: deleted.Valid,
	}
	if err := json.Unmarshal([]byte(prefs), user.Prefs); err != nil {
		return nil, err
	}
	if tierCode.Valid {
		// See readTier() when this is changed!
		user.Tier = &Tier{
			ID:                       tierID.String,
			Code:                     tierCode.String,
			Name:                     tierName.String,
			MessageLimit:             messagesLimit.Int64,
			MessageExpiryDuration:    time.Duration(messagesExpiryDuration.Int64) * time.Second,
			EmailLimit:               emailsLimit.Int64,
			ReservationLimit:         reservationsLimit.Int64,
			AttachmentFileSizeLimit:  attachmentFileSizeLimit.Int64,
			AttachmentTotalSizeLimit: attachmentTotalSizeLimit.Int64,
			AttachmentExpiryDuration: time.Duration(attachmentExpiryDuration.Int64) * time.Second,
			AttachmentBandwidthLimit: attachmentBandwidthLimit.Int64,
			StripeMonthlyPriceID:     stripeMonthlyPriceID.String, // May be empty
			StripeYearlyPriceID:      stripeYearlyPriceID.String,  // May be empty
		}
	}
	return user, nil
}

// Grants returns all user-specific access control entries
func (a *Manager) Grants(username string) ([]Grant, error) {
	rows, err := a.db.Query(selectUserAccessQuery, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make([]Grant, 0)
	for rows.Next() {
		var topic string
		var read, write bool
		if err := rows.Scan(&topic, &read, &write); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
		grants = append(grants, Grant{
			TopicPattern: fromSQLWildcard(topic),
			Allow:        NewPermission(read, write),
		})
	}
	return grants, nil
}

// Reservations returns all user-owned topics, and the associated everyone-access
func (a *Manager) Reservations(username string) ([]Reservation, error) {
	rows, err := a.db.Query(selectUserReservationsQuery, Everyone, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reservations := make([]Reservation, 0)
	for rows.Next() {
		var topic string
		var ownerRead, ownerWrite bool
		var everyoneRead, everyoneWrite sql.NullBool
		if err := rows.Scan(&topic, &ownerRead, &ownerWrite, &everyoneRead, &everyoneWrite); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
		reservations = append(reservations, Reservation{
			Topic:    topic,
			Owner:    NewPermission(ownerRead, ownerWrite),
			Everyone: NewPermission(everyoneRead.Bool, everyoneWrite.Bool), // false if null
		})
	}
	return reservations, nil
}

// HasReservation returns true if the given topic access is owned by the user
func (a *Manager) HasReservation(username, topic string) (bool, error) {
	rows, err := a.db.Query(selectUserHasReservationQuery, username, topic)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, errNoRows
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// ReservationsCount returns the number of reservations owned by this user
func (a *Manager) ReservationsCount(username string) (int64, error) {
	rows, err := a.db.Query(selectUserReservationsCountQuery, username)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errNoRows
	}
	var count int64
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ReservationOwner returns user ID of the user that owns this topic, or an
// empty string if it's not owned by anyone
func (a *Manager) ReservationOwner(topic string) (string, error) {
	rows, err := a.db.Query(selectUserReservationsOwnerQuery, topic)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", nil
	}
	var ownerUserID string
	if err := rows.Scan(&ownerUserID); err != nil {
		return "", err
	}
	return ownerUserID, nil
}

// ChangePassword changes a user's password
func (a *Manager) ChangePassword(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), a.bcryptCost)
	if err != nil {
		return err
	}
	if _, err := a.db.Exec(updateUserPassQuery, hash, username); err != nil {
		return err
	}
	return nil
}

// ChangeRole changes a user's role. When a role is changed from RoleUser to RoleAdmin,
// all existing access control entries (Grant) are removed, since they are no longer needed.
func (a *Manager) ChangeRole(username string, role Role) error {
	if !AllowedUsername(username) || !AllowedRole(role) {
		return ErrInvalidArgument
	}
	if _, err := a.db.Exec(updateUserRoleQuery, string(role), username); err != nil {
		return err
	}
	if role == RoleAdmin {
		if _, err := a.db.Exec(deleteUserAccessQuery, username, username); err != nil {
			return err
		}
	}
	return nil
}

// ChangeTier changes a user's tier using the tier code. This function does not delete reservations, messages,
// or attachments, even if the new tier has lower limits in this regard. That has to be done elsewhere.
func (a *Manager) ChangeTier(username, tier string) error {
	if !AllowedUsername(username) {
		return ErrInvalidArgument
	}
	t, err := a.Tier(tier)
	if err != nil {
		return err
	} else if err := a.checkReservationsLimit(username, t.ReservationLimit); err != nil {
		return err
	}
	if _, err := a.db.Exec(updateUserTierQuery, tier, username); err != nil {
		return err
	}
	return nil
}

// ResetTier removes the tier from the given user
func (a *Manager) ResetTier(username string) error {
	if !AllowedUsername(username) && username != Everyone && username != "" {
		return ErrInvalidArgument
	} else if err := a.checkReservationsLimit(username, 0); err != nil {
		return err
	}
	_, err := a.db.Exec(deleteUserTierQuery, username)
	return err
}

func (a *Manager) checkReservationsLimit(username string, reservationsLimit int64) error {
	u, err := a.User(username)
	if err != nil {
		return err
	}
	if u.Tier != nil && reservationsLimit < u.Tier.ReservationLimit {
		reservations, err := a.Reservations(username)
		if err != nil {
			return err
		} else if int64(len(reservations)) > reservationsLimit {
			return ErrTooManyReservations
		}
	}
	return nil
}

// AllowReservation tests if a user may create an access control entry for the given topic.
// If there are any ACL entries that are not owned by the user, an error is returned.
func (a *Manager) AllowReservation(username string, topic string) error {
	if (!AllowedUsername(username) && username != Everyone) || !AllowedTopic(topic) {
		return ErrInvalidArgument
	}
	rows, err := a.db.Query(selectOtherAccessCountQuery, topic, topic, username)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return errNoRows
	}
	var otherCount int
	if err := rows.Scan(&otherCount); err != nil {
		return err
	}
	if otherCount > 0 {
		return errTopicOwnedByOthers
	}
	return nil
}

// AllowAccess adds or updates an entry in th access control list for a specific user. It controls
// read/write access to a topic. The parameter topicPattern may include wildcards (*). The ACL entry
// owner may either be a user (username), or the system (empty).
func (a *Manager) AllowAccess(username string, topicPattern string, permission Permission) error {
	if !AllowedUsername(username) && username != Everyone {
		return ErrInvalidArgument
	} else if !AllowedTopicPattern(topicPattern) {
		return ErrInvalidArgument
	}
	owner := ""
	if _, err := a.db.Exec(upsertUserAccessQuery, username, toSQLWildcard(topicPattern), permission.IsRead(), permission.IsWrite(), owner, owner); err != nil {
		return err
	}
	return nil
}

// ResetAccess removes an access control list entry for a specific username/topic, or (if topic is
// empty) for an entire user. The parameter topicPattern may include wildcards (*).
func (a *Manager) ResetAccess(username string, topicPattern string) error {
	if !AllowedUsername(username) && username != Everyone && username != "" {
		return ErrInvalidArgument
	} else if !AllowedTopicPattern(topicPattern) && topicPattern != "" {
		return ErrInvalidArgument
	}
	if username == "" && topicPattern == "" {
		_, err := a.db.Exec(deleteAllAccessQuery, username)
		return err
	} else if topicPattern == "" {
		_, err := a.db.Exec(deleteUserAccessQuery, username, username)
		return err
	}
	_, err := a.db.Exec(deleteTopicAccessQuery, username, username, toSQLWildcard(topicPattern))
	return err
}

// AddReservation creates two access control entries for the given topic: one with full read/write access for the
// given user, and one for Everyone with the permission passed as everyone. The user also owns the entries, and
// can modify or delete them.
func (a *Manager) AddReservation(username string, topic string, everyone Permission) error {
	if !AllowedUsername(username) || username == Everyone || !AllowedTopic(topic) {
		return ErrInvalidArgument
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(upsertUserAccessQuery, username, topic, true, true, username, username); err != nil {
		return err
	}
	if _, err := tx.Exec(upsertUserAccessQuery, Everyone, topic, everyone.IsRead(), everyone.IsWrite(), username, username); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveReservations deletes the access control entries associated with the given username/topic, as
// well as all entries with Everyone/topic. This is the counterpart for AddReservation.
func (a *Manager) RemoveReservations(username string, topics ...string) error {
	if !AllowedUsername(username) || username == Everyone || len(topics) == 0 {
		return ErrInvalidArgument
	}
	for _, topic := range topics {
		if !AllowedTopic(topic) {
			return ErrInvalidArgument
		}
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, topic := range topics {
		if _, err := tx.Exec(deleteTopicAccessQuery, username, username, topic); err != nil {
			return err
		}
		if _, err := tx.Exec(deleteTopicAccessQuery, Everyone, Everyone, topic); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DefaultAccess returns the default read/write access if no access control entry matches
func (a *Manager) DefaultAccess() Permission {
	return a.defaultAccess
}

// AddTier creates a new tier in the database
func (a *Manager) AddTier(tier *Tier) error {
	if tier.ID == "" {
		tier.ID = util.RandomStringPrefix(tierIDPrefix, tierIDLength)
	}
	if _, err := a.db.Exec(insertTierQuery, tier.ID, tier.Code, tier.Name, tier.MessageLimit, int64(tier.MessageExpiryDuration.Seconds()), tier.EmailLimit, tier.ReservationLimit, tier.AttachmentFileSizeLimit, tier.AttachmentTotalSizeLimit, int64(tier.AttachmentExpiryDuration.Seconds()), tier.AttachmentBandwidthLimit, nullString(tier.StripeMonthlyPriceID), nullString(tier.StripeYearlyPriceID)); err != nil {
		return err
	}
	return nil
}

// UpdateTier updates a tier's properties in the database
func (a *Manager) UpdateTier(tier *Tier) error {
	if _, err := a.db.Exec(updateTierQuery, tier.Name, tier.MessageLimit, int64(tier.MessageExpiryDuration.Seconds()), tier.EmailLimit, tier.ReservationLimit, tier.AttachmentFileSizeLimit, tier.AttachmentTotalSizeLimit, int64(tier.AttachmentExpiryDuration.Seconds()), tier.AttachmentBandwidthLimit, nullString(tier.StripeMonthlyPriceID), nullString(tier.StripeYearlyPriceID), tier.Code); err != nil {
		return err
	}
	return nil
}

// RemoveTier deletes the tier with the given code
func (a *Manager) RemoveTier(code string) error {
	if !AllowedTier(code) {
		return ErrInvalidArgument
	}
	// This fails if any user has this tier
	if _, err := a.db.Exec(deleteTierQuery, code); err != nil {
		return err
	}
	return nil
}

// ChangeBilling updates a user's billing fields, namely the Stripe customer ID, and subscription information
func (a *Manager) ChangeBilling(username string, billing *Billing) error {
	if _, err := a.db.Exec(updateBillingQuery, nullString(billing.StripeCustomerID), nullString(billing.StripeSubscriptionID), nullString(string(billing.StripeSubscriptionStatus)), nullString(string(billing.StripeSubscriptionInterval)), nullInt64(billing.StripeSubscriptionPaidUntil.Unix()), nullInt64(billing.StripeSubscriptionCancelAt.Unix()), username); err != nil {
		return err
	}
	return nil
}

// Tiers returns a list of all Tier structs
func (a *Manager) Tiers() ([]*Tier, error) {
	rows, err := a.db.Query(selectTiersQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tiers := make([]*Tier, 0)
	for {
		tier, err := a.readTier(rows)
		if err == ErrTierNotFound {
			break
		} else if err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}
	return tiers, nil
}

// Tier returns a Tier based on the code, or ErrTierNotFound if it does not exist
func (a *Manager) Tier(code string) (*Tier, error) {
	rows, err := a.db.Query(selectTierByCodeQuery, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return a.readTier(rows)
}

// TierByStripePrice returns a Tier based on the Stripe price ID, or ErrTierNotFound if it does not exist
func (a *Manager) TierByStripePrice(priceID string) (*Tier, error) {
	rows, err := a.db.Query(selectTierByPriceIDQuery, priceID, priceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return a.readTier(rows)
}

func (a *Manager) readTier(rows *sql.Rows) (*Tier, error) {
	var id, code, name string
	var stripeMonthlyPriceID, stripeYearlyPriceID sql.NullString
	var messagesLimit, messagesExpiryDuration, emailsLimit, reservationsLimit, attachmentFileSizeLimit, attachmentTotalSizeLimit, attachmentExpiryDuration, attachmentBandwidthLimit sql.NullInt64
	if !rows.Next() {
		return nil, ErrTierNotFound
	}
	if err := rows.Scan(&id, &code, &name, &messagesLimit, &messagesExpiryDuration, &emailsLimit, &reservationsLimit, &attachmentFileSizeLimit, &attachmentTotalSizeLimit, &attachmentExpiryDuration, &attachmentBandwidthLimit, &stripeMonthlyPriceID, &stripeYearlyPriceID); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	// When changed, note readUser() as well
	return &Tier{
		ID:                       id,
		Code:                     code,
		Name:                     name,
		MessageLimit:             messagesLimit.Int64,
		MessageExpiryDuration:    time.Duration(messagesExpiryDuration.Int64) * time.Second,
		EmailLimit:               emailsLimit.Int64,
		ReservationLimit:         reservationsLimit.Int64,
		AttachmentFileSizeLimit:  attachmentFileSizeLimit.Int64,
		AttachmentTotalSizeLimit: attachmentTotalSizeLimit.Int64,
		AttachmentExpiryDuration: time.Duration(attachmentExpiryDuration.Int64) * time.Second,
		AttachmentBandwidthLimit: attachmentBandwidthLimit.Int64,
		StripeMonthlyPriceID:     stripeMonthlyPriceID.String, // May be empty
		StripeYearlyPriceID:      stripeYearlyPriceID.String,  // May be empty
	}, nil
}

// Close closes the underlying database
func (a *Manager) Close() error {
	return a.db.Close()
}

func toSQLWildcard(s string) string {
	return strings.ReplaceAll(s, "*", "%")
}

func fromSQLWildcard(s string) string {
	return strings.ReplaceAll(s, "%", "*")
}

func runStartupQueries(db *sql.DB, startupQueries string) error {
	if _, err := db.Exec(startupQueries); err != nil {
		return err
	}
	if _, err := db.Exec(builtinStartupQueries); err != nil {
		return err
	}
	return nil
}

func setupDB(db *sql.DB) error {
	// If 'schemaVersion' table does not exist, this must be a new database
	rowsSV, err := db.Query(selectSchemaVersionQuery)
	if err != nil {
		return setupNewDB(db)
	}
	defer rowsSV.Close()

	// If 'schemaVersion' table exists, read version and potentially upgrade
	schemaVersion := 0
	if !rowsSV.Next() {
		return errors.New("cannot determine schema version: database file may be corrupt")
	}
	if err := rowsSV.Scan(&schemaVersion); err != nil {
		return err
	}
	rowsSV.Close()

	// Do migrations
	if schemaVersion == currentSchemaVersion {
		return nil
	} else if schemaVersion > currentSchemaVersion {
		return fmt.Errorf("unexpected schema version: version %d is higher than current version %d", schemaVersion, currentSchemaVersion)
	}
	for i := schemaVersion; i < currentSchemaVersion; i++ {
		fn, ok := migrations[i]
		if !ok {
			return fmt.Errorf("cannot find migration step from schema version %d to %d", i, i+1)
		} else if err := fn(db); err != nil {
			return err
		}
	}
	return nil
}

func setupNewDB(db *sql.DB) error {
	if _, err := db.Exec(createTablesQueries); err != nil {
		return err
	}
	if _, err := db.Exec(insertSchemaVersion, currentSchemaVersion); err != nil {
		return err
	}
	return nil
}

func migrateFrom1(db *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 1 to 2")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Rename user -> user_old, and create new tables
	if _, err := tx.Exec(migrate1To2CreateTablesQueries); err != nil {
		return err
	}
	// Insert users from user_old into new user table, with ID and sync_topic
	rows, err := tx.Query(migrate1To2SelectAllOldUsernamesNoTx)
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
		if _, err := tx.Exec(migrate1To2InsertUserNoTx, userID, syncTopic, username); err != nil {
			return err
		}
	}
	// Migrate old "access" table to "user_access" and drop "access" and "user_old"
	if _, err := tx.Exec(migrate1To2InsertFromOldTablesAndDropNoTx); err != nil {
		return err
	}
	if _, err := tx.Exec(updateSchemaVersion, 2); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func migrateFrom2(db *sql.DB) error {
	log.Tag(tag).Info("Migrating user database schema: from 2 to 3")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(migrate2To3UpdateQueries); err != nil {
		return err
	}
	if _, err := tx.Exec(updateSchemaVersion, 3); err != nil {
		return err
	}
	return tx.Commit()
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

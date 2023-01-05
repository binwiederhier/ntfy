package user

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"golang.org/x/crypto/bcrypt"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/util"
	"strings"
	"sync"
	"time"
)

const (
	tokenLength                  = 32
	bcryptCost                   = 10
	intentionalSlowDownHash      = "$2a$10$YFCQvqQDwIIwnJM1xkAYOeih0dg17UVGanaTStnrSzC8NCWxcLDwy" // Cost should match bcryptCost
	userStatsQueueWriterInterval = 33 * time.Second
	userTokenExpiryDuration      = 72 * time.Hour
)

var (
	errNoTokenProvided    = errors.New("no token provided")
	errTopicOwnedByOthers = errors.New("topic owned by others")
)

// Manager-related queries
const (
	createTablesQueriesNoTx = `
		CREATE TABLE IF NOT EXISTS plan (
			id INT NOT NULL,		
			code TEXT NOT NULL,
			messages_limit INT NOT NULL,
			emails_limit INT NOT NULL,
			topics_limit INT NOT NULL,
			attachment_file_size_limit INT NOT NULL,
			attachment_total_size_limit INT NOT NULL,
			PRIMARY KEY (id)
		);
		CREATE TABLE IF NOT EXISTS user (
		    id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_id INT,
			user TEXT NOT NULL,
			pass TEXT NOT NULL,
			role TEXT NOT NULL,
			messages INT NOT NULL DEFAULT (0),
			emails INT NOT NULL DEFAULT (0),			
			settings JSON,
		    FOREIGN KEY (plan_id) REFERENCES plan (id)
		);
		CREATE UNIQUE INDEX idx_user ON user (user);
		CREATE TABLE IF NOT EXISTS user_access (
			user_id INT NOT NULL,
			topic TEXT NOT NULL,
			read INT NOT NULL,
			write INT NOT NULL,
			owner_user_id INT,			
			PRIMARY KEY (user_id, topic),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS user_token (
			user_id INT NOT NULL,
			token TEXT NOT NULL,
			expires INT NOT NULL,
			PRIMARY KEY (user_id, token),
			FOREIGN KEY (user_id) REFERENCES user (id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
		INSERT INTO user (id, user, pass, role) VALUES (1, '*', '', 'anonymous') ON CONFLICT (id) DO NOTHING;
	`
	createTablesQueries   = `BEGIN; ` + createTablesQueriesNoTx + ` COMMIT;`
	selectUserByNameQuery = `
		SELECT u.user, u.pass, u.role, u.messages, u.emails, u.settings, p.code, p.messages_limit, p.emails_limit, p.topics_limit, p.attachment_file_size_limit, p.attachment_total_size_limit
		FROM user u
		LEFT JOIN plan p on p.id = u.plan_id
		WHERE user = ?		
	`
	selectUserByTokenQuery = `
		SELECT u.user, u.pass, u.role, u.messages, u.emails, u.settings, p.code, p.messages_limit, p.emails_limit, p.topics_limit, p.attachment_file_size_limit, p.attachment_total_size_limit
		FROM user u
		JOIN user_token t on u.id = t.user_id
		LEFT JOIN plan p on p.id = u.plan_id
		WHERE t.token = ? AND t.expires >= ?
	`
	selectTopicPermsQuery = `
		SELECT read, write
		FROM user_access a
		JOIN user u ON u.id = a.user_id
		WHERE (u.user = ? OR u.user = ?) AND ? LIKE a.topic
		ORDER BY u.user DESC
	`
)

// Manager-related queries
const (
	insertUserQuery      = `INSERT INTO user (user, pass, role) VALUES (?, ?, ?)`
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
	updateUserPassQuery     = `UPDATE user SET pass = ? WHERE user = ?`
	updateUserRoleQuery     = `UPDATE user SET role = ? WHERE user = ?`
	updateUserSettingsQuery = `UPDATE user SET settings = ? WHERE user = ?`
	updateUserStatsQuery    = `UPDATE user SET messages = ?, emails = ? WHERE user = ?`
	deleteUserQuery         = `DELETE FROM user WHERE user = ?`

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
	selectOtherAccessCountQuery = `
		SELECT count(*)
		FROM user_access
		WHERE (topic = ? OR ? LIKE topic)
		  AND (owner_user_id IS NULL OR owner_user_id != (SELECT id FROM user WHERE user = ?))
	`
	deleteAllAccessQuery   = `DELETE FROM user_access`
	deleteUserAccessQuery  = `DELETE FROM user_access WHERE user_id = (SELECT id FROM user WHERE user = ?)`
	deleteTopicAccessQuery = `DELETE FROM user_access WHERE user_id = (SELECT id FROM user WHERE user = ?) AND topic = ?`

	insertTokenQuery         = `INSERT INTO user_token (user_id, token, expires) VALUES ((SELECT id FROM user WHERE user = ?), ?, ?)`
	updateTokenExpiryQuery   = `UPDATE user_token SET expires = ? WHERE user_id = (SELECT id FROM user WHERE user = ?) AND token = ?`
	deleteTokenQuery         = `DELETE FROM user_token WHERE user_id = (SELECT id FROM user WHERE user = ?) AND token = ?`
	deleteExpiredTokensQuery = `DELETE FROM user_token WHERE expires < ?`
	deleteUserTokensQuery    = `DELETE FROM user_token WHERE user_id = (SELECT id FROM user WHERE user = ?)`
)

// Schema management queries
const (
	currentSchemaVersion     = 2
	insertSchemaVersion      = `INSERT INTO schemaVersion VALUES (1, ?)`
	updateSchemaVersion      = `UPDATE schemaVersion SET version = ? WHERE id = 1`
	selectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`

	// 1 -> 2 (complex migration!)
	migrate1To2RenameUserTableQueryNoTx = `
		ALTER TABLE user RENAME TO user_old;
	`
	migrate1To2InsertFromOldTablesAndDropNoTx = `
		INSERT INTO user (user, pass, role) 
		SELECT user, pass, role FROM user_old;

		INSERT INTO user_access (user_id, topic, read, write)
		SELECT u.id, a.topic, a.read, a.write
		FROM user u
	 	JOIN access a ON u.user = a.user;

		DROP TABLE access;
		DROP TABLE user_old;
	`
)

// Manager is an implementation of Manager. It stores users and access control list
// in a SQLite database.
type Manager struct {
	db                  *sql.DB
	defaultAccess       Permission       // Default permission if no ACL matches
	statsQueue          map[string]*User // Username -> User, for "unimportant" user updates
	tokenExpiryInterval time.Duration    // Duration after which tokens expire, and by which tokens are extended
	mu                  sync.Mutex
}

var _ Auther = (*Manager)(nil)

// NewManager creates a new Manager instance
func NewManager(filename string, defaultAccess Permission) (*Manager, error) {
	return newManager(filename, defaultAccess, userTokenExpiryDuration, userStatsQueueWriterInterval)
}

// NewManager creates a new Manager instance
func newManager(filename string, defaultAccess Permission, tokenExpiryDuration, statsWriterInterval time.Duration) (*Manager, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupDB(db); err != nil {
		return nil, err
	}
	manager := &Manager{
		db:                  db,
		defaultAccess:       defaultAccess,
		statsQueue:          make(map[string]*User),
		tokenExpiryInterval: tokenExpiryDuration,
	}
	go manager.userStatsQueueWriter(statsWriterInterval)
	return manager, nil
}

// Authenticate checks username and password and returns a User if correct. The method
// returns in constant-ish time, regardless of whether the user exists or the password is
// correct or incorrect.
func (a *Manager) Authenticate(username, password string) (*User, error) {
	if username == Everyone {
		return nil, ErrUnauthenticated
	}
	user, err := a.User(username)
	if err != nil {
		bcrypt.CompareHashAndPassword([]byte(intentionalSlowDownHash),
			[]byte("intentional slow-down to avoid timing attacks"))
		return nil, ErrUnauthenticated
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Hash), []byte(password)); err != nil {
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
		return nil, ErrUnauthenticated
	}
	user.Token = token
	return user, nil
}

// CreateToken generates a random token for the given user and returns it. The token expires
// after a fixed duration unless ExtendToken is called.
func (a *Manager) CreateToken(user *User) (*Token, error) {
	token, expires := util.RandomString(tokenLength), time.Now().Add(userTokenExpiryDuration)
	if _, err := a.db.Exec(insertTokenQuery, user.Name, token, expires.Unix()); err != nil {
		return nil, err
	}
	return &Token{
		Value:   token,
		Expires: expires,
	}, nil
}

// ExtendToken sets the new expiry date for a token, thereby extending its use further into the future.
func (a *Manager) ExtendToken(user *User) (*Token, error) {
	if user.Token == "" {
		return nil, errNoTokenProvided
	}
	newExpires := time.Now().Add(userTokenExpiryDuration)
	if _, err := a.db.Exec(updateTokenExpiryQuery, newExpires.Unix(), user.Name, user.Token); err != nil {
		return nil, err
	}
	return &Token{
		Value:   user.Token,
		Expires: newExpires,
	}, nil
}

// RemoveToken deletes the token defined in User.Token
func (a *Manager) RemoveToken(user *User) error {
	if user.Token == "" {
		return ErrUnauthorized
	}
	if _, err := a.db.Exec(deleteTokenQuery, user.Name, user.Token); err != nil {
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

// ChangeSettings persists the user settings
func (a *Manager) ChangeSettings(user *User) error {
	settings, err := json.Marshal(user.Prefs)
	if err != nil {
		return err
	}
	if _, err := a.db.Exec(updateUserSettingsQuery, string(settings), user.Name); err != nil {
		return err
	}
	return nil
}

// EnqueueStats adds the user to a queue which writes out user stats (messages, emails, ..) in
// batches at a regular interval
func (a *Manager) EnqueueStats(user *User) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statsQueue[user.Name] = user
}

func (a *Manager) userStatsQueueWriter(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := a.writeUserStatsQueue(); err != nil {
			log.Warn("UserManager: Writing user stats queue failed: %s", err.Error())
		}
	}
}

func (a *Manager) writeUserStatsQueue() error {
	a.mu.Lock()
	if len(a.statsQueue) == 0 {
		a.mu.Unlock()
		log.Trace("UserManager: No user stats updates to commit")
		return nil
	}
	statsQueue := a.statsQueue
	a.statsQueue = make(map[string]*User)
	a.mu.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	log.Debug("UserManager: Writing user stats queue for %d user(s)", len(statsQueue))
	for username, u := range statsQueue {
		log.Trace("UserManager: Updating stats for user %s: messages=%d, emails=%d", username, u.Stats.Messages, u.Stats.Emails)
		if _, err := tx.Exec(updateUserStatsQuery, u.Stats.Messages, u.Stats.Emails, username); err != nil {
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
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}
	if _, err = a.db.Exec(insertUserQuery, username, hash, role); err != nil {
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
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(deleteUserAccessQuery, username); err != nil {
		return err
	}
	if _, err := tx.Exec(deleteUserTokensQuery, username); err != nil {
		return err
	}
	if _, err := tx.Exec(deleteUserQuery, username); err != nil {
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

// User returns the user with the given username if it exists, or ErrNotFound otherwise.
// You may also pass Everyone to retrieve the anonymous user and its Grant list.
func (a *Manager) User(username string) (*User, error) {
	rows, err := a.db.Query(selectUserByNameQuery, username)
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
	var username, hash, role string
	var settings, planCode sql.NullString
	var messages, emails int64
	var messagesLimit, emailsLimit, topicsLimit, attachmentFileSizeLimit, attachmentTotalSizeLimit sql.NullInt64
	if !rows.Next() {
		return nil, ErrNotFound
	}
	if err := rows.Scan(&username, &hash, &role, &messages, &emails, &settings, &planCode, &messagesLimit, &emailsLimit, &topicsLimit, &attachmentFileSizeLimit, &attachmentTotalSizeLimit); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	user := &User{
		Name: username,
		Hash: hash,
		Role: Role(role),
		Stats: &Stats{
			Messages: messages,
			Emails:   emails,
		},
	}
	if settings.Valid {
		user.Prefs = &Prefs{}
		if err := json.Unmarshal([]byte(settings.String), user.Prefs); err != nil {
			return nil, err
		}
	}
	if planCode.Valid {
		user.Plan = &Plan{
			Code:                     planCode.String,
			Upgradeable:              false,
			MessagesLimit:            messagesLimit.Int64,
			EmailsLimit:              emailsLimit.Int64,
			TopicsLimit:              topicsLimit.Int64,
			AttachmentFileSizeLimit:  attachmentFileSizeLimit.Int64,
			AttachmentTotalSizeLimit: attachmentTotalSizeLimit.Int64,
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

// ChangePassword changes a user's password
func (a *Manager) ChangePassword(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
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
		if _, err := a.db.Exec(deleteUserAccessQuery, username); err != nil {
			return err
		}
	}
	return nil
}

// CheckAllowAccess tests if a user may create an access control entry for the given topic.
// If there are any ACL entries that are not owned by the user, an error is returned.
func (a *Manager) CheckAllowAccess(username string, topic string) error {
	if (!AllowedUsername(username) && username != Everyone) || !AllowedTopic(topic) {
		return ErrInvalidArgument
	}
	rows, err := a.db.Query(selectOtherAccessCountQuery, topic, topic, username)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return errors.New("no rows found")
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
func (a *Manager) AllowAccess(owner, username string, topicPattern string, read bool, write bool) error {
	if !AllowedUsername(username) && username != Everyone {
		return ErrInvalidArgument
	} else if owner != "" && !AllowedUsername(owner) {
		return ErrInvalidArgument
	} else if !AllowedTopicPattern(topicPattern) {
		return ErrInvalidArgument
	}
	if _, err := a.db.Exec(upsertUserAccessQuery, username, toSQLWildcard(topicPattern), read, write, owner, owner); err != nil {
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
		_, err := a.db.Exec(deleteUserAccessQuery, username)
		return err
	}
	_, err := a.db.Exec(deleteTopicAccessQuery, username, toSQLWildcard(topicPattern))
	return err
}

// DefaultAccess returns the default read/write access if no access control entry matches
func (a *Manager) DefaultAccess() Permission {
	return a.defaultAccess
}

func toSQLWildcard(s string) string {
	return strings.ReplaceAll(s, "*", "%")
}

func fromSQLWildcard(s string) string {
	return strings.ReplaceAll(s, "%", "*")
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
	} else if schemaVersion == 1 {
		return migrateFrom1(db)
	}
	return fmt.Errorf("unexpected schema version found: %d", schemaVersion)
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
	log.Info("Migrating user database schema: from 1 to 2")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(migrate1To2RenameUserTableQueryNoTx); err != nil {
		return err
	}
	if _, err := tx.Exec(createTablesQueriesNoTx); err != nil {
		return err
	}
	if _, err := tx.Exec(migrate1To2InsertFromOldTablesAndDropNoTx); err != nil {
		return err
	}
	if _, err := tx.Exec(updateSchemaVersion, 2); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil // Update this when a new version is added
}

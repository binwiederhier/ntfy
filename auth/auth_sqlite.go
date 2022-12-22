package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost              = 10
	intentionalSlowDownHash = "$2a$10$YFCQvqQDwIIwnJM1xkAYOeih0dg17UVGanaTStnrSzC8NCWxcLDwy" // Cost should match bcryptCost
)

// Auther-related queries
const (
	createAuthTablesQueries = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS user (
			user TEXT NOT NULL PRIMARY KEY,
			pass TEXT NOT NULL,
			role TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS access (
			user TEXT NOT NULL,
			topic TEXT NOT NULL,
			read INT NOT NULL,
			write INT NOT NULL,
			PRIMARY KEY (topic, user)
		);
		CREATE TABLE IF NOT EXISTS schemaVersion (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
		COMMIT;
	`
	selectUserQuery       = `SELECT pass, role FROM user WHERE user = ?`
	selectTopicPermsQuery = `
		SELECT read, write
		FROM access
		WHERE user IN ('*', ?) AND ? LIKE topic
		ORDER BY user DESC
	`
)

// Manager-related queries
const (
	insertUserQuery      = `INSERT INTO user (user, pass, role) VALUES (?, ?, ?)`
	selectUsernamesQuery = `SELECT user FROM user ORDER BY role, user`
	updateUserPassQuery  = `UPDATE user SET pass = ? WHERE user = ?`
	updateUserRoleQuery  = `UPDATE user SET role = ? WHERE user = ?`
	deleteUserQuery      = `DELETE FROM user WHERE user = ?`

	upsertUserAccessQuery = `
		INSERT INTO access (user, topic, read, write)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user, topic) DO UPDATE SET read=excluded.read, write=excluded.write
	`
	selectUserAccessQuery  = `SELECT topic, read, write FROM access WHERE user = ?`
	deleteAllAccessQuery   = `DELETE FROM access`
	deleteUserAccessQuery  = `DELETE FROM access WHERE user = ?`
	deleteTopicAccessQuery = `DELETE FROM access WHERE user = ? AND topic = ?`
)

// Schema management queries
const (
	currentSchemaVersion     = 1
	insertSchemaVersion      = `INSERT INTO schemaVersion VALUES (1, ?)`
	selectSchemaVersionQuery = `SELECT version FROM schemaVersion WHERE id = 1`
)

// SQLiteAuth is an implementation of Auther and Manager. It stores users and access control list
// in a SQLite database.
type SQLiteAuth struct {
	db           *sql.DB
	defaultRead  bool
	defaultWrite bool
}

var _ Auther = (*SQLiteAuth)(nil)
var _ Manager = (*SQLiteAuth)(nil)

// NewSQLiteAuth creates a new SQLiteAuth instance
func NewSQLiteAuth(filename string, defaultRead, defaultWrite bool) (*SQLiteAuth, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupAuthDB(db); err != nil {
		return nil, err
	}
	return &SQLiteAuth{
		db:           db,
		defaultRead:  defaultRead,
		defaultWrite: defaultWrite,
	}, nil
}

// Authenticate checks username and password and returns a user if correct. The method
// returns in constant-ish time, regardless of whether the user exists or the password is
// correct or incorrect.
func (a *SQLiteAuth) Authenticate(username, password string) (*User, error) {
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

// Authorize returns nil if the given user has access to the given topic using the desired
// permission. The user param may be nil to signal an anonymous user.
func (a *SQLiteAuth) Authorize(user *User, topic string, perm Permission) error {
	if user != nil && user.Role == RoleAdmin {
		return nil // Admin can do everything
	}
	username := Everyone
	if user != nil {
		username = user.Name
	}
	// Select the read/write permissions for this user/topic combo. The query may return two
	// rows (one for everyone, and one for the user), but prioritizes the user. The value for
	// user.Name may be empty (= everyone).
	rows, err := a.db.Query(selectTopicPermsQuery, username, topic)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		return a.resolvePerms(a.defaultRead, a.defaultWrite, perm)
	}
	var read, write bool
	if err := rows.Scan(&read, &write); err != nil {
		return err
	} else if err := rows.Err(); err != nil {
		return err
	}
	return a.resolvePerms(read, write, perm)
}

func (a *SQLiteAuth) resolvePerms(read, write bool, perm Permission) error {
	if perm == PermissionRead && read {
		return nil
	} else if perm == PermissionWrite && write {
		return nil
	}
	return ErrUnauthorized
}

// AddUser adds a user with the given username, password and role. The password should be hashed
// before it is stored in a persistence layer.
func (a *SQLiteAuth) AddUser(username, password string, role Role) error {
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
func (a *SQLiteAuth) RemoveUser(username string) error {
	if !AllowedUsername(username) {
		return ErrInvalidArgument
	}
	if _, err := a.db.Exec(deleteUserQuery, username); err != nil {
		return err
	}
	if _, err := a.db.Exec(deleteUserAccessQuery, username); err != nil {
		return err
	}
	return nil
}

// Users returns a list of users. It always also returns the Everyone user ("*").
func (a *SQLiteAuth) Users() ([]*User, error) {
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
	everyone, err := a.everyoneUser()
	if err != nil {
		return nil, err
	}
	users = append(users, everyone)
	return users, nil
}

// User returns the user with the given username if it exists, or ErrNotFound otherwise.
// You may also pass Everyone to retrieve the anonymous user and its Grant list.
func (a *SQLiteAuth) User(username string) (*User, error) {
	if username == Everyone {
		return a.everyoneUser()
	}
	rows, err := a.db.Query(selectUserQuery, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hash, role string
	if !rows.Next() {
		return nil, ErrNotFound
	}
	if err := rows.Scan(&hash, &role); err != nil {
		return nil, err
	} else if err := rows.Err(); err != nil {
		return nil, err
	}
	grants, err := a.readGrants(username)
	if err != nil {
		return nil, err
	}
	return &User{
		Name:   username,
		Hash:   hash,
		Role:   Role(role),
		Grants: grants,
	}, nil
}

func (a *SQLiteAuth) everyoneUser() (*User, error) {
	grants, err := a.readGrants(Everyone)
	if err != nil {
		return nil, err
	}
	return &User{
		Name:   Everyone,
		Hash:   "",
		Role:   RoleAnonymous,
		Grants: grants,
	}, nil
}

func (a *SQLiteAuth) readGrants(username string) ([]Grant, error) {
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
			AllowRead:    read,
			AllowWrite:   write,
		})
	}
	return grants, nil
}

// ChangePassword changes a user's password
func (a *SQLiteAuth) ChangePassword(username, password string) error {
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
func (a *SQLiteAuth) ChangeRole(username string, role Role) error {
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

// AllowAccess adds or updates an entry in th access control list for a specific user. It controls
// read/write access to a topic. The parameter topicPattern may include wildcards (*).
func (a *SQLiteAuth) AllowAccess(username string, topicPattern string, read bool, write bool) error {
	if (!AllowedUsername(username) && username != Everyone) || !AllowedTopicPattern(topicPattern) {
		return ErrInvalidArgument
	}
	if _, err := a.db.Exec(upsertUserAccessQuery, username, toSQLWildcard(topicPattern), read, write); err != nil {
		return err
	}
	return nil
}

// ResetAccess removes an access control list entry for a specific username/topic, or (if topic is
// empty) for an entire user. The parameter topicPattern may include wildcards (*).
func (a *SQLiteAuth) ResetAccess(username string, topicPattern string) error {
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
func (a *SQLiteAuth) DefaultAccess() (read bool, write bool) {
	return a.defaultRead, a.defaultWrite
}

func toSQLWildcard(s string) string {
	return strings.ReplaceAll(s, "*", "%")
}

func fromSQLWildcard(s string) string {
	return strings.ReplaceAll(s, "%", "*")
}

func setupAuthDB(db *sql.DB) error {
	// If 'schemaVersion' table does not exist, this must be a new database
	rowsSV, err := db.Query(selectSchemaVersionQuery)
	if err != nil {
		return setupNewAuthDB(db)
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
	}
	return fmt.Errorf("unexpected schema version found: %d", schemaVersion)
}

func setupNewAuthDB(db *sql.DB) error {
	if _, err := db.Exec(createAuthTablesQueries); err != nil {
		return err
	}
	if _, err := db.Exec(insertSchemaVersion, currentSchemaVersion); err != nil {
		return err
	}
	return nil
}

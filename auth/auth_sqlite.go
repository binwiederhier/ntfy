package auth

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
)

/*

SELECT * FROM user;
SELECT * FROM access;

INSERT INTO user VALUES ('phil','$2a$06$.4W0LI5mcxzxhpjUvpTaNeu0MhRO0T7B.CYnmAkRnlztIy7PrSODu', 'admin');
INSERT INTO user VALUES ('ben','$2a$06$skJK/AecWCUmiCjr69ke.Ow/hFA616RdvJJPxnI221zyohsRlyXL.', 'user');
INSERT INTO user VALUES ('marian','$2a$10$8U90swQIatvHHI4sw0Wo7.OUy6dUwzMcoOABi6BsS4uF0x3zcSXRW', 'user');

INSERT INTO access VALUES ('ben','alerts',1,1);
INSERT INTO access VALUES ('marian','alerts',1,0);
INSERT INTO access VALUES ('','announcements',1,0);
INSERT INTO access VALUES ('','write-all',1,1);

*/

const (
	bcryptCost = 11
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
		CREATE TABLE IF NOT EXISTS schema_version (
			id INT PRIMARY KEY,
			version INT NOT NULL
		);
		COMMIT;
	`
	selectUserQuery       = `SELECT pass, role FROM user WHERE user = ?`
	selectTopicPermsQuery = `
		SELECT read, write 
		FROM access 
		WHERE user IN ('', ?) AND topic = ?
		ORDER BY user DESC
	`
)

// Manager-related queries
const (
	insertUserQuery           = `INSERT INTO user (user, pass, role) VALUES (?, ?, ?)`
	selectUsernamesQuery      = `SELECT user FROM user ORDER BY role, user`
	selectUserTopicPermsQuery = `SELECT topic, read, write FROM access WHERE user = ?`
	updateUserPassQuery       = `UPDATE user SET pass = ? WHERE user = ?`
	updateUserRoleQuery       = `UPDATE user SET role = ? WHERE user = ?`
	upsertAccessQuery         = `
		INSERT INTO access (user, topic, read, write) 
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user, topic) DO UPDATE SET read=excluded.read, write=excluded.write
	`
	deleteUserQuery      = `DELETE FROM user WHERE user = ?`
	deleteAllAccessQuery = `DELETE FROM access WHERE user = ?`
	deleteAccessQuery    = `DELETE FROM access WHERE user = ? AND topic = ?`
)

type SQLiteAuth struct {
	db           *sql.DB
	defaultRead  bool
	defaultWrite bool
}

var _ Auther = (*SQLiteAuth)(nil)
var _ Manager = (*SQLiteAuth)(nil)

func NewSQLiteAuth(filename string, defaultRead, defaultWrite bool) (*SQLiteAuth, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupNewAuthDB(db); err != nil {
		return nil, err
	}
	return &SQLiteAuth{
		db:           db,
		defaultRead:  defaultRead,
		defaultWrite: defaultWrite,
	}, nil
}

func setupNewAuthDB(db *sql.DB) error {
	if _, err := db.Exec(createAuthTablesQueries); err != nil {
		return err
	}
	// FIXME schema version
	return nil
}

func (a *SQLiteAuth) Authenticate(username, password string) (*User, error) {
	rows, err := a.db.Query(selectUserQuery, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hash, role string
	if rows.Next() {
		if err := rows.Scan(&hash, &role); err != nil {
			return nil, err
		} else if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, err
	}
	return &User{
		Name: username,
		Role: Role(role),
	}, nil
}

func (a *SQLiteAuth) Authorize(user *User, topic string, perm Permission) error {
	if user != nil && user.Role == RoleAdmin {
		return nil // Admin can do everything
	}
	// Select the read/write permissions for this user/topic combo. The query may return two
	// rows (one for everyone, and one for the user), but prioritizes the user. The value for
	// user.Name may be empty (= everyone).
	var username string
	if user != nil {
		username = user.Name
	}
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

func (a *SQLiteAuth) AddUser(username, password string, role Role) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}
	if _, err = a.db.Exec(insertUserQuery, username, hash, role); err != nil {
		return err
	}
	return nil
}

func (a *SQLiteAuth) RemoveUser(username string) error {
	if _, err := a.db.Exec(deleteUserQuery, username); err != nil {
		return err
	}
	if _, err := a.db.Exec(deleteAllAccessQuery, username); err != nil {
		return err
	}
	return nil
}

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
	return users, nil
}

func (a *SQLiteAuth) User(username string) (*User, error) {
	urows, err := a.db.Query(selectUserQuery, username)
	if err != nil {
		return nil, err
	}
	defer urows.Close()
	var hash, role string
	if !urows.Next() {
		return nil, ErrNotFound
	}
	if err := urows.Scan(&hash, &role); err != nil {
		return nil, err
	} else if err := urows.Err(); err != nil {
		return nil, err
	}
	arows, err := a.db.Query(selectUserTopicPermsQuery, username)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	grants := make([]Grant, 0)
	for arows.Next() {
		var topic string
		var read, write bool
		if err := arows.Scan(&topic, &read, &write); err != nil {
			return nil, err
		} else if err := arows.Err(); err != nil {
			return nil, err
		}
		grants = append(grants, Grant{
			Topic: topic,
			Read:  read,
			Write: write,
		})
	}
	return &User{
		Name:   username,
		Pass:   hash,
		Role:   Role(role),
		Grants: grants,
	}, nil
}

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

func (a *SQLiteAuth) ChangeRole(username string, role Role) error {
	if _, err := a.db.Exec(updateUserRoleQuery, string(role), username); err != nil {
		return err
	}
	if role == RoleAdmin {
		if _, err := a.db.Exec(deleteAllAccessQuery, username); err != nil {
			return err
		}
	}
	return nil
}

func (a *SQLiteAuth) AllowAccess(username string, topic string, read bool, write bool) error {
	if _, err := a.db.Exec(upsertAccessQuery, username, topic, read, write); err != nil {
		return err
	}
	return nil
}

func (a *SQLiteAuth) ResetAccess(username string, topic string) error {
	if topic == "" {
		if _, err := a.db.Exec(deleteAllAccessQuery, username); err != nil {
			return err
		}
	} else {
		if _, err := a.db.Exec(deleteAccessQuery, username, topic); err != nil {
			return err
		}
	}
	return nil
}

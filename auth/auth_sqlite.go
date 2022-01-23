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
	insertUser     = `INSERT INTO user (user, pass, role) VALUES (?, ?, ?)`
	updateUserPass = `UPDATE user SET pass = ? WHERE user = ?`
	updateUserRole = `UPDATE user SET role = ? WHERE user = ?`
	upsertAccess   = `
		INSERT INTO access (user, topic, read, write) 
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user, topic) DO UPDATE SET read=excluded.read, write=excluded.write
	`
	deleteUser      = `DELETE FROM user WHERE user = ?`
	deleteAllAccess = `DELETE FROM access WHERE user = ?`
	deleteAccess    = `DELETE FROM access WHERE user = ? AND topic = ?`
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
	if user.Role == RoleAdmin {
		return nil // Admin can do everything
	}
	// Select the read/write permissions for this user/topic combo. The query may return two
	// rows (one for everyone, and one for the user), but prioritizes the user. The value for
	// user.Name may be empty (= everyone).
	rows, err := a.db.Query(selectTopicPermsQuery, user.Name, topic)
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
	if _, err = a.db.Exec(insertUser, username, hash, role); err != nil {
		return err
	}
	return nil
}

func (a *SQLiteAuth) RemoveUser(username string) error {
	if _, err := a.db.Exec(deleteUser, username); err != nil {
		return err
	}
	if _, err := a.db.Exec(deleteAllAccess, username); err != nil {
		return err
	}
	return nil
}

func (a *SQLiteAuth) ChangePassword(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}
	if _, err := a.db.Exec(updateUserPass, hash, username); err != nil {
		return err
	}
	return nil
}

func (a *SQLiteAuth) ChangeRole(username string, role Role) error {
	if _, err := a.db.Exec(updateUserRole, string(role), username); err != nil {
		return err
	}
	return nil
}

func (a *SQLiteAuth) AllowAccess(username string, topic string, read bool, write bool) error {
	if _, err := a.db.Exec(upsertAccess, username, topic, read, write); err != nil {
		return err
	}
	return nil
}

func (a *SQLiteAuth) ResetAccess(username string, topic string) error {
	if topic == "" {
		if _, err := a.db.Exec(deleteAllAccess, username); err != nil {
			return err
		}
	} else {
		if _, err := a.db.Exec(deleteAccess, username, topic); err != nil {
			return err
		}
	}
	return nil
}

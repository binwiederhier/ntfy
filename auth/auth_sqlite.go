package auth

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
)

/*

SELECT * FROM user;
SELECT * FROM user_topic;

INSERT INTO user VALUES ('phil','$2a$06$.4W0LI5mcxzxhpjUvpTaNeu0MhRO0T7B.CYnmAkRnlztIy7PrSODu', 'admin');
INSERT INTO user VALUES ('ben','$2a$06$skJK/AecWCUmiCjr69ke.Ow/hFA616RdvJJPxnI221zyohsRlyXL.', 'user');
INSERT INTO user VALUES ('marian','$2a$10$8U90swQIatvHHI4sw0Wo7.OUy6dUwzMcoOABi6BsS4uF0x3zcSXRW', 'user');

INSERT INTO user_topic VALUES ('ben','alerts',1,1);
INSERT INTO user_topic VALUES ('marian','alerts',1,0);
INSERT INTO user_topic VALUES ('','announcements',1,0);
INSERT INTO user_topic VALUES ('','write-all',1,1);

---
dabbling for CLI
	ntfy user add phil --role=admin
	ntfy user del phil
	ntfy user change-pass phil
	ntfy user allow phil mytopic
	ntfy user allow phil mytopic --read-only
	ntfy user deny phil mytopic
	ntfy user list
	   phil (admin)
	   - read-write access to everything
	   ben (user)
	   - read-write access to a topic alerts
	   - read access to
       everyone (no user)
       - read-only access to topic announcements


*/

const (
	createAuthTablesQueries = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS user (
			user TEXT NOT NULL PRIMARY KEY,
			pass TEXT NOT NULL,
			role TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS user_topic (
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
		FROM user_topic 
		WHERE user IN ('', ?) AND topic = ?
		ORDER BY user DESC
	`
)

type SQLiteAuth struct {
	db           *sql.DB
	defaultRead  bool
	defaultWrite bool
}

var _ Auth = (*SQLiteAuth)(nil)

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

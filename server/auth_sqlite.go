package server

import (
	"database/sql"
	"golang.org/x/crypto/bcrypt"
)

/*

SELECT * FROM user;
SELECT * FROM user_topic;

INSERT INTO user VALUES ('phil','$2a$06$.4W0LI5mcxzxhpjUvpTaNeu0MhRO0T7B.CYnmAkRnlztIy7PrSODu', 'admin');
INSERT INTO user VALUES ('ben','$2a$06$skJK/AecWCUmiCjr69ke.Ow/hFA616RdvJJPxnI221zyohsRlyXL.', 'user');
INSERT INTO user VALUES ('marian','$2a$06$N/BcXR0g6XUlmWttMqciWugR6xQKm2lVj31HLid6Mc4cnzpeOMgnq', 'user');

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

type sqliteAuth struct {
	db           *sql.DB
	defaultRead  bool
	defaultWrite bool
}

var _ auth = (*sqliteAuth)(nil)

func newSqliteAuth(filename string, defaultRead, defaultWrite bool) (*sqliteAuth, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupNewAuthDB(db); err != nil {
		return nil, err
	}
	return &sqliteAuth{
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

func (a *sqliteAuth) Authenticate(username, password string) (*user, error) {
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
	return &user{
		Name: username,
		Role: role,
	}, nil
}

func (a *sqliteAuth) Authorize(user *user, topic string, perm int) error {
	if user.Role == roleAdmin {
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

func (a *sqliteAuth) resolvePerms(read, write bool, perm int) error {
	if perm == permRead && read {
		return nil
	} else if perm == permWrite && write {
		return nil
	}
	return errHTTPUnauthorized
}

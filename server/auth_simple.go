package server

import (
	"database/sql"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"log"
)

/*

SELECT * FROM user;
SELECT * FROM topic;
SELECT * FROM topic_user;

INSERT INTO user VALUES('phil','$2a$06$.4W0LI5mcxzxhpjUvpTaNeu0MhRO0T7B.CYnmAkRnlztIy7PrSODu', 'admin');
INSERT INTO user VALUES('ben','$2a$06$skJK/AecWCUmiCjr69ke.Ow/hFA616RdvJJPxnI221zyohsRlyXL.', 'user');
INSERT INTO user VALUES('marian','$2a$06$N/BcXR0g6XUlmWttMqciWugR6xQKm2lVj31HLid6Mc4cnzpeOMgnq', 'user');

INSERT INTO topic_user VALUES('alerts','ben',1,1);
INSERT INTO topic_user VALUES('alerts','marian',1,0);

INSERT INTO topic VALUES('announcements',1,0);

*/

const (
	permRead  = 1
	permWrite = 2
)

const (
	roleAdmin = "admin"
	roleUser  = "user"
	roleNone  = "none"
)

const (
	createAuthTablesQueries = `
		BEGIN;
		CREATE TABLE IF NOT EXISTS user (
			user TEXT NOT NULL PRIMARY KEY,
			pass TEXT NOT NULL,
			role TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS topic (
			topic TEXT NOT NULL PRIMARY KEY,
			anon_read INT NOT NULL,
			anon_write INT NOT NULL			
		);
		CREATE TABLE IF NOT EXISTS topic_user (
			topic TEXT NOT NULL,
			user TEXT NOT NULL,		
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
	selectUserQuery           = `SELECT pass FROM user WHERE user = ?`
	selectTopicPermsAnonQuery = `SELECT ?, anon_read, anon_write FROM topic WHERE topic = ?`
	selectTopicPermsUserQuery = `
		SELECT role, IFNULL(read, 0), IFNULL(write, 0)
		FROM user
		LEFT JOIN topic_user ON user.user = topic_user.user AND topic_user.topic = ?
		WHERE user.user = ?
	`
)

type auther interface {
	Authenticate(user, pass string) error
	Authorize(user, topic string, perm int) error
}

type memAuther struct {
}

func (m *memAuther) Authenticate(user, pass string) error {
	if user == "phil" && pass == "phil" {
		return nil
	}
	return errHTTPUnauthorized
}

func (m *memAuther) Authorize(user, topic string, perm int) error {
	if perm == permRead {
		return nil
	}
	if user == "phil" && topic == "mytopic" {
		return nil
	}
	return errHTTPUnauthorized
}

type sqliteAuther struct {
	db           *sql.DB
	defaultRead  bool
	defaultWrite bool
}

var _ auther = (*sqliteAuther)(nil)

func newSqliteAuther(filename string, defaultRead, defaultWrite bool) (*sqliteAuther, error) {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := setupNewAuthDB(db); err != nil {
		return nil, err
	}
	return &sqliteAuther{
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

func (a *sqliteAuther) Authenticate(user, pass string) error {
	rows, err := a.db.Query(selectUserQuery, user)
	if err != nil {
		return err
	}
	defer rows.Close()
	var hash string
	if !rows.Next() {
		return fmt.Errorf("user %s not found", user)
	}
	if err := rows.Scan(&hash); err != nil {
		return err
	} else if err := rows.Err(); err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass))
}

func (a *sqliteAuther) Authorize(user, topic string, perm int) error {
	if user == "" {
		return a.authorizeAnon(topic, perm)
	}
	return a.authorizeUser(user, topic, perm)
}

func (a *sqliteAuther) authorizeAnon(topic string, perm int) error {
	rows, err := a.db.Query(selectTopicPermsAnonQuery, roleNone, topic)
	if err != nil {
		return err
	}
	return a.checkPerms(rows, perm)
}

func (a *sqliteAuther) authorizeUser(user string, topic string, perm int) error {
	rows, err := a.db.Query(selectTopicPermsUserQuery, topic, user)
	if err != nil {
		return err
	}
	return a.checkPerms(rows, perm)
}

func (a *sqliteAuther) checkPerms(rows *sql.Rows, perm int) error {
	defer rows.Close()
	if !rows.Next() {
		return a.resolvePerms(a.defaultRead, a.defaultWrite, perm)
	}
	var role string
	var read, write bool
	if err := rows.Scan(&role, &read, &write); err != nil {
		return err
	} else if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("%#v, %#v, %#v", role, read, write)
	if role == roleAdmin {
		return nil // Admin can do everything
	}
	return a.resolvePerms(read, write, perm)
}

func (a *sqliteAuther) resolvePerms(read, write bool, perm int) error {
	if perm == permRead && read {
		return nil
	} else if perm == permWrite && write {
		return nil
	}
	return errHTTPUnauthorized
}

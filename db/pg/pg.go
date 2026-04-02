package pg

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver

	"heckel.io/ntfy/v2/db"
)

// Open opens a PostgreSQL connection pool for a primary database. It pings the database
// to verify connectivity before returning.
func Open(dsn string) (*db.Host, error) {
	d, err := open(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := d.DB.Ping(); err != nil {
		return nil, fmt.Errorf("database ping failed on %v: %w", d.Addr, err)
	}
	return d, nil
}

// OpenReplica opens a PostgreSQL connection pool for a read replica. Unlike Open, it does
// not ping the database, since replicas are health-checked in the background by db.DB.
func OpenReplica(dsn string) (*db.Host, error) {
	return open(dsn)
}

// open opens a PostgreSQL database connection pool from a DSN string. It supports custom
// query parameters for pool configuration: pool_max_conns (default 10), pool_max_idle_conns,
// pool_conn_max_lifetime, and pool_conn_max_idle_time. These parameters are stripped from
// the DSN before passing it to the driver.
func open(dsn string) (*db.Host, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}
	switch u.Scheme {
	case "postgres", "postgresql":
		// OK
	default:
		return nil, fmt.Errorf("invalid database URL scheme %q, must be \"postgres\" or \"postgresql\" (URL: %s)", u.Scheme, censorPassword(u))
	}
	q := u.Query()
	maxOpenConns, err := extractIntParam(q, "pool_max_conns", 10)
	if err != nil {
		return nil, err
	}
	maxIdleConns, err := extractIntParam(q, "pool_max_idle_conns", 0)
	if err != nil {
		return nil, err
	}
	connMaxLifetime, err := extractDurationParam(q, "pool_conn_max_lifetime", 0)
	if err != nil {
		return nil, err
	}
	connMaxIdleTime, err := extractDurationParam(q, "pool_conn_max_idle_time", 0)
	if err != nil {
		return nil, err
	}
	u.RawQuery = q.Encode()
	d, err := sql.Open("pgx", u.String())
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(maxOpenConns)
	if maxIdleConns > 0 {
		d.SetMaxIdleConns(maxIdleConns)
	}
	if connMaxLifetime > 0 {
		d.SetConnMaxLifetime(connMaxLifetime)
	}
	if connMaxIdleTime > 0 {
		d.SetConnMaxIdleTime(connMaxIdleTime)
	}
	return &db.Host{
		Addr: u.Host,
		DB:   d,
	}, nil
}

func extractIntParam(q url.Values, key string, defaultValue int) (int, error) {
	s := q.Get(key)
	if s == "" {
		return defaultValue, nil
	}
	q.Del(key)
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", key, s, err)
	}
	return v, nil
}

// censorPassword returns a string representation of the URL with the password replaced by "*****".
func censorPassword(u *url.URL) string {
	if password, hasPassword := u.User.Password(); hasPassword {
		return strings.Replace(u.String(), ":"+password+"@", ":*****@", 1)
	}
	return u.String()
}

func extractDurationParam(q url.Values, key string, defaultValue time.Duration) (time.Duration, error) {
	s := q.Get(key)
	if s == "" {
		return defaultValue, nil
	}
	q.Del(key)
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", key, s, err)
	}
	return d, nil
}

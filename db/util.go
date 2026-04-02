package db

import "database/sql"

// ExecTx executes a function within a database transaction. If the function returns an error,
// the transaction is rolled back. Otherwise, the transaction is committed.
func ExecTx(db Beginner, f func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// QueryTx executes a function within a database transaction and returns the result. If the function
// returns an error, the transaction is rolled back. Otherwise, the transaction is committed.
func QueryTx[T any](db Beginner, f func(tx *sql.Tx) (T, error)) (T, error) {
	tx, err := db.Begin()
	if err != nil {
		var zero T
		return zero, err
	}
	defer tx.Rollback()
	t, err := f(tx)
	if err != nil {
		return t, err
	}
	if err := tx.Commit(); err != nil {
		return t, err
	}
	return t, nil
}

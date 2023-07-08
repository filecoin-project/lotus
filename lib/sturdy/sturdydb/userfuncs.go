package sturdydb

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
)

// rawStringOnly is _intentionally_private_ to force only basic strings in SQL queries.
// In any package, raw strings will satisfy compilation.  Ex:
//
//	sturdydb.Exec("INSERT INTO version (number) VALUES (1)")
//
// This prevents SQL injection attacks where the input contains query fragments.
type rawStringOnly string

// Exec executes changes (INSERT, DELETE,  or UPDATE).
// Note, for CREATE & DROP please keep these permanent and express
// them in the ./sql/ files (next number).
func (db *DB) Exec(ctx context.Context, sql rawStringOnly, arguments ...any) error {
	_, err := db.pgx.Exec(ctx, string(sql), arguments...)
	return err
}

type Qry interface {
	Next() bool
	Err() error
	Close()
	Scan(...any) error
	Values() ([]any, error)
}

// Query offers Next/Err/Close/Scan/Values/StructScan
type Query struct {
	Qry
}

// Query allows iterating returned values to save memory consumption
// with the downside of needing to `defer q.Close()`. For a simpler interface,
// try Select()
// Next() must be called to advance the row cursor, including the first time:
// Ex:
// q, err := db.Query(ctx, "SELECT id, name FROM users")
// handleError(err)
// defer q.Close()
//
//	for q.Next() {
//		  var id int
//	   var name string
//	   handleError(q.Scan(&id, &name))
//	   fmt.Println(id, name)
//	}
func (db *DB) Query(ctx context.Context, sql rawStringOnly, arguments ...any) (*Query, error) {
	q, err := db.pgx.Query(ctx, string(sql), arguments...)
	return &Query{q}, err
}
func (q *Query) StructScan(s any) error {
	return pgxscan.ScanRow(s, q.Qry.(pgx.Rows))
}

type Row interface {
	Scan(...any) error
}

// QueryRow gets 1 row using column order matching.
// This is a timesaver for the special case of wanting the first row returned only.
// EX:
//
//	var name, pet string
//	var ID = 123
//	err := db.QueryRow(ctx, "SELECT name, pet FROM users WHERE ID=?", ID).Scan(&name, &pet)
func (db *DB) QueryRow(ctx context.Context, sql rawStringOnly, arguments ...any) Row {
	return db.pgx.QueryRow(ctx, string(sql), arguments...)
}

/*
Select multiple rows into a slice using name matching
Ex:

	type user struct {
		Name string
		ID int
		Number string `db:"tel_no"`
	}

	var users []user
	pet := "cat"
	err := db.Select(ctx, &users, "SELECT name, id, tel_no FROM customers WHERE pet=?", pet)
*/
func (db *DB) Select(ctx context.Context, sliceOfStructPtr any, sql rawStringOnly, arguments ...any) error {
	return pgxscan.Select(ctx, db.pgx, sliceOfStructPtr, string(sql), arguments...)
}

type Transaction struct {
	pgx.Tx
}

// BeginTransaction is how you can access transactions using this library.
// The entire transaction happens in the function passed in.
// The return must be true or a rollback will occur.
func (db *DB) BeginTransaction(ctx context.Context, f func(t *Transaction) (commit bool)) (retErr error) {
	tx, err := db.pgx.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	var commit bool
	defer func() { // Panic clean-up.
		if !commit {
			retErr = tx.Rollback(ctx)
		}
	}()
	commit = f(&Transaction{tx})
	if commit {
		return tx.Commit(ctx)
	}
	return nil
}

// Exec in a transaction.
func (t *Transaction) Exec(ctx context.Context, sql rawStringOnly, arguments ...any) error {
	_, err := t.Tx.Exec(ctx, string(sql), arguments...)
	return err
}

// Query in a transaction.
func (t *Transaction) Query(ctx context.Context, sql rawStringOnly, arguments ...any) (*Query, error) {
	q, err := t.Tx.Query(ctx, string(sql), arguments...)
	return &Query{q}, err
}

// QueryRow in a transaction.
func (t *Transaction) QueryRow(ctx context.Context, sql rawStringOnly, arguments ...any) Row {
	return t.Tx.QueryRow(ctx, string(sql), arguments...)
}

// Select in a transaction.
func (t *Transaction) Select(ctx context.Context, sliceOfStructPtr any, sql rawStringOnly, arguments ...any) error {
	return pgxscan.Select(ctx, t.Tx, sliceOfStructPtr, string(sql), arguments...)
}

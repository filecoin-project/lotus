package harmonydb

import (
	"context"
	"errors"
	"runtime"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/samber/lo"
)

var errTx = errors.New("Cannot use a non-transaction func in a transaction")

// rawStringOnly is _intentionally_private_ to force only basic strings in SQL queries.
// In any package, raw strings will satisfy compilation.  Ex:
//
//	harmonydb.Exec("INSERT INTO version (number) VALUES (1)")
//
// This prevents SQL injection attacks where the input contains query fragments.
type rawStringOnly string

// Exec executes changes (INSERT, DELETE,  or UPDATE).
// Note, for CREATE & DROP please keep these permanent and express
// them in the ./sql/ files (next number).
func (db *DB) Exec(ctx context.Context, sql rawStringOnly, arguments ...any) (count int, err error) {
	if db.usedInTransaction() {
		return 0, errTx
	}
	res, err := db.pgx.Exec(ctx, string(sql), arguments...)
	return int(res.RowsAffected()), err
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
	if db.usedInTransaction() {
		return &Query{}, errTx
	}
	q, err := db.pgx.Query(ctx, string(sql), arguments...)
	return &Query{q}, err
}
func (q *Query) StructScan(s any) error {
	return pgxscan.ScanRow(s, q.Qry.(pgx.Rows))
}

type Row interface {
	Scan(...any) error
}

type rowErr struct{}

func (rowErr) Scan(_ ...any) error { return errTx }

// QueryRow gets 1 row using column order matching.
// This is a timesaver for the special case of wanting the first row returned only.
// EX:
//
//	var name, pet string
//	var ID = 123
//	err := db.QueryRow(ctx, "SELECT name, pet FROM users WHERE ID=?", ID).Scan(&name, &pet)
func (db *DB) QueryRow(ctx context.Context, sql rawStringOnly, arguments ...any) Row {
	if db.usedInTransaction() {
		return rowErr{}
	}
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
	if db.usedInTransaction() {
		return errTx
	}
	return pgxscan.Select(ctx, db.pgx, sliceOfStructPtr, string(sql), arguments...)
}

type Tx struct {
	pgx.Tx
	ctx context.Context
}

// usedInTransaction is a helper to prevent nesting transactions
// & non-transaction calls in transactions. It only checks 20 frames.
// Fast: This memory should all be in CPU Caches.
func (db *DB) usedInTransaction() bool {
	var framePtrs = (&[20]uintptr{})[:]                   // 20 can be stack-local (no alloc)
	framePtrs = framePtrs[:runtime.Callers(3, framePtrs)] // skip past our caller.
	return lo.Contains(framePtrs, db.BTFP.Load())         // Unsafe read @ beginTx overlap, but 'return false' is correct there.
}

// BeginTransaction is how you can access transactions using this library.
// The entire transaction happens in the function passed in.
// The return must be true or a rollback will occur.
// Be sure to test the error for IsErrSerialization() if you want to retry
//
//	when there is a DB serialization error.
//
//go:noinline
func (db *DB) BeginTransaction(ctx context.Context, f func(*Tx) (commit bool, err error)) (didCommit bool, retErr error) {
	db.BTFPOnce.Do(func() {
		fp := make([]uintptr, 20)
		runtime.Callers(1, fp)
		db.BTFP.Store(fp[0])
	})
	if db.usedInTransaction() {
		return false, errTx
	}
	tx, err := db.pgx.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	var commit bool
	defer func() { // Panic clean-up.
		if !commit {
			if tmp := tx.Rollback(ctx); tmp != nil {
				retErr = tmp
			}
		}
	}()
	commit, err = f(&Tx{tx, ctx})
	if err != nil {
		return false, err
	}
	if commit {
		err = tx.Commit(ctx)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// Exec in a transaction.
func (t *Tx) Exec(sql rawStringOnly, arguments ...any) (count int, err error) {
	res, err := t.Tx.Exec(t.ctx, string(sql), arguments...)
	return int(res.RowsAffected()), err
}

// Query in a transaction.
func (t *Tx) Query(sql rawStringOnly, arguments ...any) (*Query, error) {
	q, err := t.Tx.Query(t.ctx, string(sql), arguments...)
	return &Query{q}, err
}

// QueryRow in a transaction.
func (t *Tx) QueryRow(sql rawStringOnly, arguments ...any) Row {
	return t.Tx.QueryRow(t.ctx, string(sql), arguments...)
}

// Select in a transaction.
func (t *Tx) Select(sliceOfStructPtr any, sql rawStringOnly, arguments ...any) error {
	return pgxscan.Select(t.ctx, t.Tx, sliceOfStructPtr, string(sql), arguments...)
}

func IsErrUniqueContraint(err error) bool {
	var e2 *pgconn.PgError
	return errors.As(err, &e2) && e2.Code == pgerrcode.UniqueViolation
}

func IsErrSerialization(err error) bool {
	var e2 *pgconn.PgError
	return errors.As(err, &e2) && e2.Code == pgerrcode.SerializationFailure
}

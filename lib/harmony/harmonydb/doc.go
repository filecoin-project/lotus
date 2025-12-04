/*
# HarmonyDB provides database abstractions over SP-wide Postgres-compatible instance(s).

# Features

	Rolling to secondary database servers on connection failure
	Convenience features for Go + SQL
	Prevention of SQL injection vulnerabilities
	Monitors behavior via Prometheus stats and logging of errors.

# Usage

Processes should use New() to instantiate a *DB and keep it.
Consumers can use this *DB concurrently.
Creating and changing tables & views should happen in ./sql/ folder.
Name the file "today's date" in the format: YYYYMMDD.sql (ex: 20231231.sql for the year's last day)

	a. CREATE TABLE should NOT have a schema:
		GOOD: CREATE TABLE foo ();
		BAD:  CREATE TABLE me.foo ();
	b. Schema is managed for you. It provides isolation for integration tests & multi-use.
	c. Git Merges: All run once, so old-after-new is OK when there are no deps.
	d. NEVER change shipped sql files. Have later files make corrections.
	e. Anything not ran will be ran, so an older date making it to master is OK.

Write SQL with context, raw strings, and args:

	name := "Alice"
	var ID int
	err := QueryRow(ctx, "SELECT id from people where first_name=?", name).Scan(&ID)
	fmt.Println(ID)

Note: Scan() is column-oriented, while Select() & StructScan() is field name/tag oriented.
*/
package harmonydb

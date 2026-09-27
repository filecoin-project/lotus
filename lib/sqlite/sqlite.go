package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	logging "github.com/ipfs/go-log/v2"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"
)

var log = logging.Logger("sqlite")

type MigrationFunc func(ctx context.Context, tx *sql.Tx) error

var pragmas = []string{
	"PRAGMA synchronous = normal",
	"PRAGMA temp_store = memory",
	"PRAGMA mmap_size = 30000000000",
	"PRAGMA auto_vacuum = NONE",
	"PRAGMA automatic_index = OFF",
	"PRAGMA journal_mode = WAL",
	"PRAGMA journal_size_limit = 0", // always reset journal and wal files
	"PRAGMA foreign_keys = ON",
}

const metaTableDdl = `CREATE TABLE IF NOT EXISTS _meta (
	version UINT64 NOT NULL UNIQUE
)`

const (
	// vacuumMinFreePages is the smallest number of free pages (a page is typically 4KiB) that
	// makes a post-migration VACUUM worth its cost.
	vacuumMinFreePages = 1000
	// vacuumMinFreeRatioPercent is the smallest percentage of the database that has to hold free
	// pages for a post-migration VACUUM to be worth its cost.
	vacuumMinFreeRatioPercent = 1
)

// metaDdl returns the DDL statements required to create the _meta table and add the required
// up to the given version.
func metaDdl(version uint64) []string {
	var ddls []string
	for i := 1; i <= int(version); i++ {
		ddls = append(ddls, `INSERT OR IGNORE INTO _meta (version) VALUES (`+strconv.Itoa(i)+`)`)
	}
	return append([]string{metaTableDdl}, ddls...)
}

// Open opens a database at the given path. If the database does not exist, it will be created.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, xerrors.Errorf("error creating database base directory [@ %s]: %w", path, err)
	}

	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, xerrors.Errorf("error checking file status for database [@ %s]: %w", path, err)
	}

	db, err := sql.Open("sqlite3", path+"?mode=rwc")
	if err != nil {
		return nil, xerrors.Errorf("error opening database [@ %s]: %w", path, err)
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, xerrors.Errorf("error setting database pragma %q: %w", pragma, err)
		}
	}

	var foreignKeysEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys;").Scan(&foreignKeysEnabled); err != nil {
		return nil, xerrors.Errorf("failed to check foreign keys setting: %w", err)
	}
	if foreignKeysEnabled == 0 {
		return nil, xerrors.Errorf("foreign keys are not enabled for database [@ %s]", path)
	}

	return db, nil
}

// InitDb initializes the database by checking whether it needs to be created or upgraded.
// The ddls are the DDL statements to create the tables in the database and their initial required
// content. The schemaVersion will be set inside the database if it is newly created. Otherwise, the
// version is read from the database and returned. This value should be checked against the expected
// version to determine if the database needs to be upgraded.
// It is up to the caller to close the database if an error is returned by this function.
func InitDb(
	ctx context.Context,
	name string,
	db *sql.DB,
	ddls []string,
	versionMigrations []MigrationFunc,
) error {

	schemaVersion := len(versionMigrations) + 1

	q, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='_meta';")
	if q != nil {
		defer func() { _ = q.Close() }()
	}

	if errors.Is(err, sql.ErrNoRows) || !q.Next() {
		// empty database, create the schema including the _meta table and its versions
		ddls := append(metaDdl(uint64(schemaVersion)), ddls...)
		for _, ddl := range ddls {
			if _, err := db.Exec(ddl); err != nil {
				return xerrors.Errorf("failed to %s database execute ddl %q: %w", name, ddl, err)
			}
		}
		return nil
	}

	if err != nil {
		return xerrors.Errorf("error looking for %s database _meta table: %w", name, err)
	}

	if err := q.Close(); err != nil {
		return xerrors.Errorf("error closing %s database _meta table query: %w", name, err)
	}

	// check the schema version to see if we need to upgrade the database schema
	var foundVersion int
	if err = db.QueryRow("SELECT max(version) FROM _meta").Scan(&foundVersion); err != nil {
		return xerrors.Errorf("invalid %s database version: no version found", name)
	}

	if foundVersion > schemaVersion {
		return xerrors.Errorf("invalid %s database version: version %d is greater than the number of migrations %d", name, foundVersion, len(versionMigrations))
	}

	runVacuum := foundVersion != schemaVersion

	// run a migration for each version that we have not yet applied, where foundVersion is what is
	// currently in the database and schemaVersion is the target version. If they are the same,
	// nothing is run.
	for i := foundVersion + 1; i <= schemaVersion; i++ {
		now := time.Now()

		log.Infof("Migrating %s database to version %d...", name, i)

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return xerrors.Errorf("failed to start %s database transaction: %w", name, err)
		}
		defer func() { _ = tx.Rollback() }()
		// versions start at 1, but the migrations are 0-indexed where the first migration would take us to version 2
		if err := versionMigrations[i-2](ctx, tx); err != nil {
			return xerrors.Errorf("failed to migrate %s database to version %d: %w", name, i, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO _meta (version) VALUES (?)`, i); err != nil {
			return xerrors.Errorf("failed to update %s database _meta table: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return xerrors.Errorf("failed to commit %s database v%d migration transaction: %w", name, i, err)
		}

		log.Infof("Successfully migrated %s database from version %d to %d in %s", name, i-1, i, time.Since(now))
	}

	if runVacuum {
		// During the large migrations, we have likely increased the WAL size a lot, so lets do some
		// simple DB administration to free up space (VACUUM followed by truncating the WAL file)
		// as this would be a good time to do it when no other writes are happening.
		//
		// A VACUUM rewrites the whole database into a temporary copy and then replaces the original
		// with it, so it is only worth running when the migration actually left free pages behind
		// for us to reclaim: migrations that only add tables or indexes (the common case) free
		// nothing, and an unconditional VACUUM then spends a full database rewrite to reclaim
		// nothing. For a chain index database (tens of GiB) that can add hours to node startup.
		pageCount, freelistCount, err := databasePages(ctx, db)
		switch {
		case err != nil:
			log.Warnf("error checking free pages in %s database, skipping vacuum: %s", name, err)
		case !vacuumWorthwhile(pageCount, freelistCount):
			log.Infof("Skipping %s database vacuum: only %d of %d pages are free space, which is not worth a full database rewrite", name, freelistCount, pageCount)
		default:
			log.Infof("Performing %s database vacuum to reclaim %d free pages out of %d", name, freelistCount, pageCount)
			if err := vacuum(ctx, db); err != nil {
				log.Warnf("error vacuuming %s database: %s", name, err)
			}
		}
		// Truncating the WAL is cheap compared to a VACUUM and returns whatever the migration left
		// in it, so it is always worth doing once the schema has changed.
		if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			log.Warnf("error checkpointing %s database wal: %s", name, err)
		}
	}

	return nil
}

// databasePages returns the total number of pages in the database and the number of those pages
// that are on the freelist, i.e. the space that a VACUUM would return to the filesystem.
func databasePages(ctx context.Context, db *sql.DB) (pageCount, freelistCount int64, err error) {
	if err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, 0, xerrors.Errorf("error reading database page count: %w", err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&freelistCount); err != nil {
		return 0, 0, xerrors.Errorf("error reading database freelist count: %w", err)
	}
	return pageCount, freelistCount, nil
}

// vacuumWorthwhile reports whether the free pages in a database are a large enough part of it to be
// worth rewriting the whole database for.
func vacuumWorthwhile(pageCount, freelistCount int64) bool {
	if pageCount <= 0 || freelistCount < vacuumMinFreePages {
		return false
	}
	return freelistCount*100 >= pageCount*vacuumMinFreeRatioPercent
}

// vacuum rebuilds the database, returning the space held by free pages to the filesystem.
//
// It runs on a connection of its own with temp_store forced to file, because VACUUM copies the
// whole database into a temporary database before replacing the original: with the
// temp_store = memory pragma set by Open(), that temporary copy is held in RAM, and a
// database-sized allocation is enough to exhaust the memory of the machine running the node, push
// it into swap and turn the vacuum into an hours-long step that also risks the OOM killer.
//
// Note that SQLite chooses the temporary file location from SQLITE_TMPDIR, TMPDIR, /var/tmp or
// /tmp; wherever that resolves to needs room for a database-sized temporary file (and on systems
// where /tmp is a memory backed tmpfs, SQLITE_TMPDIR has to be pointed at a disk backed directory
// for this to help).
func vacuum(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return xerrors.Errorf("error getting database connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "PRAGMA temp_store = file"); err != nil {
		return xerrors.Errorf("error setting temp_store for vacuum: %w", err)
	}

	// 0 == DEFAULT, 1 == FILE, 2 == MEMORY
	var tempStore int
	if err := conn.QueryRowContext(ctx, "PRAGMA temp_store").Scan(&tempStore); err != nil {
		return xerrors.Errorf("error checking temp_store for vacuum: %w", err)
	}
	if tempStore != 1 {
		return xerrors.Errorf("refusing to vacuum with temp_store %d (expected file): the temporary database would be held in memory", tempStore)
	}

	if _, err := conn.ExecContext(ctx, "VACUUM"); err != nil {
		return xerrors.Errorf("error vacuuming database: %w", err)
	}
	return nil
}

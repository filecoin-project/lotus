package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVacuumWorthwhile(t *testing.T) {
	for _, tc := range []struct {
		name          string
		pageCount     int64
		freelistCount int64
		expected      bool
	}{
		{name: "empty database", pageCount: 0, freelistCount: 0, expected: false},
		// a real chain index database that has just been migrated and never had anything removed
		// from it: there is nothing for a vacuum to reclaim
		{name: "no free pages", pageCount: 11_255_354, freelistCount: 0, expected: false},
		{name: "below the absolute threshold", pageCount: 1_000_000, freelistCount: 999, expected: false},
		{name: "below the ratio threshold", pageCount: 1_000_000, freelistCount: 9_999, expected: false},
		{name: "at the ratio threshold", pageCount: 100_000, freelistCount: 1_000, expected: true},
		{name: "above both thresholds", pageCount: 1_000_000, freelistCount: 100_000, expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, vacuumWorthwhile(tc.pageCount, tc.freelistCount))
		})
	}
}

// TestVacuumReclaimsFreePages checks that the post-migration vacuum returns the pages freed by
// deleted data to the filesystem.
func TestVacuumReclaimsFreePages(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(dbPath)
	require.NoError(t, err)

	require.NoError(t, InitDb(ctx, "testdb", db, []string{
		`CREATE TABLE IF NOT EXISTS blip (id INTEGER PRIMARY KEY, blip_name BLOB NOT NULL)`,
	}, nil))

	// fill the database up, then remove most of it again so that the pages it used become free
	// pages, i.e. space that only a vacuum can return to the filesystem
	_, err = db.Exec(`INSERT INTO blip (blip_name)
		SELECT randomblob(1024) FROM (
			WITH RECURSIVE cnt(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM cnt WHERE x < 50000)
			SELECT x FROM cnt
		)`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM blip WHERE id % 10 != 0`)
	require.NoError(t, err)

	// reopening, as a node restart does, leaves the freelist as it is
	require.NoError(t, db.Close())

	db, err = Open(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	pageCount, freelistCount, err := databasePages(ctx, db)
	require.NoError(t, err)
	require.True(t, vacuumWorthwhile(pageCount, freelistCount),
		"expected %d free pages out of %d to be worth a vacuum", freelistCount, pageCount)

	before, err := os.Stat(dbPath)
	require.NoError(t, err)

	require.NoError(t, vacuum(ctx, db))

	_, freelistCount, err = databasePages(ctx, db)
	require.NoError(t, err)
	require.Zero(t, freelistCount, "vacuum should have returned the free pages to the filesystem")

	after, err := os.Stat(dbPath)
	require.NoError(t, err)
	require.Less(t, after.Size(), before.Size(), "vacuum should have shrunk the database file")
}

// TestInitDbMigratesWithoutVacuum checks that a migration is committed and the database is left
// usable when the vacuum is skipped for lack of free pages to reclaim.
func TestInitDbMigratesWithoutVacuum(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	ddl := []string{
		`CREATE TABLE IF NOT EXISTS blip (id INTEGER PRIMARY KEY, blip_name TEXT NOT NULL)`,
	}
	require.NoError(t, InitDb(ctx, "testdb", db, ddl, nil))

	_, err = db.Exec(`INSERT INTO blip (blip_name) VALUES ('blip1')`)
	require.NoError(t, err)

	migration := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`ALTER TABLE blip ADD COLUMN blip_extra TEXT NOT NULL DEFAULT '!'`)
		return err
	}
	require.NoError(t, InitDb(ctx, "testdb", db, ddl, []MigrationFunc{migration}))

	// the migration version is committed whether or not the vacuum ran
	var version int
	require.NoError(t, db.QueryRow(`SELECT max(version) FROM _meta`).Scan(&version))
	require.Equal(t, 2, version)

	// and this database has nothing for a vacuum to reclaim
	pageCount, freelistCount, err := databasePages(ctx, db)
	require.NoError(t, err)
	require.False(t, vacuumWorthwhile(pageCount, freelistCount))

	var name string
	require.NoError(t, db.QueryRow(`SELECT blip_name FROM blip`).Scan(&name))
	require.Equal(t, "blip1", name)
}

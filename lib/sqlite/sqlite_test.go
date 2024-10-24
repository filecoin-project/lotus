package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/lib/sqlite"
)

func TestSqlite(t *testing.T) {
	req := require.New(t)

	ddl := []string{
		`CREATE TABLE IF NOT EXISTS blip (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			blip_name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS bloop (
		 	blip_id INTEGER NOT NULL,
			bloop_name TEXT NOT NULL,
			FOREIGN KEY (blip_id) REFERENCES blip(id)
		 )`,
		`CREATE INDEX IF NOT EXISTS blip_name_index ON blip (blip_name)`,
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "/test.db")

	db, err := sqlite.Open(dbPath)
	req.NoError(err)
	req.NotNil(db)

	err = sqlite.InitDb(context.Background(), "testdb", db, ddl, nil)
	req.NoError(err)

	// insert some data

	r, err := db.Exec("INSERT INTO blip (blip_name) VALUES ('blip1')")
	req.NoError(err)
	id, err := r.LastInsertId()
	req.NoError(err)
	req.Equal(int64(1), id)
	_, err = db.Exec("INSERT INTO bloop (blip_id, bloop_name) VALUES (?, 'bloop1')", id)
	req.NoError(err)
	r, err = db.Exec("INSERT INTO blip (blip_name) VALUES ('blip2')")
	req.NoError(err)
	id, err = r.LastInsertId()
	req.NoError(err)
	req.Equal(int64(2), id)
	_, err = db.Exec("INSERT INTO bloop (blip_id, bloop_name) VALUES (?, 'bloop2')", id)
	req.NoError(err)

	// check that the db contains what we think it should

	expectedIndexes := []string{"blip_name_index"}

	expectedData := []tabledata{
		{
			name: "_meta",
			cols: []string{"version"},
			data: [][]interface{}{
				{int64(1)},
			},
		},
		{
			name: "blip",
			cols: []string{"id", "blip_name"},
			data: [][]interface{}{
				{int64(1), "blip1"},
				{int64(2), "blip2"},
			},
		},
		{
			name: "bloop",
			cols: []string{"blip_id", "bloop_name"},
			data: [][]interface{}{
				{int64(1), "bloop1"},
				{int64(2), "bloop2"},
			},
		},
	}

	actualIndexes, actualData := dumpTables(t, db)
	req.Equal(expectedIndexes, actualIndexes)
	req.Equal(expectedData, actualData)

	req.NoError(db.Close())

	// open again, check contents is the same

	db, err = sqlite.Open(dbPath)
	req.NoError(err)
	req.NotNil(db)

	err = sqlite.InitDb(context.Background(), "testdb", db, ddl, nil)
	req.NoError(err)

	// database should contain the same things

	actualIndexes, actualData = dumpTables(t, db)
	req.Equal(expectedIndexes, actualIndexes)
	req.Equal(expectedData, actualData)

	req.NoError(db.Close())

	// open again, with a migration

	db, err = sqlite.Open(dbPath)
	req.NoError(err)
	req.NotNil(db)
	req.NotNil(db)

	migration1 := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec("ALTER TABLE blip ADD COLUMN blip_extra TEXT NOT NULL DEFAULT '!'")
		return err
	}

	err = sqlite.InitDb(context.Background(), "testdb", db, ddl, []sqlite.MigrationFunc{migration1})
	req.NoError(err)

	// also add something new
	r, err = db.Exec("INSERT INTO blip (blip_name, blip_extra) VALUES ('blip1', '!!!')")
	req.NoError(err)
	id, err = r.LastInsertId()
	req.NoError(err)
	_, err = db.Exec("INSERT INTO bloop (blip_id, bloop_name) VALUES (?, 'bloop3')", id)
	req.NoError(err)

	// database should contain new stuff

	expectedData[0].data = append(expectedData[0].data, []interface{}{int64(2)}) // _meta schema version 2
	expectedData[1] = tabledata{
		name: "blip",
		cols: []string{"id", "blip_name", "blip_extra"},
		data: [][]interface{}{
			{int64(1), "blip1", "!"},
			{int64(2), "blip2", "!"},
			{int64(3), "blip1", "!!!"},
		},
	}
	expectedData[2].data = append(expectedData[2].data, []interface{}{int64(3), "bloop3"})

	actualIndexes, actualData = dumpTables(t, db)
	req.Equal(expectedIndexes, actualIndexes)
	req.Equal(expectedData, actualData)

	req.NoError(db.Close())

	// open again, with another migration

	db, err = sqlite.Open(dbPath)
	req.NoError(err)
	req.NotNil(db)

	migration2 := func(ctx context.Context, tx *sql.Tx) error {
		// add an index
		_, err := tx.Exec("CREATE INDEX IF NOT EXISTS blip_extra_index ON blip (blip_extra)")
		return err
	}

	err = sqlite.InitDb(context.Background(), "testdb", db, ddl, []sqlite.MigrationFunc{migration1, migration2})
	req.NoError(err)

	// database should contain new stuff

	expectedData[0].data = append(expectedData[0].data, []interface{}{int64(3)}) // _meta schema version 3
	expectedIndexes = append(expectedIndexes, "blip_extra_index")

	actualIndexes, actualData = dumpTables(t, db)
	req.Equal(expectedIndexes, actualIndexes)
	req.Equal(expectedData, actualData)

	req.NoError(db.Close())
}

func dumpTables(t *testing.T, db *sql.DB) ([]string, []tabledata) {
	req := require.New(t)

	var indexes []string
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index'")
	req.NoError(err)
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		req.NoError(err)
		if !strings.Contains(name, "sqlite_autoindex") {
			indexes = append(indexes, name)
		}
	}

	var data []tabledata
	rows, err = db.Query("SELECT name, sql FROM sqlite_master WHERE type = 'table'")
	req.NoError(err)
	for rows.Next() {
		var name, sql string
		err = rows.Scan(&name, &sql)
		req.NoError(err)
		if strings.HasPrefix(name, "sqlite") {
			continue
		}
		sqla := strings.Split(sql, "\n")
		cols := []string{}
		for _, s := range sqla {
			// alter table does funky things to the sql, hence the "," ReplaceAll:
			s = strings.Split(strings.TrimSpace(strings.ReplaceAll(s, ",", "")), " ")[0]
			switch s {
			case "CREATE", "FOREIGN", "", ")":
			default:
				cols = append(cols, s)
			}
		}
		data = append(data, tabledata{name: name, cols: cols})
		rows2, err := db.Query("SELECT * FROM " + name)
		req.NoError(err)
		for rows2.Next() {
			vals := make([]interface{}, len(cols))
			vals2 := make([]interface{}, len(cols))
			for i := range vals {
				vals[i] = &vals2[i]
			}
			err = rows2.Scan(vals...)
			req.NoError(err)
			data[len(data)-1].data = append(data[len(data)-1].data, vals2)
		}
	}
	return indexes, data
}

type tabledata struct {
	name string
	cols []string
	data [][]interface{}
}

package sturdydb

// TODO 2 - build integration tests

import (
	"context"
	"embed"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type schema struct {
	schema string
}

// DefaultSchema is what production systems use.
func defaultSchema() schema {
	return schema{}
}

// ItestNewID see ITestWithID doc
func ITestNewID() string {
	return strconv.Itoa(rand.Intn(99999))
}

// ITestWithID is for starting or joining an integration test.
// Usage: New("", "", "", "", ITestWithID(ITestNewID()), log.Error)
func ITestWithID(id string) schema {
	return schema{schema: "itest_" + id}
}

type DB struct {
	pgx       *pgxpool.Pool
	cfg       *pgxpool.Config
	schema    string
	hostnames []string
	log       func(string)
}

var logger = logging.Logger("sturdydb")

// NewProd is a convenience function for running a production workload.
func NewProd() (*DB, error) {
	return New(defaultSchema(), func(s string) { logger.Error(s) })
}

// New is to be called once per binary to establish the pool.
// log() is for errors. It returns an upgraded database's connection.
// This entry point serves both production and integration tests, so it's more DI.
func New(schema schema, log func(string)) (*DB, error) {
	// TODO read from config: hosts []string, user, password, database string,

	connString := ""
	if len(hosts) > 0 {
		connString = "host=" + hosts[0] + " "
	}
	for k, v := range map[string]string{"user": user, "password": password, "dbname": database} {
		if v != "" {
			connString += k + "=" + v + " "
		}
	}
	if err := ensureSchemaExists(connString, schema.schema); err != nil {
		return nil, err
	}

	cfg, err := pgxpool.ParseConfig(connString + "search_schema=" + schema.schema)
	if err != nil {
		return nil, err
	}

	// enable multiple fallback hosts.
	for _, h := range hosts[1:] {
		cfg.ConnConfig.Fallbacks = append(cfg.ConnConfig.Fallbacks, &pgconn.FallbackConfig{Host: h})
	}

	cfg.ConnConfig.OnNotice = func(conn *pgconn.PgConn, n *pgconn.Notice) {
		log("database notice: " + n.Message + ": " + n.Detail)
		DBMeasures.Errors.M(1)
	}

	db := DB{cfg: cfg, schema: schema.schema, hostnames: hosts, log: log} // pgx populated in AddStatsAndConnect
	if err := db.addStatsAndConnect(); err != nil {
		return nil, err
	}

	return &db, db.upgrade()
}

type tracer struct {
}

type ctxkey string

var sqlStart = ctxkey("sqlStart")

func (t tracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, sqlStart, time.Now())
}
func (t tracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	DBMeasures.Hits.M(1)
	ms := time.Since(ctx.Value(sqlStart).(time.Time)).Milliseconds()
	DBMeasures.TotalWait.M(ms)
	DBMeasures.Waits.Observe(float64(ms))
	if data.Err != nil {
		DBMeasures.Errors.M(1)
	}
	// Can log what type of query it is, but not what tables
	// Can log rows affected.
}

// addStatsAndConnect connects a prometheus logger. Be sure to run this before using the DB.
func (db *DB) addStatsAndConnect() error {

	db.cfg.ConnConfig.Tracer = tracer{}

	hostnameToIndex := map[string]float64{}
	for i, h := range db.hostnames {
		hostnameToIndex[h] = float64(i)
	}
	db.cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		s := db.pgx.Stat()
		DBMeasures.OpenConnections.M(int64(s.TotalConns()))
		DBMeasures.WhichHost.Observe(hostnameToIndex[c.Config().Host])

		// Here's where you can do any connection seasoning
		return nil
	}

	var err error
	db.pgx, err = pgxpool.NewWithConfig(context.Background(), db.cfg)
	if err != nil {
		db.log(fmt.Sprintf("Unable to connect to database: %v\n", err))
		return err
	}
	return nil
}

// ITestDeleteAll will delete everything created for "this" integration test.
// This must be called at the end of each integration test.
func (db *DB) ITestDeleteAll() {
	if !strings.HasPrefix(db.schema, "itest_") {
		fmt.Println("Warning: this should never be called on anything but an itest schema.")
		return
	}
	defer db.pgx.Close()
	_, err := db.pgx.Exec(context.Background(), "DROP SCHEMA ?", db.schema)
	if err != nil {
		fmt.Println("warning: unclean itest shutdown: cannot delete schema: " + err.Error())
		return
	}
}

func ensureSchemaExists(connString, schema string) error {
	// FUTURE allow using fallback DBs for start-up.
	p, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		return err
	}
	defer p.Close(context.Background())

	_, err = p.Exec(context.Background(), "CREATE SCHEMA IF NOT EXISTS ?", schema)
	if err != nil {
		return err
	}
	return nil
}

//go:embed sql
var fs embed.FS

func (db *DB) upgrade() error {
	// Does the version table exist? if not, make it.
	// NOTE: This cannot change except via the next sql file.
	err := db.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS base (
		id INTEGER SERIAL PRIMARY KEY,
		entry CHAR(12),
		applied TIMESTAMP DEFAULT current_timestamp
	)`)
	if err != nil {
		db.log("Upgrade failed.")
		return err
	}

	// __Run scripts in order.__

	landed := map[string]bool{}
	{
		var landedEntries []struct{ Entry string }
		err = db.Select(context.Background(), &landedEntries, "SELECT entry FROM base")
		if err != nil {
			db.log("Cannot read entries: " + err.Error())
			return err
		}
		for _, l := range landedEntries {
			landed[l.Entry] = true
		}
	}
	dir, err := fs.ReadDir(".")
	if err != nil {
		db.log("Cannot read fs entries: " + err.Error())
		return err
	}
	sort.Slice(dir, func(i, j int) bool { return dir[i].Name() < dir[j].Name() })
	for _, e := range dir {
		name := e.Name()
		if landed[name] || !strings.HasSuffix(name, ".sql") {
			continue
		}
		file, err := fs.ReadFile(name)
		if err != nil {
			db.log("weird embed file read err")
			return err
		}
		for _, s := range strings.Split(string(file), ";") { // Implement the changes.
			if len(strings.TrimSpace(s)) == 0 {
				continue
			}
			_, err = db.pgx.Exec(context.Background(), s)
			if err != nil {
				db.log(fmt.Sprintf("Could not upgrade! File %s, Query: %s, Returned: %s", name, s, err.Error()))
				return err
			}
		}

		// Mark Completed.
		err = db.Exec(context.Background(), "INSERT INTO base (entry) VALUE (?)", name)
		if err != nil {
			db.log("Cannot update base: " + err.Error())
			return err
		}
	}
	return nil
}

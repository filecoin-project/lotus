package harmonydb

import (
	"context"
	"embed"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/yugabyte/pgx/v5"
	"github.com/yugabyte/pgx/v5/pgconn"
	"github.com/yugabyte/pgx/v5/pgxpool"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/node/config"
)

type DB struct {
	pgx       *pgxpool.Pool
	cfg       *pgxpool.Config
	schema    string
	hostnames []string
	BTFPOnce  sync.Once
	BTFP      atomic.Uintptr // BeginTransactionFramePointer
}

var logger = logging.Logger("harmonydb")

// NewFromConfig is a convenience function.
// In usage:
//
//	db, err := NewFromConfig(config.HarmonyDB)  // in binary init
func NewFromConfig(cfg config.HarmonyDB) (*DB, error) {
	return New(
		cfg.Hosts,
		cfg.Username,
		cfg.Password,
		cfg.Database,
		cfg.Port,
	)
}

// New is to be called once per binary to establish the pool.
// log() is for errors. It returns an upgraded database's connection.
// This entry point serves both production and integration tests, so it's more DI.
func New(hosts []string, username, password, database, port string) (*DB, error) {
	connString := ""
	if len(hosts) > 0 {
		connString = "host=" + hosts[0] + " "
	}
	for k, v := range map[string]string{"user": username, "password": password, "dbname": database, "port": port} {
		if strings.TrimSpace(v) != "" {
			connString += k + "=" + v + " "
		}
	}

	schema := "curio"

	if err := ensureSchemaExists(connString, schema); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(connString + "search_path=" + schema)
	if err != nil {
		return nil, err
	}

	// enable multiple fallback hosts.
	for _, h := range hosts[1:] {
		cfg.ConnConfig.Fallbacks = append(cfg.ConnConfig.Fallbacks, &pgconn.FallbackConfig{Host: h})
	}

	cfg.ConnConfig.OnNotice = func(conn *pgconn.PgConn, n *pgconn.Notice) {
		logger.Debug("database notice: " + n.Message + ": " + n.Detail)
	}

	db := DB{cfg: cfg, schema: schema, hostnames: hosts} // pgx populated in AddStatsAndConnect
	if err := db.addStatsAndConnect(); err != nil {
		return nil, err
	}

	return &db, db.upgrade()
}

func (db *DB) GetRoutableIP() (string, error) {
	tx, err := db.pgx.Begin(context.Background())
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	local := tx.Conn().PgConn().Conn().LocalAddr()
	addr, ok := local.(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("could not get local addr from %v", addr)
	}
	return addr.IP.String(), nil
}

// addStatsAndConnect connects a prometheus logger. Be sure to run this before using the DB.
func (db *DB) addStatsAndConnect() error {

	hostnameToIndex := map[string]float64{}
	for i, h := range db.hostnames {
		hostnameToIndex[h] = float64(i)
	}

	// Timeout the first connection so we know if the DB is down.
	ctx, ctxClose := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer ctxClose()
	var err error
	db.pgx, err = pgxpool.NewWithConfig(ctx, db.cfg)
	if err != nil {
		logger.Error(fmt.Sprintf("Unable to connect to database: %v\n", err))
		return err
	}
	return nil
}

var schemaREString = "^[A-Za-z0-9_]+$"
var schemaRE = regexp.MustCompile(schemaREString)

func ensureSchemaExists(connString, schema string) error {
	// FUTURE allow using fallback DBs for start-up.
	ctx, cncl := context.WithDeadline(context.Background(), time.Now().Add(3*time.Second))
	p, err := pgx.Connect(ctx, connString)
	defer cncl()
	if err != nil {
		return xerrors.Errorf("unable to connect to db: %s, err: %v", connString, err)
	}
	defer func() { _ = p.Close(context.Background()) }()

	if len(schema) < 5 || !schemaRE.MatchString(schema) {
		return xerrors.New("schema must be of the form " + schemaREString + "\n Got: " + schema)
	}
	_, err = p.Exec(context.Background(), "CREATE SCHEMA IF NOT EXISTS "+schema)
	if err != nil {
		return xerrors.Errorf("cannot create schema: %w", err)
	}
	return nil
}

//go:embed sql
var fs embed.FS

func (db *DB) upgrade() error {
	// Does the version table exist? if not, make it.
	// NOTE: This cannot change except via the next sql file.
	_, err := db.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS base (
		id SERIAL PRIMARY KEY,
		entry CHAR(12),
		applied TIMESTAMP DEFAULT current_timestamp
	)`)
	if err != nil {
		logger.Error("Upgrade failed.")
		return xerrors.Errorf("Cannot create base table %w", err)
	}

	// __Run scripts in order.__

	landed := map[string]bool{}
	{
		var landedEntries []struct{ Entry string }
		err = db.Select(context.Background(), &landedEntries, "SELECT entry FROM base")
		if err != nil {
			logger.Error("Cannot read entries: " + err.Error())
			return xerrors.Errorf("cannot read entries: %w", err)
		}
		for _, l := range landedEntries {
			landed[l.Entry[:8]] = true
		}
	}
	dir, err := fs.ReadDir("sql")
	if err != nil {
		logger.Error("Cannot read fs entries: " + err.Error())
		return err
	}
	sort.Slice(dir, func(i, j int) bool { return dir[i].Name() < dir[j].Name() })

	if len(dir) == 0 {
		logger.Error("No sql files found.")
	}
	for _, e := range dir {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			logger.Debug("Must have only SQL files here, found: " + name)
			continue
		}
		if landed[name[:8]] {
			logger.Debug("DB Schema " + name + " already applied.")
			continue
		}
		file, err := fs.ReadFile("sql/" + name)
		if err != nil {
			logger.Error("weird embed file read err")
			return err
		}

		logger.Infow("Upgrading", "file", name, "size", len(file))

		for _, s := range parseSQLStatements(string(file)) { // Implement the changes.
			if len(strings.TrimSpace(s)) == 0 {
				continue
			}
			_, err = db.pgx.Exec(context.Background(), s)
			if err != nil {
				msg := fmt.Sprintf("Could not upgrade! File %s, Query: %s, Returned: %s", name, s, err.Error())
				logger.Error(msg)
				return xerrors.New(msg) // makes devs lives easier by placing message at the end.
			}
		}

		// Mark Completed.
		_, err = db.Exec(context.Background(), "INSERT INTO base (entry) VALUES ($1)", name[:8])
		if err != nil {
			logger.Error("Cannot update base: " + err.Error())
			return xerrors.Errorf("cannot insert into base: %w", err)
		}
	}
	return nil
}

func parseSQLStatements(sqlContent string) []string {
	var statements []string
	var currentStatement strings.Builder

	lines := strings.Split(sqlContent, "\n")
	var inFunction bool

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "--") {
			// Skip empty lines and comments.
			continue
		}

		// Detect function blocks starting or ending.
		if strings.Contains(trimmedLine, "$$") {
			inFunction = !inFunction
		}

		// Add the line to the current statement.
		currentStatement.WriteString(line + "\n")

		// If we're not in a function and the line ends with a semicolon, or we just closed a function block.
		if (!inFunction && strings.HasSuffix(trimmedLine, ";")) || (strings.Contains(trimmedLine, "$$") && !inFunction) {
			statements = append(statements, currentStatement.String())
			currentStatement.Reset()
		}
	}

	// Add any remaining statement not followed by a semicolon (should not happen in well-formed SQL but just in case).
	if currentStatement.Len() > 0 {
		statements = append(statements, currentStatement.String())
	}

	return statements
}

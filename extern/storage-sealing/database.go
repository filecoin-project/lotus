package sealing

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type Response struct {
	status string
}

func connectDataBase() (*sql.DB) {
	db, err := sql.Open("sqlite3", "sectors.db")
	if err != nil {
		log.Errorf("database connect failed: ", err)
		return nil
	}
	return db
}

func (db *sql.DB)initDatabase() {
	// 创建数据表
	sql_table := `
CREATE TABLE IF NOT EXISTS "sectors" (
	"sector_number"  INTEGER PRIMARY KEY AUTOINCREMENT,
	"state"          VARCHAR NULL,
	"commd"          VARCHAR,
	"commr"          VARCHAR,
	"proof"          VARCHAR,
	"precommit_msg"  VARCHAR,
	"commit_msg"     VARCHAR,
	"expiration"     INTEGER
)
CREATE TABLE IF NOT EXISTS "sector_log" (
	"id"            INTEGER,
	"kind"          VARCHAR,
	"timestamp"     TIMESTAMP,
	"trace"         VARCHAR,
	"message"       VARCHAR,
	"sector_number" INTEGER,
	CONSTRAINT      fk_sectors
	FOREIGN KEY (sector_number)
	REFERNENCES sectors(sector_number)
)
CREATE TABLE IF NOT EXISTS "sector_seed" (
	"id"    INTEGER,
	"value" VARCHAR,
	"epoch" INTEGER,
	"sector_number" INTEGER,
	CONSTRAINT      fk_sectors
	FOREIGN KEY (sector_number)
	REFERNENCES sectors(sector_number)
)
CREATE TABLE IF NOT EXISTS "sector_ticket" (
	"id"    INTEGER,
	"value" VARCHAR,
	"epoch" INTEGER,
	"sector_number" INTEGER,
	CONSTRAINT      fk_sectors
	FOREIGN KEY (sector_number)
	REFERNENCES sectors(sector_number)
)
	`
}

func (*sql.DB)createAndUpdate(state *SectorInfo) (Response, err) {

	return {}, nil
}
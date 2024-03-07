package filter_test

import (
	"database/sql"
	"path"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

const V2_DUMP = `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE event (
                id INTEGER PRIMARY KEY,
                height INTEGER NOT NULL,
                tipset_key BLOB NOT NULL,
                tipset_key_cid BLOB NOT NULL,
                emitter_addr BLOB NOT NULL,
                event_index INTEGER NOT NULL,
                message_cid BLOB NOT NULL,
                message_index INTEGER NOT NULL,
                reverted INTEGER NOT NULL
        );
INSERT INTO event VALUES(1,20,X'0171a0e40220440557380ab22c576118b11040c301ceb52e3712cd46b93c021576af3cc3f8bd',X'0171a0e402205eb0d6f509d97c2bc765a13189cd1a4dd2fbf9023563a4e7ecb183e5e2ae3602',X'040a555a21c730eb2ae1159120bd780e15f2c634ed24',0,X'0171a0e4022023b08253a1d39f0306ad0a905ef8553489ff5a94e82562f395da2e3d8b01eb85',0,0);
INSERT INTO event VALUES(2,25,X'0171a0e40220beeff892e675374f22d51bf5c32afe6f4f6d2e68a8812a81dad9d8f0c239e516',X'0171a0e4022088f2d6e01dd9c0d138c2626319639e388fbe284d96e162ad7329e15b3a1898c1',X'040a555a21c730eb2ae1159120bd780e15f2c634ed24',0,X'0171a0e40220f7bf7aa42b9e3a6c17d237968d6c62b3ba5bc8c8cd3fac239c5dd39a3d4ab40e',0,0);
INSERT INTO event VALUES(3,30,X'0171a0e40220cdd19768ce4e471561e78605bdfeb82e2e693604bed401596db3fd00d946bdf1',X'0171a0e40220c042a4795aa1f1e9dfa6e2cff8a5081ccd90b04e7636d269f421a0509c0edd78',X'040a555a21c730eb2ae1159120bd780e15f2c634ed24',0,X'0171a0e40220d79bb23187fcced749ed15065865989057d3ba5cc815fbb6ce7a98ea778d17e5',0,0);
CREATE TABLE event_entry (
                event_id INTEGER,
                indexed INTEGER NOT NULL,
                flags BLOB NOT NULL,
                key TEXT NOT NULL,
                codec INTEGER,
                value BLOB NOT NULL
        );
INSERT INTO event_entry VALUES(1,1,X'03','d',85,X'1122334455667788');
INSERT INTO event_entry VALUES(3,1,X'03','t1',85,X'0000000000000000000000000000000000000000000000000000000000001111');
INSERT INTO event_entry VALUES(3,1,X'03','t2',85,X'0000000000000000000000000000000000000000000000000000000000002222');
INSERT INTO event_entry VALUES(3,1,X'03','t3',85,X'0000000000000000000000000000000000000000000000000000000000003333');
INSERT INTO event_entry VALUES(3,1,X'03','t4',85,X'0000000000000000000000000000000000000000000000000000000000004444');
INSERT INTO event_entry VALUES(3,1,X'03','d',85,X'1122334455667788');
CREATE TABLE _meta (
        version UINT64 NOT NULL UNIQUE
        );
INSERT INTO _meta VALUES(1);
INSERT INTO _meta VALUES(2);
CREATE INDEX height_tipset_key ON event (height,tipset_key);
COMMIT;
`

func TestV2ToV3Migration(t *testing.T) {
	req := require.New(t)
	dir := t.TempDir()

	// Connect to SQLite DB
	db, err := sql.Open("sqlite3", path.Join(dir, "new_database.db"))
	req.NoError(err)
	defer db.Close()

	// Execute the commands from the SQLite dump
	commands := strings.Split(V2_DUMP, ";")
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command != "" {
			_, err := db.Exec(command)
			req.NoError(err)
		}
	}

	t.Log("Database created and populated successfully.")
}

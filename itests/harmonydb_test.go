package itests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/impl"
)

func withSetup(t *testing.T, f func(*kit.TestMiner)) {
	_, miner, _ := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs(),
		kit.WithSectorIndexDB(),
	)

	f(miner)
}

func TestCrud(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	withSetup(t, func(miner *kit.TestMiner) {
		cdb := miner.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		_, err := cdb.Exec(ctx, `
			INSERT INTO 
				itest_scratch (some_int, content) 
			VALUES 
				(11, 'cows'), 
				(5, 'cats')
		`)
		if err != nil {
			t.Fatal("Could not insert: ", err)
		}
		var ints []struct {
			Count       int    `db:"some_int"`
			Animal      string `db:"content"`
			Unpopulated int
		}
		err = cdb.Select(ctx, &ints, "SELECT content, some_int FROM itest_scratch")
		if err != nil {
			t.Fatal("Could not select: ", err)
		}
		if len(ints) != 2 {
			t.Fatal("unexpected count of returns. Want 2, Got ", len(ints))
		}
		if ints[0].Count != 11 || ints[1].Count != 5 {
			t.Fatal("expected [11,5] got ", ints)
		}
		if ints[0].Animal != "cows" || ints[1].Animal != "cats" {
			t.Fatal("expected, [cows, cats] ", ints)
		}
		fmt.Println("test completed")
	})
}

func TestTransaction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	withSetup(t, func(miner *kit.TestMiner) {
		cdb := miner.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		if _, err := cdb.Exec(ctx, "INSERT INTO itest_scratch (some_int) VALUES (4), (5), (6)"); err != nil {
			t.Fatal("E0", err)
		}
		_, err := cdb.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
			if _, err := tx.Exec("INSERT INTO itest_scratch (some_int) VALUES (7), (8), (9)"); err != nil {
				t.Fatal("E1", err)
			}

			// sum1 is read from OUTSIDE the transaction so it's the old value
			var sum1 int
			if err := cdb.QueryRow(ctx, "SELECT SUM(some_int) FROM itest_scratch").Scan(&sum1); err != nil {
				t.Fatal("E2", err)
			}
			if sum1 != 4+5+6 {
				t.Fatal("Expected 15, got ", sum1)
			}

			// sum2 is from INSIDE the transaction, so the updated value.
			var sum2 int
			if err := tx.QueryRow("SELECT SUM(some_int) FROM itest_scratch").Scan(&sum2); err != nil {
				t.Fatal("E3", err)
			}
			if sum2 != 4+5+6+7+8+9 {
				t.Fatal("Expected 39, got ", sum2)
			}
			return false, nil // rollback
		})
		if err != nil {
			t.Fatal("ET", err)
		}

		var sum2 int
		// Query() example (yes, QueryRow would be preferred here)
		q, err := cdb.Query(ctx, "SELECT SUM(some_int) FROM itest_scratch")
		if err != nil {
			t.Fatal("E4", err)
		}
		defer q.Close()
		var rowCt int
		for q.Next() {
			err := q.Scan(&sum2)
			if err != nil {
				t.Fatal("error scanning ", err)
			}
			rowCt++
		}
		if sum2 != 4+5+6 {
			t.Fatal("Expected 15, got ", sum2)
		}
		if rowCt != 1 {
			t.Fatal("unexpected count of rows")
		}
	})
}

func TestPartialWalk(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	withSetup(t, func(miner *kit.TestMiner) {
		cdb := miner.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		if _, err := cdb.Exec(ctx, `
			INSERT INTO 
				itest_scratch (content, some_int) 
			VALUES 
				('andy was here', 5), 
				('lotus is awesome', 6), 
				('hello world', 7),
				('3rd integration test', 8),
				('fiddlesticks', 9)
			`); err != nil {
			t.Fatal("e1", err)
		}

		// TASK: FIND THE ID of the string with a specific SHA256
		needle := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
		q, err := cdb.Query(ctx, `SELECT id, content FROM itest_scratch`)
		if err != nil {
			t.Fatal("e2", err)
		}
		defer q.Close()

		var tmp struct {
			Src string `db:"content"`
			ID  int
		}

		var done bool
		for q.Next() {

			if err := q.StructScan(&tmp); err != nil {
				t.Fatal("structscan err " + err.Error())
			}

			bSha := sha256.Sum256([]byte(tmp.Src))
			if hex.EncodeToString(bSha[:]) == needle {
				done = true
				break
			}
		}
		if !done {
			t.Fatal("We didn't find it.")
		}
		// Answer: tmp.ID
	})
}

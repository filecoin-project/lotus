package ethhashlookup

import (
	"context"
	"os"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sqlite"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
)

var log = logging.Logger("txhashindex")

// ChainStore interface; we could use store.ChainStore directly,
// but this simplifies unit testing.
type ChainStore interface {
	SubscribeHeadChanges(f store.ReorgNotifee)
	GetSecpkMessagesForTipset(ctx context.Context, ts *types.TipSet) ([]*types.SignedMessage, error)
	GetHeaviestTipSet() *types.TipSet
	GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
}

var _ ChainStore = (*store.ChainStore)(nil)

// PopulateAfterSnapshot populates the tx hash index after a snapshot is restored.
func PopulateAfterSnapshot(ctx context.Context, path string, cs ChainStore) error {
	// if a database already exists, we try to delete it and create a new one
	if _, err := os.Stat(path); err == nil {
		if err = os.Remove(path); err != nil {
			return xerrors.Errorf("tx hash index already exists at %s and can't be deleted", path)
		}
	}

	db, _, err := sqlite.Open(path)
	if err != nil {
		return xerrors.Errorf("failed to setup tx hash index db: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Errorf("error closing tx hash database: %s", err)
		}
	}()

	if err = sqlite.InitDb(ctx, "message index", db, ddls, []sqlite.MigrationFunc{}); err != nil {
		_ = db.Close()
		return xerrors.Errorf("error creating tx hash index database: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return xerrors.Errorf("error when starting transaction: %w", err)
	}

	rollback := func() {
		if err := tx.Rollback(); err != nil {
			log.Errorf("error in rollback: %s", err)
		}
	}

	insertStmt, err := tx.Prepare(insertTxHash)
	if err != nil {
		rollback()
		return xerrors.Errorf("error preparing insertStmt: %w", err)
	}

	defer func() {
		_ = insertStmt.Close()
	}()

	var (
		curTs       = cs.GetHeaviestTipSet()
		startHeight = curTs.Height()
		msgs        []*types.SignedMessage
	)

	for curTs != nil {
		msgs, err = cs.GetSecpkMessagesForTipset(ctx, curTs)
		if err != nil {
			log.Infof("stopping import after %d tipsets", startHeight-curTs.Height())
			break
		}

		for _, m := range msgs {
			ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(m)
			if err != nil {
				rollback()
				return err
			}

			txHash, err := ethTx.TxHash()
			if err != nil {
				rollback()
				return err
			}

			_, err = insertStmt.Exec(txHash.String(), m.Cid().String())
			if err != nil {
				rollback()
				return err
			}
		}

		curTs, err = cs.GetTipSetFromKey(ctx, curTs.Parents())
		if err != nil {
			rollback()
			return xerrors.Errorf("error walking chain: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("error committing transaction: %w", err)
	}

	return nil
}

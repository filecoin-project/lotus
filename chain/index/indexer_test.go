package index

import (
	"context"
	"database/sql"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestRestoreTipsetIfExists(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	si, _, _ := setupWithHeadIndexed(t, 10, rng)

	tsKeyCid := randomCid(t, rng)
	tsKeyCidBytes := tsKeyCid.Bytes()

	err := withTx(ctx, si.db, func(tx *sql.Tx) error {
		// tipset does not exist
		exists, err := si.restoreTipsetIfExists(ctx, tx, tsKeyCidBytes)
		require.NoError(t, err)
		require.False(t, exists)

		// insert reverted tipset
		_, err = tx.Stmt(si.stmts.insertTipsetMessageStmt).Exec(tsKeyCidBytes, 1, 1, randomCid(t, rng).Bytes(), 0)
		require.NoError(t, err)

		// tipset exists and is NOT reverted
		exists, err = si.restoreTipsetIfExists(ctx, tx, tsKeyCidBytes)
		require.NoError(t, err)
		require.True(t, exists)

		// Verify that the tipset is not reverted
		var reverted bool
		err = tx.QueryRow("SELECT reverted FROM tipset_message WHERE tipset_key_cid = ?", tsKeyCidBytes).Scan(&reverted)
		require.NoError(t, err)
		require.False(t, reverted, "Tipset should not be reverted")

		return nil
	})
	require.NoError(t, err)

	exists, err := si.isTipsetIndexed(ctx, tsKeyCidBytes)
	require.NoError(t, err)
	require.True(t, exists)

	fc := randomCid(t, rng)
	exists, err = si.isTipsetIndexed(ctx, fc.Bytes())
	require.NoError(t, err)
	require.False(t, exists)
}

func TestWritersWaitForWriterLock(t *testing.T) {
	ctx := context.Background()

	from, err := ethtypes.EthAddress{1}.ToFilecoinAddress()
	require.NoError(t, err)
	to, err := address.NewIDAddress(1)
	require.NoError(t, err)
	sig := make([]byte, 65)
	sig[0] = 1
	delegated := &types.SignedMessage{
		Message: types.Message{
			From:       from,
			To:         to,
			Value:      big.Zero(),
			GasFeeCap:  big.NewInt(1),
			GasPremium: big.NewInt(1),
			GasLimit:   1,
			Method:     builtintypes.MethodsEVM.InvokeContract,
		},
		Signature: crypto.Signature{Type: crypto.SigTypeDelegated, Data: sig},
	}
	ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(delegated)
	require.NoError(t, err)
	delegatedHash, err := ethTx.TxHash()
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		write func(si *SqliteIndexer, head *types.TipSet, msgCid cid.Cid) error
		check func(t *testing.T, si *SqliteIndexer, msgCid cid.Cid)
	}{{
		name: "IndexEthTxHash",
		write: func(si *SqliteIndexer, _ *types.TipSet, msgCid cid.Cid) error {
			return si.IndexEthTxHash(ctx, ethtypes.EthHash{1}, msgCid)
		},
		check: func(t *testing.T, si *SqliteIndexer, msgCid cid.Cid) {
			got, err := si.GetCidFromHash(ctx, ethtypes.EthHash{1})
			require.NoError(t, err)
			require.Equal(t, msgCid, got)
		},
	}, {
		name: "IndexSignedMessage",
		write: func(si *SqliteIndexer, _ *types.TipSet, _ cid.Cid) error {
			return si.IndexSignedMessage(ctx, delegated)
		},
		check: func(t *testing.T, si *SqliteIndexer, _ cid.Cid) {
			got, err := si.GetCidFromHash(ctx, delegatedHash)
			require.NoError(t, err)
			require.Equal(t, delegated.Cid(), got)
		},
	}, {
		name: "ReconcileWithChain",
		write: func(si *SqliteIndexer, head *types.TipSet, _ cid.Cid) error {
			return si.ReconcileWithChain(ctx, head)
		},
		check: func(*testing.T, *SqliteIndexer, cid.Cid) {},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			rng := pseudo.New(pseudo.NewSource(14030))
			si, head, _ := setupWithHeadIndexed(t, 10, rng)
			t.Cleanup(func() { _ = si.Close() })
			msgCid := randomCid(t, rng)

			si.writerLk.Lock()
			done := make(chan error, 1)
			go func() { done <- tc.write(si, head, msgCid) }()
			select {
			case err := <-done:
				t.Fatalf("%s completed while writerLk was held: %v", tc.name, err)
			case <-time.After(200 * time.Millisecond):
			}
			si.writerLk.Unlock()
			require.NoError(t, <-done)
			tc.check(t, si, msgCid)
		})
	}
}

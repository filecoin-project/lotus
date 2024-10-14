package index

import (
	"context"
	"database/sql"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

package index

import (
	"context"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGC(t *testing.T) {
	ctx := context.Background()
	genesisTime := time.Now()
	rng := pseudo.New(pseudo.NewSource(genesisTime.UnixNano()))

	// head at height 60
	// insert tipsets at heigh 1,10,50.
	// retention epochs is 20
	// tipset at height 60 will be in the future
	si, _, _ := setupWithHeadIndexed(t, 60, rng)
	si.gcRetentionEpochs = 20
	defer func() { _ = si.Close() }()

	// all tipsets will be in future
	tsKeyCid1, _, _ := insertRandomTipsetAtHeight(t, si, 1, false, genesisTime)
	tsKeyCid2, _, _ := insertRandomTipsetAtHeight(t, si, 10, false, genesisTime)
	tsKeyCid3, _, _ := insertRandomTipsetAtHeight(t, si, 50, false, genesisTime)

	si.gc(ctx)

	// tipset at height 1 data should be removed
	var count int
	err := si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = si.stmts.getNonRevertedTipsetEventCountStmt.QueryRow(tsKeyCid1.Bytes()).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = si.stmts.getNonRevertedTipsetEventEntriesCountStmt.QueryRow(tsKeyCid1.Bytes()).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// tipset at height 10 data should be removed
	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 10").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = si.stmts.getNonRevertedTipsetEventCountStmt.QueryRow(tsKeyCid2.Bytes()).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = si.stmts.getNonRevertedTipsetEventEntriesCountStmt.QueryRow(tsKeyCid2.Bytes()).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// tipset at height 50 should not be removed
	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 50").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	err = si.stmts.getNonRevertedTipsetEventCountStmt.QueryRow(tsKeyCid3.Bytes()).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	err = si.stmts.getNonRevertedTipsetEventEntriesCountStmt.QueryRow(tsKeyCid3.Bytes()).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	err = si.db.QueryRow("SELECT COUNT(*) FROM eth_tx_hash").Scan(&count)
	require.NoError(t, err)
	// eth_tx_hash for tipset at height 50 timestamp should not be removed
	require.Equal(t, 1, count)
}

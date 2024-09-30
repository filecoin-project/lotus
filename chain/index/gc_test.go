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
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))

	// head at height 60
	// insert tipsets at heigh 1,10,50.
	// retention epochs is 20
	si, _, _ := setupWithHeadIndexed(t, 60, rng)
	si.gcRetentionEpochs = 20
	defer func() { _ = si.Close() }()

	tsCid1 := randomCid(t, rng)
	tsCid10 := randomCid(t, rng)
	tsCid50 := randomCid(t, rng)

	insertTipsetMessage(t, si, tipsetMessage{
		tipsetKeyCid: tsCid1.Bytes(),
		height:       1,
		reverted:     false,
		messageCid:   randomCid(t, rng).Bytes(),
		messageIndex: 0,
	})

	insertTipsetMessage(t, si, tipsetMessage{
		tipsetKeyCid: tsCid10.Bytes(),
		height:       10,
		reverted:     false,
		messageCid:   randomCid(t, rng).Bytes(),
		messageIndex: 0,
	})

	insertTipsetMessage(t, si, tipsetMessage{
		tipsetKeyCid: tsCid50.Bytes(),
		height:       50,
		reverted:     false,
		messageCid:   randomCid(t, rng).Bytes(),
		messageIndex: 0,
	})

	si.gc(ctx)

	// tipset at height 1 and 10 should be removed
	var count int
	err := si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 10").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// tipset at height 50 should not be removed
	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 50").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

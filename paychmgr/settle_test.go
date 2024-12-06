package paychmgr

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	tutils "github.com/filecoin-project/specs-actors/support/testing"
)

func TestPaychSettle(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	expch := tutils.NewIDAddr(t, 100)
	expch2 := tutils.NewIDAddr(t, 101)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	amt := big.NewInt(10)
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
	require.NoError(t, err)

	// Send channel create response
	response := testChannelResponse(t, expch)
	mock.receiveMsgResponse(mcid, response)

	// Get the channel address
	ch, err := mgr.GetPaychWaitReady(ctx, mcid)
	require.NoError(t, err)
	require.Equal(t, expch, ch)

	// Settle the channel
	_, err = mgr.Settle(ctx, ch)
	require.NoError(t, err)

	// Send another request for funds to the same from/to
	// (should create a new channel because the previous channel
	// is settling)
	amt2 := big.NewInt(5)
	_, mcid2, err := mgr.GetPaych(ctx, from, to, amt2, onChainReserve)
	require.NoError(t, err)
	require.NotEqual(t, cid.Undef, mcid2)

	// Send new channel create response
	response2 := testChannelResponse(t, expch2)
	mock.receiveMsgResponse(mcid2, response2)

	// Make sure the new channel is different from the old channel
	ch2, err := mgr.GetPaychWaitReady(ctx, mcid2)
	require.NoError(t, err)
	require.NotEqual(t, ch, ch2)

	// There should now be two channels
	cis, err := mgr.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 2)
}

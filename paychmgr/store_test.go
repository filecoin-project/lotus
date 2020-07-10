package paychmgr

import (
	"testing"

	"github.com/filecoin-project/go-address"

	tutils "github.com/filecoin-project/specs-actors/support/testing"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	addrs, err := store.ListChannels()
	require.NoError(t, err)
	require.Len(t, addrs, 0)

	ci := &ChannelInfo{
		Channel: tutils.NewIDAddr(t, 100),
		Control: tutils.NewIDAddr(t, 101),
		Target:  tutils.NewIDAddr(t, 102),

		Direction: DirOutbound,
		Vouchers:  []*VoucherInfo{{Voucher: nil, Proof: []byte{}}},
	}

	ci2 := &ChannelInfo{
		Channel: tutils.NewIDAddr(t, 200),
		Control: tutils.NewIDAddr(t, 201),
		Target:  tutils.NewIDAddr(t, 202),

		Direction: DirOutbound,
		Vouchers:  []*VoucherInfo{{Voucher: nil, Proof: []byte{}}},
	}

	// Track the channel
	err = store.TrackChannel(ci)
	require.NoError(t, err)

	// Tracking same channel again should error
	err = store.TrackChannel(ci)
	require.Error(t, err)

	// Track another channel
	err = store.TrackChannel(ci2)
	require.NoError(t, err)

	// List channels should include all channels
	addrs, err = store.ListChannels()
	require.NoError(t, err)
	require.Len(t, addrs, 2)
	require.Contains(t, addrsStrings(addrs), "t0100")
	require.Contains(t, addrsStrings(addrs), "t0200")

	// Request vouchers for channel
	vouchers, err := store.VouchersForPaych(ci.Channel)
	require.NoError(t, err)
	require.Len(t, vouchers, 1)

	// Requesting voucher for non-existent channel should error
	_, err = store.VouchersForPaych(tutils.NewIDAddr(t, 300))
	require.Equal(t, err, ErrChannelNotTracked)

	// Allocate lane for channel
	lane, err := store.AllocateLane(ci.Channel)
	require.NoError(t, err)
	require.Equal(t, lane, uint64(0))

	// Allocate next lane for channel
	lane, err = store.AllocateLane(ci.Channel)
	require.NoError(t, err)
	require.Equal(t, lane, uint64(1))

	// Allocate next lane for non-existent channel should error
	_, err = store.AllocateLane(tutils.NewIDAddr(t, 300))
	require.Equal(t, err, ErrChannelNotTracked)
}

func addrsStrings(addrs []address.Address) []string {
	str := make([]string, len(addrs))
	for i, a := range addrs {
		str[i] = a.String()
	}
	return str
}

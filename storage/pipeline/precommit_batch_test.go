package sealing

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/storage/pipeline/mocks"
	"github.com/filecoin-project/lotus/storage/pipeline/sealiface"
)

// A sector carrying deal IDs is refused from nv29 as it is added, so it never reaches b.todo. An
// entry the network will not accept must not sit in the batch, where it fails every later attempt
// and never clears itself.
func TestAddPreCommitDealIDs(t *testing.T) {
	maddr, err := address.NewIDAddress(123)
	require.NoError(t, err)

	const sectorNumber = abi.SectorNumber(42)
	const height = abi.ChainEpoch(100)

	for _, tc := range []struct {
		name    string
		nv      network.Version
		dealIDs []abi.DealID
		refused bool
	}{
		{"nv28 accepts deal IDs", network.Version28, []abi.DealID{7}, false},
		{"nv29 refuses deal IDs", network.Version29, []abi.DealID{7}, true},
		{"nv29 accepts none", network.Version29, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			api := mocks.NewMockPreCommitBatcherApi(ctrl)

			ts := makeTestTipSet(t, height)
			api.EXPECT().ChainHead(gomock.Any()).Return(ts, nil)
			api.EXPECT().StateNetworkVersion(gomock.Any(), ts.Key()).Return(tc.nv, nil)

			b := &PreCommitBatcher{
				api:     api,
				maddr:   maddr,
				mctx:    context.Background(),
				cutoffs: map[abi.SectorNumber]time.Time{},
				todo:    map[abi.SectorNumber]*preCommitEntry{},
				waiting: map[abi.SectorNumber][]chan sealiface.PreCommitBatchRes{},
				notify:  make(chan struct{}, 1),
			}

			// An accepted sector waits for a batch that never runs, so cancel out of the wait and
			// read b.todo instead.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := b.AddPreCommit(ctx,
				SectorInfo{SectorNumber: sectorNumber},
				big.Zero(),
				&miner.SectorPreCommitInfo{SectorNumber: sectorNumber, DealIDs: tc.dealIDs},
			)

			if tc.refused {
				require.ErrorContains(t, err, "deal_ids")
				require.NotContains(t, b.todo, sectorNumber, "a refused sector must not be left in the batch")
				require.NotContains(t, b.waiting, sectorNumber, "a refused sector must not be left waiting")
				return
			}

			require.ErrorIs(t, err, context.Canceled)
			require.Contains(t, b.todo, sectorNumber)
		})
	}
}

package sealing_test

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	miner6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/miner"

	"github.com/filecoin-project/go-state-types/network"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/extern/storage-sealing/mocks"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/node/config"
	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

var fc = config.MinerFeeConfig{
	MaxPreCommitGasFee:      types.FIL(types.FromFil(1)),
	MaxCommitGasFee:         types.FIL(types.FromFil(1)),
	MaxTerminateGasFee:      types.FIL(types.FromFil(1)),
	MaxPreCommitBatchGasFee: config.BatchFeeConfig{Base: types.FIL(types.FromFil(3)), PerSector: types.FIL(types.FromFil(1))},
	MaxCommitBatchGasFee:    config.BatchFeeConfig{Base: types.FIL(types.FromFil(3)), PerSector: types.FIL(types.FromFil(1))},
}

func TestPrecommitBatcher(t *testing.T) {
	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	ctx := context.Background()

	as := func(ctx context.Context, mi miner.MinerInfo, use api.AddrUse, goodFunds, minFunds abi.TokenAmount) (address.Address, abi.TokenAmount, error) {
		return t0123, big.Zero(), nil
	}

	maxBatch := miner6.PreCommitSectorBatchMaxSize

	cfg := func() (sealiface.Config, error) {
		return sealiface.Config{
			MaxWaitDealsSectors:       2,
			MaxSealingSectors:         0,
			MaxSealingSectorsForDeals: 0,
			WaitDealsDelay:            time.Hour * 6,
			AlwaysKeepUnsealedCopy:    true,

			BatchPreCommits:            true,
			MaxPreCommitBatch:          maxBatch,
			PreCommitBatchWait:         24 * time.Hour,
			PreCommitBatchSlack:        3 * time.Hour,
			BatchPreCommitAboveBaseFee: big.NewInt(10000),

			AggregateCommits: true,
			MinCommitBatch:   miner6.MinAggregatedSectors,
			MaxCommitBatch:   miner6.MaxAggregatedSectors,
			CommitBatchWait:  24 * time.Hour,
			CommitBatchSlack: 1 * time.Hour,

			TerminateBatchMin:  1,
			TerminateBatchMax:  100,
			TerminateBatchWait: 5 * time.Minute,
		}, nil
	}

	type promise func(t *testing.T)
	type action func(t *testing.T, s *mocks.MockPreCommitBatcherApi, pcb *sealing.PreCommitBatcher) promise

	actions := func(as ...action) action {
		return func(t *testing.T, s *mocks.MockPreCommitBatcherApi, pcb *sealing.PreCommitBatcher) promise {
			var ps []promise
			for _, a := range as {
				p := a(t, s, pcb)
				if p != nil {
					ps = append(ps, p)
				}
			}

			if len(ps) > 0 {
				return func(t *testing.T) {
					for _, p := range ps {
						p(t)
					}
				}
			}
			return nil
		}
	}

	addSector := func(sn abi.SectorNumber) action {
		return func(t *testing.T, s *mocks.MockPreCommitBatcherApi, pcb *sealing.PreCommitBatcher) promise {
			var pcres sealiface.PreCommitBatchRes
			var pcerr error
			done := sync.Mutex{}
			done.Lock()

			si := sealing.SectorInfo{
				SectorNumber: sn,
			}

			s.EXPECT().ChainHead(gomock.Any()).Return(nil, abi.ChainEpoch(1), nil)

			go func() {
				defer done.Unlock()
				pcres, pcerr = pcb.AddPreCommit(ctx, si, big.Zero(), &miner0.SectorPreCommitInfo{
					SectorNumber: si.SectorNumber,
					SealedCID:    fakePieceCid(t),
					DealIDs:      nil,
					Expiration:   0,
				})
			}()

			return func(t *testing.T) {
				done.Lock()
				require.NoError(t, pcerr)
				require.Empty(t, pcres.Error)
				require.Contains(t, pcres.Sectors, si.SectorNumber)
			}
		}
	}

	addSectors := func(sectors []abi.SectorNumber) action {
		as := make([]action, len(sectors))
		for i, sector := range sectors {
			as[i] = addSector(sector)
		}
		return actions(as...)
	}

	waitPending := func(n int) action {
		return func(t *testing.T, s *mocks.MockPreCommitBatcherApi, pcb *sealing.PreCommitBatcher) promise {
			require.Eventually(t, func() bool {
				p, err := pcb.Pending(ctx)
				require.NoError(t, err)
				return len(p) == n
			}, time.Second*5, 10*time.Millisecond)

			return nil
		}
	}

	expectSend := func(expect []abi.SectorNumber) action {
		return func(t *testing.T, s *mocks.MockPreCommitBatcherApi, pcb *sealing.PreCommitBatcher) promise {
			s.EXPECT().ChainHead(gomock.Any()).Return(nil, abi.ChainEpoch(1), nil)
			s.EXPECT().ChainBaseFee(gomock.Any(), gomock.Any()).Return(big.NewInt(10001), nil)
			s.EXPECT().StateNetworkVersion(gomock.Any(), gomock.Any()).Return(network.Version14, nil)

			s.EXPECT().StateMinerInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(miner.MinerInfo{Owner: t0123, Worker: t0123}, nil)
			s.EXPECT().SendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), funMatcher(func(i interface{}) bool {
				b := i.([]byte)
				var params miner6.PreCommitSectorBatchParams
				require.NoError(t, params.UnmarshalCBOR(bytes.NewReader(b)))
				for s, number := range expect {
					require.Equal(t, number, params.Sectors[s].SectorNumber)
				}
				return true
			}))
			return nil
		}
	}

	expectSendsSingle := func(expect []abi.SectorNumber) action {
		return func(t *testing.T, s *mocks.MockPreCommitBatcherApi, pcb *sealing.PreCommitBatcher) promise {
			s.EXPECT().ChainHead(gomock.Any()).Return(nil, abi.ChainEpoch(1), nil)
			s.EXPECT().ChainBaseFee(gomock.Any(), gomock.Any()).Return(big.NewInt(9999), nil)
			s.EXPECT().StateNetworkVersion(gomock.Any(), gomock.Any()).Return(network.Version14, nil)

			s.EXPECT().StateMinerInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(miner.MinerInfo{Owner: t0123, Worker: t0123}, nil)
			for _, number := range expect {
				numClone := number
				s.EXPECT().SendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), funMatcher(func(i interface{}) bool {
					b := i.([]byte)
					var params miner6.PreCommitSectorParams
					require.NoError(t, params.UnmarshalCBOR(bytes.NewReader(b)))
					require.Equal(t, numClone, params.SectorNumber)
					return true
				}))
			}
			return nil
		}
	}

	flush := func(expect []abi.SectorNumber) action {
		return func(t *testing.T, s *mocks.MockPreCommitBatcherApi, pcb *sealing.PreCommitBatcher) promise {
			_ = expectSend(expect)(t, s, pcb)

			r, err := pcb.Flush(ctx)
			require.NoError(t, err)
			require.Len(t, r, 1)
			require.Empty(t, r[0].Error)
			sort.Slice(r[0].Sectors, func(i, j int) bool {
				return r[0].Sectors[i] < r[0].Sectors[j]
			})
			require.Equal(t, expect, r[0].Sectors)

			return nil
		}
	}

	getSectors := func(n int) []abi.SectorNumber {
		out := make([]abi.SectorNumber, n)
		for i := range out {
			out[i] = abi.SectorNumber(i)
		}
		return out
	}

	tcs := map[string]struct {
		actions []action
	}{
		"addSingle": {
			actions: []action{
				addSector(0),
				waitPending(1),
				flush([]abi.SectorNumber{0}),
			},
		},
		"addTwo": {
			actions: []action{
				addSectors(getSectors(2)),
				waitPending(2),
				flush(getSectors(2)),
			},
		},
		"addMax": {
			actions: []action{
				expectSend(getSectors(maxBatch)),
				addSectors(getSectors(maxBatch)),
			},
		},
		"addMax-belowBaseFee": {
			actions: []action{
				expectSendsSingle(getSectors(maxBatch)),
				addSectors(getSectors(maxBatch)),
			},
		},
	}

	for name, tc := range tcs {
		tc := tc

		t.Run(name, func(t *testing.T) {
			// create go mock controller here
			mockCtrl := gomock.NewController(t)
			// when test is done, assert expectations on all mock objects.
			defer mockCtrl.Finish()

			// create them mocks
			pcapi := mocks.NewMockPreCommitBatcherApi(mockCtrl)

			pcb := sealing.NewPreCommitBatcher(ctx, t0123, pcapi, as, fc, cfg)

			var promises []promise

			for _, a := range tc.actions {
				p := a(t, pcapi, pcb)
				if p != nil {
					promises = append(promises, p)
				}
			}

			for _, p := range promises {
				p(t)
			}

			err := pcb.Stop(ctx)
			require.NoError(t, err)
		})
	}
}

type funMatcher func(interface{}) bool

func (funMatcher) Matches(interface{}) bool {
	return true
}

func (funMatcher) String() string {
	return "fun"
}

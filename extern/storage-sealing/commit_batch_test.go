package sealing_test

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/extern/storage-sealing/mocks"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
)

func TestCommitBatcher(t *testing.T) {
	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	ctx := context.Background()

	as := func(ctx context.Context, mi miner.MinerInfo, use api.AddrUse, goodFunds, minFunds abi.TokenAmount) (address.Address, abi.TokenAmount, error) {
		return t0123, big.Zero(), nil
	}

	maxBatch := miner5.MaxAggregatedSectors
	minBatch := miner5.MinAggregatedSectors

	cfg := func() (sealiface.Config, error) {
		return sealiface.Config{
			MaxWaitDealsSectors:       2,
			MaxSealingSectors:         0,
			MaxSealingSectorsForDeals: 0,
			WaitDealsDelay:            time.Hour * 6,
			AlwaysKeepUnsealedCopy:    true,

			BatchPreCommits:     true,
			MaxPreCommitBatch:   miner5.PreCommitSectorBatchMaxSize,
			PreCommitBatchWait:  24 * time.Hour,
			PreCommitBatchSlack: 3 * time.Hour,

			AggregateCommits: true,
			MinCommitBatch:   minBatch,
			MaxCommitBatch:   maxBatch,
			CommitBatchWait:  24 * time.Hour,
			CommitBatchSlack: 1 * time.Hour,

			AggregateAboveBaseFee:      types.BigMul(types.PicoFil, types.NewInt(150)), // 0.15 nFIL
			BatchPreCommitAboveBaseFee: types.BigMul(types.PicoFil, types.NewInt(150)), // 0.15 nFIL

			TerminateBatchMin:  1,
			TerminateBatchMax:  100,
			TerminateBatchWait: 5 * time.Minute,
		}, nil
	}

	type promise func(t *testing.T)
	type action func(t *testing.T, s *mocks.MockCommitBatcherApi, pcb *sealing.CommitBatcher) promise

	actions := func(as ...action) action {
		return func(t *testing.T, s *mocks.MockCommitBatcherApi, pcb *sealing.CommitBatcher) promise {
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
		return func(t *testing.T, s *mocks.MockCommitBatcherApi, pcb *sealing.CommitBatcher) promise {
			var pcres sealiface.CommitBatchRes
			var pcerr error
			done := sync.Mutex{}
			done.Lock()

			si := sealing.SectorInfo{
				SectorNumber: sn,
			}

			s.EXPECT().ChainHead(gomock.Any()).Return(nil, abi.ChainEpoch(1), nil)
			s.EXPECT().StateNetworkVersion(gomock.Any(), gomock.Any()).Return(network.Version13, nil)
			s.EXPECT().StateSectorPreCommitInfo(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&miner.SectorPreCommitOnChainInfo{
				PreCommitDeposit: big.Zero(),
			}, nil)

			go func() {
				defer done.Unlock()
				pcres, pcerr = pcb.AddCommit(ctx, si, sealing.AggregateInput{
					Info: proof5.AggregateSealVerifyInfo{
						Number: sn,
					},
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
		return func(t *testing.T, s *mocks.MockCommitBatcherApi, pcb *sealing.CommitBatcher) promise {
			require.Eventually(t, func() bool {
				p, err := pcb.Pending(ctx)
				require.NoError(t, err)
				return len(p) == n
			}, time.Second*5, 10*time.Millisecond)

			return nil
		}
	}

	expectSend := func(expect []abi.SectorNumber, aboveBalancer, failOnePCI bool) action {
		return func(t *testing.T, s *mocks.MockCommitBatcherApi, pcb *sealing.CommitBatcher) promise {
			s.EXPECT().StateMinerInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(miner.MinerInfo{Owner: t0123, Worker: t0123}, nil)

			ti := len(expect)
			batch := false
			if ti >= minBatch {
				batch = true
				ti = 1
			}

			basefee := types.PicoFil
			if aboveBalancer {
				basefee = types.NanoFil
			}

			if batch {
				s.EXPECT().ChainHead(gomock.Any()).Return(nil, abi.ChainEpoch(1), nil)
				s.EXPECT().ChainBaseFee(gomock.Any(), gomock.Any()).Return(basefee, nil)
			}

			if !aboveBalancer {
				batch = false
				ti = len(expect)
			}

			s.EXPECT().ChainHead(gomock.Any()).Return(nil, abi.ChainEpoch(1), nil)

			pciC := len(expect)
			if failOnePCI {
				s.EXPECT().StateSectorPreCommitInfo(gomock.Any(), gomock.Any(), abi.SectorNumber(1), gomock.Any()).Return(nil, nil).Times(1) // not found
				pciC = len(expect) - 1
				if !batch {
					ti--
				}
			}
			s.EXPECT().StateSectorPreCommitInfo(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&miner.SectorPreCommitOnChainInfo{
				PreCommitDeposit: big.Zero(),
			}, nil).Times(pciC)
			s.EXPECT().StateMinerInitialPledgeCollateral(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(big.Zero(), nil).Times(pciC)

			if batch {
				s.EXPECT().StateNetworkVersion(gomock.Any(), gomock.Any()).Return(network.Version13, nil)
				s.EXPECT().ChainBaseFee(gomock.Any(), gomock.Any()).Return(basefee, nil)
			}

			s.EXPECT().SendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), funMatcher(func(i interface{}) bool {
				b := i.([]byte)
				if batch {
					var params miner5.ProveCommitAggregateParams
					require.NoError(t, params.UnmarshalCBOR(bytes.NewReader(b)))
					for _, number := range expect {
						set, err := params.SectorNumbers.IsSet(uint64(number))
						require.NoError(t, err)
						require.True(t, set)
					}
				} else {
					var params miner5.ProveCommitSectorParams
					require.NoError(t, params.UnmarshalCBOR(bytes.NewReader(b)))
				}
				return true
			})).Times(ti)
			return nil
		}
	}

	flush := func(expect []abi.SectorNumber, aboveBalancer, failOnePCI bool) action {
		return func(t *testing.T, s *mocks.MockCommitBatcherApi, pcb *sealing.CommitBatcher) promise {
			_ = expectSend(expect, aboveBalancer, failOnePCI)(t, s, pcb)

			batch := len(expect) >= minBatch && aboveBalancer

			r, err := pcb.Flush(ctx)
			require.NoError(t, err)
			if batch {
				require.Len(t, r, 1)
				require.Empty(t, r[0].Error)
				sort.Slice(r[0].Sectors, func(i, j int) bool {
					return r[0].Sectors[i] < r[0].Sectors[j]
				})
				require.Equal(t, expect, r[0].Sectors)
				if !failOnePCI {
					require.Len(t, r[0].FailedSectors, 0)
				} else {
					require.Len(t, r[0].FailedSectors, 1)
					_, found := r[0].FailedSectors[1]
					require.True(t, found)
				}
			} else {
				require.Len(t, r, len(expect))
				for _, res := range r {
					require.Len(t, res.Sectors, 1)
					require.Empty(t, res.Error)
				}
				sort.Slice(r, func(i, j int) bool {
					return r[i].Sectors[0] < r[j].Sectors[0]
				})
				for i, res := range r {
					require.Equal(t, abi.SectorNumber(i), res.Sectors[0])
					if failOnePCI && res.Sectors[0] == 1 {
						require.Len(t, res.FailedSectors, 1)
						_, found := res.FailedSectors[1]
						require.True(t, found)
					} else {
						require.Empty(t, res.FailedSectors)
					}
				}
			}

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
		"addSingle-aboveBalancer": {
			actions: []action{
				addSector(0),
				waitPending(1),
				flush([]abi.SectorNumber{0}, true, false),
			},
		},
		"addTwo-aboveBalancer": {
			actions: []action{
				addSectors(getSectors(2)),
				waitPending(2),
				flush(getSectors(2), true, false),
			},
		},
		"addAte-aboveBalancer": {
			actions: []action{
				addSectors(getSectors(8)),
				waitPending(8),
				flush(getSectors(8), true, false),
			},
		},
		"addMax-aboveBalancer": {
			actions: []action{
				expectSend(getSectors(maxBatch), true, false),
				addSectors(getSectors(maxBatch)),
			},
		},
		"addSingle-belowBalancer": {
			actions: []action{
				addSector(0),
				waitPending(1),
				flush([]abi.SectorNumber{0}, false, false),
			},
		},
		"addTwo-belowBalancer": {
			actions: []action{
				addSectors(getSectors(2)),
				waitPending(2),
				flush(getSectors(2), false, false),
			},
		},
		"addAte-belowBalancer": {
			actions: []action{
				addSectors(getSectors(8)),
				waitPending(8),
				flush(getSectors(8), false, false),
			},
		},
		"addMax-belowBalancer": {
			actions: []action{
				expectSend(getSectors(maxBatch), false, false),
				addSectors(getSectors(maxBatch)),
			},
		},

		"addAte-aboveBalancer-failOne": {
			actions: []action{
				addSectors(getSectors(8)),
				waitPending(8),
				flush(getSectors(8), true, true),
			},
		},
		"addAte-belowBalancer-failOne": {
			actions: []action{
				addSectors(getSectors(8)),
				waitPending(8),
				flush(getSectors(8), false, true),
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
			pcapi := mocks.NewMockCommitBatcherApi(mockCtrl)

			pcb := sealing.NewCommitBatcher(ctx, t0123, pcapi, as, fc, cfg, &fakeProver{})

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

type fakeProver struct{}

func (f fakeProver) AggregateSealProofs(aggregateInfo proof5.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return []byte("Trust me, I'm a proof"), nil
}

var _ ffiwrapper.Prover = &fakeProver{}

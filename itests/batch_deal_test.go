package itests

import (
	"context"
	"fmt"
	"github.com/filecoin-project/lotus/chain/types"
	logging "github.com/ipfs/go-log/v2"
	"sort"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func TestBatchDealInput(t *testing.T) {
	kit.QuietMiningLogs()
	_ = logging.SetLogLevel("sectors", "DEBUG")

	const (
		blockTime = 10 * time.Millisecond

		// For these tests where the block time is artificially short, just use
		// a deal start epoch that is guaranteed to be far enough in the future
		// so that the deal starts sealing in time
		dealStartEpoch = abi.ChainEpoch(2 << 12)

		earlyStartEpoch = 3500
	)

	type runType int
	const (
		happy runType = iota
		dealExpirePC
		dealExpireC
	)

	run := func(piece, deals, expectSectors int, rt runType) func(t *testing.T) {
		return func(t *testing.T) {
			ctx := context.Background()

			publishPeriod := 10 * time.Second
			maxDealsPerMsg := uint64(deals)

			tenIfNonHappyTest := uint64(10)
			if rt == happy {
				tenIfNonHappyTest = 0
			}

			// Set max deals per publish deals message to maxDealsPerMsg
			opts := kit.ConstructorOpts(node.Options(
				node.Override(
					new(*storageadapter.DealPublisher),
					storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
						Period:         publishPeriod,
						MaxDealsPerMsg: maxDealsPerMsg,
					})),
				node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
					return func() (sealiface.Config, error) {
						return sealiface.Config{
							MaxWaitDealsSectors:       2 + tenIfNonHappyTest,
							MaxSealingSectors:         1 + tenIfNonHappyTest,
							MaxSealingSectorsForDeals: 3,
							AlwaysKeepUnsealedCopy:    true,
							WaitDealsDelay:            time.Hour,

							BatchPreCommits:  true,
							AggregateCommits: true,

							MaxCommitBatch:    expectSectors + int(tenIfNonHappyTest),
							MaxPreCommitBatch: expectSectors + int(tenIfNonHappyTest),

							PreCommitBatchWait: time.Hour,
							CommitBatchWait:    time.Hour,
						}, nil
					}, nil
				}),
			), kit.LatestActorsAt(-1))
			client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), opts)
			ens.InterconnectAll().BeginMining(blockTime)
			dh := kit.NewDealHarness(t, client, miner, miner)

			err := miner.MarketSetAsk(ctx, big.Zero(), big.Zero(), 200, 128, 32<<30)
			require.NoError(t, err)

			checkNoPadding := func() {
				hts, err := client.ChainHead(ctx)
				require.NoError(t, err)
				t.Logf("HH: %d\n", hts.Height())

				sl, err := miner.SectorsList(ctx)
				require.NoError(t, err)

				sort.Slice(sl, func(i, j int) bool {
					return sl[i] < sl[j]
				})

				for _, snum := range sl {
					si, err := miner.SectorsStatus(ctx, snum, false)
					require.NoError(t, err)

					t.Logf("S %d: %+v %s\n", snum, si.Deals, si.State)

					for _, deal := range si.Deals {
						if deal == 0 {
							fmt.Printf("sector %d had a padding piece!\n", snum)
						}
					}
				}
			}

			// Run maxDealsPerMsg deals in parallel
			deals := make([]*cid.Cid, maxDealsPerMsg)

			done := make(chan struct {
				i int
				d *cid.Cid
			}, maxDealsPerMsg)
			for rseed := 0; rseed < int(maxDealsPerMsg); rseed++ {
				rseed := rseed
				go func() {
					startEpoch := dealStartEpoch
					if rt != happy && rseed == 0 {
						startEpoch = earlyStartEpoch
					}

					res, _, _, err := kit.CreateImportFile(ctx, client, rseed, piece)
					require.NoError(t, err)

					done <- struct {
						i int
						d *cid.Cid
					}{rseed, dh.StartDeal(ctx, res.Root, false, startEpoch)}
				}()
			}

			// Wait for maxDealsPerMsg of the deals to be published
			for i := 0; i < int(maxDealsPerMsg); i++ {
				di := <-done
				deals[di.i] = di.d
			}

			t.Run("start-deals", func(t *testing.T) {})

			switch rt {
			case happy:
				dh.WaitDealStates(ctx, deals, dh.DealState(storagemarket.StorageDealActive))
				checkNoPadding()
			case dealExpirePC, dealExpireC:
				// first wait for the sector to get into a batcher
				switch rt {
				case dealExpirePC:
					dh.WaitDealStates(ctx, deals,
						dh.DealState(storagemarket.StorageDealAwaitingPreCommit),
						dh.DealSectorState(sealing.SubmitPreCommitBatch))
				case dealExpireC:
					dh.WaitDealStates(ctx, deals,
						dh.DealState(storagemarket.StorageDealAwaitingPreCommit),
						dh.DealSectorState(sealing.SubmitPreCommitBatch))

					fb, err := miner.SectorPreCommitFlush(ctx)
					require.NoError(t, err)
					require.Equal(t, len(fb), 1)
					require.Equal(t, len(fb[0].Sectors), expectSectors)

					dh.WaitDealStates(ctx, deals,
						dh.DealState(storagemarket.StorageDealSealing),
						dh.DealSectorState(sealing.SubmitCommitAggregate))
				}

				t.Run("wait-batch", func(t *testing.T) {})

				cd, err := client.ClientGetDealInfo(ctx, *deals[0])
				require.NoError(t, err)

				d, err := client.StateMarketStorageDeal(ctx, cd.DealID, types.EmptyTSK)
				require.NoError(t, err)

				// wait for deal start epoch
				client.WaitTillChain(ctx, kit.HeightAtLeast(d.Proposal.StartEpoch+10))

				t.Run("wait-expire", func(t *testing.T) {})

				// check that sectors are still in aggregate submit states
				switch rt {
				case dealExpirePC:
					dh.WaitDealStates(ctx, deals,
						dh.DealSectorState(sealing.SubmitPreCommitBatch))
				case dealExpireC:
					dh.WaitDealStates(ctx, deals,
						dh.DealSectorState(sealing.SubmitCommitAggregate))
				}

				t.Run("check-still-batch", func(t *testing.T) {})

				switch rt {
				case dealExpirePC:
					fb, err := miner.SectorPreCommitFlush(ctx)
					require.NoError(t, err)
					require.Equal(t, len(fb), 1)
					require.Equal(t, len(fb[0].Sectors), expectSectors)

					// this batch will fail with an expired deal, so wait a bit and retry

					t.Run("submit-pc-batch-1", func(t *testing.T) {})

					time.Sleep(2 * time.Second)

					fb, err = miner.SectorPreCommitFlush(ctx)
					require.NoError(t, err)
					require.Equal(t, len(fb), 1)
					require.Equal(t, len(fb[0].Sectors), expectSectors) // todo the sector with the expired deal will be removed, so expect 1 here (we're getting zero for whatever reason)

					t.Run("submit-pc-batch-2", func(t *testing.T) {})

					dh.WaitDealStates(ctx, deals,
						dh.DealSectorState(sealing.SubmitCommitAggregate))

					t.Run("begin-wait-c-batch", func(t *testing.T) {})
				}

				fb, err := miner.SectorCommitFlush(ctx)
				require.NoError(t, err)
				require.Equal(t, len(fb), 1)
				require.Equal(t, len(fb[0].Sectors), expectSectors)

				t.Run("submit-c-batch", func(t *testing.T) {})

				dh.WaitDealStates(ctx, deals,
					dh.DealSectorState(sealing.Proving))

				t.Run("proving", func(t *testing.T) {})
			}

			sl, err := miner.SectorsList(ctx)
			require.NoError(t, err)
			require.Equal(t, len(sl), expectSectors)
		}
	}

	t.Run("4-p1600B", run(1600, 4, 4, happy))
	t.Run("4-p513B", run(513, 4, 2, happy))

	t.Run("4-p513B-pc-dealexpire", run(513, 4, 2, dealExpirePC))
	t.Run("4-p513B-c-dealexpire", run(513, 4, 2, dealExpireC))

	if !testing.Short() {
		t.Run("32-p257B", run(257, 32, 8, happy))
		t.Run("32-p10B", run(10, 32, 2, happy))

		// fixme: this appears to break data-transfer / markets in some really creative ways
		// t.Run("128-p10B", run(10, 128, 8))
	}
}

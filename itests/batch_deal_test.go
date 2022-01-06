package itests

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/stretchr/testify/require"
)

func TestBatchDealInput(t *testing.T) {
	t.Skip("this test is disabled as it's flaky: #4611")
	kit.QuietMiningLogs()

	var (
		blockTime = 10 * time.Millisecond

		// For these tests where the block time is artificially short, just use
		// a deal start epoch that is guaranteed to be far enough in the future
		// so that the deal starts sealing in time
		dealStartEpoch = abi.ChainEpoch(2 << 12)
	)

	run := func(piece, deals, expectSectors int) func(t *testing.T) {
		return func(t *testing.T) {
			ctx := context.Background()

			publishPeriod := 10 * time.Second
			maxDealsPerMsg := uint64(deals)

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
							MaxWaitDealsSectors:       2,
							MaxSealingSectors:         1,
							MaxSealingSectorsForDeals: 3,
							AlwaysKeepUnsealedCopy:    true,
							WaitDealsDelay:            time.Hour,
						}, nil
					}, nil
				}),
			))
			client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), opts)
			ens.InterconnectAll().BeginMining(blockTime)
			dh := kit.NewDealHarness(t, client, miner, miner)

			err := miner.MarketSetAsk(ctx, big.Zero(), big.Zero(), 200, 128, 32<<30)
			require.NoError(t, err)

			checkNoPadding := func() {
				sl, err := miner.SectorsList(ctx)
				require.NoError(t, err)

				sort.Slice(sl, func(i, j int) bool {
					return sl[i] < sl[j]
				})

				for _, snum := range sl {
					si, err := miner.SectorsStatus(ctx, snum, false)
					require.NoError(t, err)

					// fmt.Printf("S %d: %+v %s\n", snum, si.Deals, si.State)

					for _, deal := range si.Deals {
						if deal == 0 {
							fmt.Printf("sector %d had a padding piece!\n", snum)
						}
					}
				}
			}

			// Starts a deal and waits until it's published
			runDealTillSeal := func(rseed int) {
				res, _, _, err := kit.CreateImportFile(ctx, client, rseed, piece)
				require.NoError(t, err)

				dp := dh.DefaultStartDealParams()
				dp.Data.Root = res.Root
				dp.DealStartEpoch = dealStartEpoch

				deal := dh.StartDeal(ctx, dp)
				dh.WaitDealSealed(ctx, deal, false, true, checkNoPadding)
			}

			// Run maxDealsPerMsg deals in parallel
			done := make(chan struct{}, maxDealsPerMsg)
			for rseed := 0; rseed < int(maxDealsPerMsg); rseed++ {
				rseed := rseed
				go func() {
					runDealTillSeal(rseed)
					done <- struct{}{}
				}()
			}

			// Wait for maxDealsPerMsg of the deals to be published
			for i := 0; i < int(maxDealsPerMsg); i++ {
				<-done
			}

			checkNoPadding()

			sl, err := miner.SectorsList(ctx)
			require.NoError(t, err)
			require.Equal(t, len(sl), expectSectors)
		}
	}

	t.Run("4-p1600B", run(1600, 4, 4))
	t.Run("4-p513B", run(513, 4, 2))
	if !testing.Short() {
		t.Run("32-p257B", run(257, 32, 8))
		t.Run("32-p10B", run(10, 32, 2))

		// fixme: this appears to break data-transfer / markets in some really creative ways
		// t.Run("128-p10B", run(10, 128, 8))
	}
}

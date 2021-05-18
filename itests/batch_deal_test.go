package itests

import (
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/stretchr/testify/require"
)

func TestBatchDealInput(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blockTime = 10 * time.Millisecond

		// For these tests where the block time is artificially short, just use
		// a deal start epoch that is guaranteed to be far enough in the future
		// so that the deal starts sealing in time
		dealStartEpoch = abi.ChainEpoch(2 << 12)

		publishPeriod  = 10 * time.Second
		maxDealsPerMsg = uint64(4)
	)

	// Set max deals per publish deals message to maxDealsPerMsg
	minerDef := []kit.StorageMiner{{
		Full: 0,
		Opts: node.Options(
			node.Override(
				new(*storageadapter.DealPublisher),
				storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
					Period:         publishPeriod,
					MaxDealsPerMsg: maxDealsPerMsg,
				})),
			node.Override(new(dtypes.GetSealingConfigFunc), func() (dtypes.GetSealingConfigFunc, error) {
				return func() (sealiface.Config, error) {
					return sealiface.Config{
						MaxWaitDealsSectors:       1,
						MaxSealingSectors:         1,
						MaxSealingSectorsForDeals: 2,
						AlwaysKeepUnsealedCopy:    true,
					}, nil
				}, nil
			}),
		),
		Preseal: kit.PresealGenesis,
	}}

	// Create a connect client and miner node
	n, sn := kit.MockSbBuilder(t, kit.OneFull, minerDef)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]
	s := kit.ConnectAndStartMining(t, blockTime, client, miner)
	defer s.BlockMiner.Stop()

	// Starts a deal and waits until it's published
	runDealTillSeal := func(rseed int) {
		res, _, err := kit.CreateClientFile(s.Ctx, s.Client, rseed)
		require.NoError(t, err)

		dc := kit.StartDeal(t, s.Ctx, s.Miner, s.Client, res.Root, false, dealStartEpoch)
		kit.WaitDealSealed(t, s.Ctx, s.Miner, s.Client, dc, false)
	}

	// Run maxDealsPerMsg+1 deals in parallel
	done := make(chan struct{}, maxDealsPerMsg+1)
	for rseed := 1; rseed <= int(maxDealsPerMsg+1); rseed++ {
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

	sl, err := sn[0].SectorsList(s.Ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(sl), 4)
	require.LessOrEqual(t, len(sl), 5)
}

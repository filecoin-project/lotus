package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/storage"
	"github.com/stretchr/testify/require"
)

var (
	publishPeriod  = 1 * time.Second
	maxDealsPerMsg = uint64(2) // Set max deals per publish deals message to 2

	blockTime = 3 * time.Millisecond
)

func TestDealsRetryLackOfFunds(t *testing.T) {
	ctx := context.Background()
	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)

	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg8MiBV1)
	kit.QuietMiningLogs()

	// Allow 8MB sectors
	eightMBSectorsOpt := kit.SectorSize(8 << 20)

	publishStorageDealKey, err := wallet.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	opts := node.Options(
		node.Override(new(*storageadapter.DealPublisher),
			storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
				Period:         publishPeriod,
				MaxDealsPerMsg: maxDealsPerMsg,
			}),
		),
		node.Override(new(*storage.AddressSelector), modules.AddressSelector(&config.MinerAddressConfig{
			DealPublishControl: []string{
				publishStorageDealKey.Address.String(),
			},
			DisableOwnerFallback:  true,
			DisableWorkerFallback: true,
		})),
	)

	minerFullNode, clientFullNode, miner, ens := kit.EnsembleTwoOne(t, kit.Account(publishStorageDealKey, types.NewInt(1020000000000)), kit.ConstructorOpts(opts), kit.MockProofs(), eightMBSectorsOpt)

	//TODO: this fails slightly differently - handle this case as well
	//minerFullNode, clientFullNode, miner, ens := kit.EnsembleTwoOne(t, kit.Account(publishStorageDealKey, types.NewInt(1)), kit.ConstructorOpts(opts), kit.MockProofs(), eightMBSectorsOpt)

	kit.QuietMiningLogs()

	ens.
		Start().
		InterconnectAll().
		BeginMining(blockTime)

	_, err = minerFullNode.WalletImport(ctx, &publishStorageDealKey.KeyInfo)
	require.NoError(t, err)

	miner.SetControlAddresses(publishStorageDealKey.Address)

	dh := kit.NewDealHarness(t, clientFullNode, miner, miner)

	res, _ := clientFullNode.CreateImportFile(ctx, 0, 4<<20) // 4MiB file.
	list, err := clientFullNode.ClientListImports(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, res.Root, *list[0].Root)

	dp := dh.DefaultStartDealParams()
	dp.Data.Root = res.Root
	dp.FastRetrieval = true
	dp.EpochPrice = abi.NewTokenAmount(62500000) // minimum asking price.
	deal := dh.StartDeal(ctx, dp)

	propcid := *deal

	go func() {
		time.Sleep(3 * time.Second)

		kit.SendFunds(ctx, t, minerFullNode, publishStorageDealKey.Address, types.FromFil(1))

		err := miner.MarketRetryPublishDeal(ctx, propcid)
		if err != nil {
			panic(err)
		}
	}()

	dh.WaitDealSealed(ctx, deal, false, false, nil)
}

// stm: #integration
package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/storage/ctladdr"
)

var (
	publishPeriod  = 1 * time.Second
	maxDealsPerMsg = uint64(2) // Set max deals per publish deals message to 2

	blockTime = 3 * time.Millisecond
)

func TestDealsRetryLackOfFunds(t *testing.T) {
	t.Run("cover-gas", func(t *testing.T) {
		testDealsRetryLackOfFunds(t, types.NewInt(1020000000000))
	})
	t.Run("empty", func(t *testing.T) {
		testDealsRetryLackOfFunds(t, types.NewInt(1))
	})
}

func testDealsRetryLackOfFunds(t *testing.T, publishStorageAccountFunds abi.TokenAmount) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @CLIENT_STORAGE_DEALS_LIST_IMPORTS_001
	ctx := context.Background()

	kit.QuietMiningLogs()

	// Allow 8MB sectors
	eightMBSectorsOpt := kit.SectorSize(8 << 20)

	publishStorageDealKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	opts := node.Options(
		node.Override(new(*storageadapter.DealPublisher),
			storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
				Period:         publishPeriod,
				MaxDealsPerMsg: maxDealsPerMsg,
			}),
		),
		node.Override(new(*ctladdr.AddressSelector), modules.AddressSelector(&config.MinerAddressConfig{
			DealPublishControl: []string{
				publishStorageDealKey.Address.String(),
			},
			DisableOwnerFallback:  true,
			DisableWorkerFallback: true,
		})),
	)

	minerFullNode, clientFullNode, miner, ens := kit.EnsembleTwoOne(t, kit.Account(publishStorageDealKey, publishStorageAccountFunds), kit.ConstructorOpts(opts), kit.MockProofs(), eightMBSectorsOpt)

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
		time.Sleep(30 * time.Second)

		kit.SendFunds(ctx, t, minerFullNode, publishStorageDealKey.Address, types.FromFil(1))

		err := miner.MarketRetryPublishDeal(ctx, propcid)
		if err != nil {
			panic(err)
		}
	}()

	dh.WaitDealSealed(ctx, deal, false, false, nil)
}

func TestDealsRetryLackOfFunds_blockInPublishDeal(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001
	//stm: @CLIENT_STORAGE_DEALS_LIST_IMPORTS_001
	ctx := context.Background()
	kit.QuietMiningLogs()

	// Allow 8MB sectors
	eightMBSectorsOpt := kit.SectorSize(8 << 20)

	publishStorageDealKey, err := key.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	opts := node.Options(
		node.Override(new(*storageadapter.DealPublisher),
			storageadapter.NewDealPublisher(nil, storageadapter.PublishMsgConfig{
				Period:         publishPeriod,
				MaxDealsPerMsg: maxDealsPerMsg,
			}),
		),
		node.Override(new(*ctladdr.AddressSelector), modules.AddressSelector(&config.MinerAddressConfig{
			DealPublishControl: []string{
				publishStorageDealKey.Address.String(),
			},
			DisableOwnerFallback:  true,
			DisableWorkerFallback: true,
		})),
	)

	publishStorageAccountFunds := types.NewInt(1020000000000)
	minerFullNode, clientFullNode, miner, ens := kit.EnsembleTwoOne(t, kit.Account(publishStorageDealKey, publishStorageAccountFunds), kit.ConstructorOpts(opts), kit.MockProofs(), eightMBSectorsOpt)

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

	dealSealed := make(chan struct{})
	go func() {
		dh.WaitDealSealedQuiet(ctx, deal, false, false, nil)
		dealSealed <- struct{}{}
	}()

	select {
	case <-dealSealed:
		t.Fatal("deal shouldn't have sealed")
	case <-time.After(time.Second * 15):
	}
}

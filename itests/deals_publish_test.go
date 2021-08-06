package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/markets/storageadapter"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/storage"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	"github.com/stretchr/testify/require"
)

func TestPublishDealsBatching(t *testing.T) {
	var (
		ctx            = context.Background()
		publishPeriod  = 10 * time.Second
		maxDealsPerMsg = uint64(2) // Set max deals per publish deals message to 2
		startEpoch     = abi.ChainEpoch(2 << 12)
	)

	kit.QuietMiningLogs()

	publisherKey, err := wallet.GenerateKey(types.KTSecp256k1)
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
				publisherKey.Address.String(),
			},
			DisableOwnerFallback:  true,
			DisableWorkerFallback: true,
		})),
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.Account(publisherKey, types.FromFil(10)), kit.MockProofs(), kit.ConstructorOpts(opts))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	_, err = client.WalletImport(ctx, &publisherKey.KeyInfo)
	require.NoError(t, err)

	miner.SetControlAddresses(publisherKey.Address)

	dh := kit.NewDealHarness(t, client, miner, miner)

	// Starts a deal and waits until it's published
	runDealTillPublish := func(rseed int) {
		res, _ := client.CreateImportFile(ctx, rseed, 0)

		upds, err := client.ClientGetDealUpdates(ctx)
		require.NoError(t, err)

		dp := dh.DefaultStartDealParams()
		dp.Data.Root = res.Root
		dp.DealStartEpoch = startEpoch
		dh.StartDeal(ctx, dp)

		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
		time.Sleep(time.Second)

		done := make(chan struct{})
		go func() {
			for upd := range upds {
				if upd.DataRef.Root == res.Root && upd.State == storagemarket.StorageDealAwaitingPreCommit {
					done <- struct{}{}
				}
			}
		}()
		<-done
	}

	// Run three deals in parallel
	done := make(chan struct{}, maxDealsPerMsg+1)
	for rseed := 1; rseed <= 3; rseed++ {
		rseed := rseed
		go func() {
			runDealTillPublish(rseed)
			done <- struct{}{}
		}()
	}

	// Wait for two of the deals to be published
	for i := 0; i < int(maxDealsPerMsg); i++ {
		<-done
	}

	// Expect a single PublishStorageDeals message that includes the first two deals
	msgCids, err := client.StateListMessages(ctx, &api.MessageMatch{To: market.Address}, types.EmptyTSK, 1)
	require.NoError(t, err)
	count := 0
	for _, msgCid := range msgCids {
		msg, err := client.ChainGetMessage(ctx, msgCid)
		require.NoError(t, err)

		if msg.Method == market.Methods.PublishStorageDeals {
			count++
			var pubDealsParams market2.PublishStorageDealsParams
			err = pubDealsParams.UnmarshalCBOR(bytes.NewReader(msg.Params))
			require.NoError(t, err)
			require.Len(t, pubDealsParams.Deals, int(maxDealsPerMsg))
			require.Equal(t, publisherKey.Address.String(), msg.From.String())
		}
	}
	require.Equal(t, 1, count)

	// The third deal should be published once the publish period expires.
	// Allow a little padding as it takes a moment for the state change to
	// be noticed by the client.
	padding := 10 * time.Second
	select {
	case <-time.After(publishPeriod + padding):
		require.Fail(t, "Expected 3rd deal to be published once publish period elapsed")
	case <-done: // Success
	}
}

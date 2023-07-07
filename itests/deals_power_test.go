// stm: #integration
package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestFirstDealEnablesMining(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	// test making a deal with a fresh miner, and see if it starts to mine.
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	kit.QuietMiningLogs()

	var (
		client   kit.TestFullNode
		genMiner kit.TestMiner // bootstrap
		provider kit.TestMiner // no sectors, will need to create one
	)

	ens := kit.NewEnsemble(t, kit.MockProofs())
	ens.FullNode(&client)
	ens.Miner(&genMiner, &client, kit.WithAllSubsystems())
	ens.Miner(&provider, &client, kit.WithAllSubsystems(), kit.PresealSectors(0))
	ens.Start().InterconnectAll().BeginMining(50 * time.Millisecond)

	ctx := context.Background()

	dh := kit.NewDealHarness(t, &client, &provider, &provider)

	ref, _ := client.CreateImportFile(ctx, 5, 0)

	t.Log("FILE CID:", ref.Root)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// start a goroutine to monitor head changes from the client
	// once the provider has mined a block, thanks to the power acquired from the deal,
	// we pass the test.
	providerMined := make(chan struct{})

	go func() {
		_ = client.WaitTillChain(ctx, kit.BlocksMinedByAll(provider.ActorAddr))
		close(providerMined)
	}()

	// now perform the deal.
	dp := dh.DefaultStartDealParams()
	dp.Data.Root = ref.Root
	deal := dh.StartDeal(ctx, dp)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(time.Second)

	dh.WaitDealSealed(ctx, deal, false, false, nil)

	<-providerMined
}

package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestFirstDealEnablesMining(t *testing.T) {
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
		_ = client.WaitTillChain(ctx, kit.BlockMinedBy(provider.ActorAddr))
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

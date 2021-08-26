package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/lotus/itests/kit"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	market3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/market"
	market4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/market"
	market5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/market"
	"github.com/stretchr/testify/require"
)

// Test that the deal state eventually moves to "Expired" on both client and miner
func TestDealExpiry(t *testing.T) {
	kit.QuietMiningLogs()

	// reset minimum deal duration to 0, so we can make very short-lived deals.
	// NOTE: this will need updating with every new specs-actors version.
	market2.DealMinDuration = 0
	market3.DealMinDuration = 0
	market4.DealMinDuration = 0
	market5.DealMinDuration = 0

	ctx := context.Background()

	var (
		client kit.TestFullNode
		miner1 kit.TestMiner
	)

	ens := kit.NewEnsemble(t, kit.MockProofs())
	ens.FullNode(&client)
	ens.Miner(&miner1, &client, kit.WithAllSubsystems())
	bm := ens.Start().InterconnectAll().BeginMining(50 * time.Millisecond)

	dh := kit.NewDealHarness(t, &client, &miner1, &miner1)

	client.WaitTillChain(ctx, kit.HeightAtLeast(5))

	// Make a deal with a short duration
	dealProposalCid, _, _ := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{
		Rseed:   0,
		FastRet: true,
		// Needs to be far enough in the future to ensure the deal has been sealed
		StartEpoch: 3000,
		// Short deal duration
		MinBlocksDuration: 50,
	})

	// Inject null blocks each time the chain advances by a block so as to
	// get to deal expiration faster
	go func() {
		ch, _ := client.ChainNotify(ctx)
		for range ch {
			bm[0].InjectNulls(10)
		}
	}()

	clientExpired := false
	minerExpired := false
	for {
		ts, err := client.ChainHead(ctx)
		require.NoError(t, err)

		t.Logf("Chain height: %d", ts.Height())

		// Get the miner deal
		minerDeals, err := miner1.MarketListIncompleteDeals(ctx)
		require.NoError(t, err)
		require.Greater(t, len(minerDeals), 0)

		var minerDeal *storagemarket.MinerDeal
		for _, d := range minerDeals {
			if d.ProposalCid == *dealProposalCid {
				minerDeal = &d
			}
		}
		require.NotNil(t, minerDeal)

		t.Logf("Miner deal:")
		t.Logf("  %s -> %s", minerDeal.Proposal.Client, minerDeal.Proposal.Provider)
		t.Logf("    StartEpoch: %d", minerDeal.Proposal.StartEpoch)
		t.Logf("    EndEpoch: %d", minerDeal.Proposal.EndEpoch)
		t.Logf("    State: %s", storagemarket.DealStates[minerDeal.State])
		//spew.Dump(d)

		// Get the client deal
		clientDeals, err := client.ClientListDeals(ctx)
		require.NoError(t, err)

		t.Logf("Client deal state: %s", storagemarket.DealStates[clientDeals[0].State])

		// Expect the deal to eventually expire on the client and the miner
		if clientDeals[0].State == storagemarket.StorageDealExpired {
			t.Logf("Client deal expired")
			clientExpired = true
		}
		if minerDeal.State == storagemarket.StorageDealExpired {
			t.Logf("Miner deal expired")
			minerExpired = true
		}
		if clientExpired && minerExpired {
			t.Logf("PASS: Client and miner deal expired")
			return
		}

		if ts.Height() > 5000 {
			t.Fatalf("Reached height %d without client and miner deals expiring", ts.Height())
		}

		time.Sleep(2 * time.Second)
	}
}

package itests

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	market3 "github.com/filecoin-project/specs-actors/v3/actors/builtin/market"
	market4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/market"
	market5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/market"

	"github.com/filecoin-project/lotus/itests/kit"
)

// Test that the deal state eventually moves to "Expired" on both client and miner
func TestDealExpiry(t *testing.T) {
	kit.QuietMiningLogs()

	resetMinDealDuration(t)

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

		// Get the miner deal from the proposal CID
		minerDeal := getMinerDeal(ctx, t, miner1, *dealProposalCid)

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

func getMinerDeal(ctx context.Context, t *testing.T, miner1 kit.TestMiner, dealProposalCid cid.Cid) storagemarket.MinerDeal {
	minerDeals, err := miner1.MarketListIncompleteDeals(ctx)
	require.NoError(t, err)
	require.Greater(t, len(minerDeals), 0)

	for _, d := range minerDeals {
		if d.ProposalCid == dealProposalCid {
			return d
		}
	}
	t.Fatalf("miner deal with proposal CID %s not found", dealProposalCid)
	return storagemarket.MinerDeal{}
}

// reset minimum deal duration to 0, so we can make very short-lived deals.
// NOTE: this will need updating with every new specs-actors version.
func resetMinDealDuration(t *testing.T) {
	m2 := market2.DealMinDuration
	m3 := market3.DealMinDuration
	m4 := market4.DealMinDuration
	m5 := market5.DealMinDuration

	market2.DealMinDuration = 0
	market3.DealMinDuration = 0
	market4.DealMinDuration = 0
	market5.DealMinDuration = 0

	t.Cleanup(func() {
		market2.DealMinDuration = m2
		market3.DealMinDuration = m3
		market4.DealMinDuration = m4
		market5.DealMinDuration = m5
	})
}

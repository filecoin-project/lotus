package itests

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/extern/storage-sealing/sealiface"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// Test that when a miner terminates a sector containing a deal, the deal state
// eventually moves to "Slashed" on both client and miner
func TestDealSlashing(t *testing.T) {
	kit.QuietMiningLogs()
	_ = logging.SetLogLevel("sectors", "debug")

	ctx := context.Background()

	var (
		client kit.TestFullNode
		miner1 kit.TestMiner
	)

	// Set up sealing config so that there is no batching of terminate actions
	var sealingCfgFn dtypes.GetSealingConfigFunc = func() (sealiface.Config, error) {
		return sealiface.Config{
			MaxWaitDealsSectors:       2,
			MaxSealingSectors:         0,
			MaxSealingSectorsForDeals: 0,
			WaitDealsDelay:            time.Second,
			AlwaysKeepUnsealedCopy:    true,

			BatchPreCommits:     true,
			MaxPreCommitBatch:   miner5.PreCommitSectorBatchMaxSize,
			PreCommitBatchWait:  time.Second,
			PreCommitBatchSlack: time.Second,

			AggregateCommits: true,
			MinCommitBatch:   1,
			MaxCommitBatch:   1,
			CommitBatchWait:  time.Second,
			CommitBatchSlack: time.Second,

			AggregateAboveBaseFee: types.BigMul(types.PicoFil, types.NewInt(150)), // 0.15 nFIL

			TerminateBatchMin:  1,
			TerminateBatchMax:  1,
			TerminateBatchWait: time.Second,
		}, nil
	}
	fn := func() dtypes.GetSealingConfigFunc { return sealingCfgFn }
	sealingCfg := kit.ConstructorOpts(node.Override(new(dtypes.GetSealingConfigFunc), fn))

	// Set up a client and miner
	ens := kit.NewEnsemble(t, kit.MockProofs())
	ens.FullNode(&client)
	ens.Miner(&miner1, &client, kit.WithAllSubsystems(), sealingCfg)
	ens.Start().InterconnectAll().BeginMining(50 * time.Millisecond)

	dh := kit.NewDealHarness(t, &client, &miner1, &miner1)

	client.WaitTillChain(ctx, kit.HeightAtLeast(5))

	// Make a storage deal
	dealProposalCid, _, _ := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{
		Rseed:   0,
		FastRet: true,
	})

	// Get the miner deal from the proposal CID
	minerDeal := getDealByProposalCid(ctx, t, miner1, *dealProposalCid)

	// Terminate the sector containing the deal
	t.Logf("Terminating sector %d containing deal %s", minerDeal.SectorNumber, dealProposalCid)
	err := miner1.SectorTerminate(ctx, minerDeal.SectorNumber)
	require.NoError(t, err)

	clientExpired := false
	minerExpired := false
	for {
		ts, err := client.ChainHead(ctx)
		require.NoError(t, err)

		t.Logf("Chain height: %d", ts.Height())

		// Get the miner deal from the proposal CID
		minerDeal := getDealByProposalCid(ctx, t, miner1, *dealProposalCid)
		// Get the miner state from the piece CID
		mktDeal := getMarketDeal(ctx, t, miner1, minerDeal.Proposal.PieceCID)

		t.Logf("Miner deal:")
		t.Logf("  %s -> %s", minerDeal.Proposal.Client, minerDeal.Proposal.Provider)
		t.Logf("    StartEpoch: %d", minerDeal.Proposal.StartEpoch)
		t.Logf("    EndEpoch: %d", minerDeal.Proposal.EndEpoch)
		t.Logf("    SlashEpoch: %d", mktDeal.State.SlashEpoch)
		t.Logf("    LastUpdatedEpoch: %d", mktDeal.State.LastUpdatedEpoch)
		t.Logf("    State: %s", storagemarket.DealStates[minerDeal.State])
		//spew.Dump(d)

		// Get the client deal
		clientDeals, err := client.ClientListDeals(ctx)
		require.NoError(t, err)

		t.Logf("Client deal state: %s\n", storagemarket.DealStates[clientDeals[0].State])

		// Expect the deal to eventually be slashed on the client and the miner
		if clientDeals[0].State == storagemarket.StorageDealSlashed {
			t.Logf("Client deal slashed")
			clientExpired = true
		}
		if minerDeal.State == storagemarket.StorageDealSlashed {
			t.Logf("Miner deal slashed")
			minerExpired = true
		}
		if clientExpired && minerExpired {
			t.Logf("PASS: Client and miner deal slashed")
			return
		}

		if ts.Height() > 4000 {
			t.Fatalf("Reached height %d without client and miner deals being slashed", ts.Height())
		}

		time.Sleep(2 * time.Second)
	}
}

func getMarketDeal(ctx context.Context, t *testing.T, miner1 kit.TestMiner, pieceCid cid.Cid) api.MarketDeal {
	mktDeals, err := miner1.MarketListDeals(ctx)
	require.NoError(t, err)
	require.Greater(t, len(mktDeals), 0)

	for _, d := range mktDeals {
		if d.Proposal.PieceCID == pieceCid {
			return d
		}
	}
	t.Fatalf("miner deal with piece CID %s not found", pieceCid)
	return api.MarketDeal{}
}

func getDealByProposalCid(ctx context.Context, t *testing.T, miner1 kit.TestMiner, dealProposalCid cid.Cid) storagemarket.MinerDeal {
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

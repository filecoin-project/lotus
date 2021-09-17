package itests

import (
	"bufio"
	"context"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipld/go-car"
	"github.com/ipld/go-car/v2/blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-fil-markets/shared"
	"github.com/filecoin-project/go-fil-markets/storagemarket"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func TestDealRetrieveByAnyCid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx := context.Background()
	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	// Allow 8MB sectors
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg8MiBV1)
	kit.QuietMiningLogs()

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	// Override the dependency injection for the blockstore accessor, so that
	// we can get a reference to the blockstore containing our deal later in
	// the test
	var bsa storagemarket.BlockstoreAccessor
	bsaFn := func(importmgr dtypes.ClientImportMgr) storagemarket.BlockstoreAccessor {
		bsa = modules.StorageBlockstoreAccessor(importmgr)
		return bsa
	}
	bsaOpt := kit.ConstructorOpts(node.Override(new(storagemarket.BlockstoreAccessor), bsaFn))

	// Allow 8MB sectors
	eightMBSectorsOpt := kit.SectorSize(8 << 20)

	// Create a client, and a miner with its own full node
	_, client, miner, ens := kit.EnsembleTwoOne(t, kit.MockProofs(), bsaOpt, eightMBSectorsOpt)
	ens.InterconnectAll().BeginMining(250 * time.Millisecond)

	dh := kit.NewDealHarness(t, client, miner, miner)

	// Generate a DAG with multiple levels, so that we can test the case where
	// the client requests a CID for a block which is not the root block but
	// does have a subtree below it in the DAG
	dagOpts := kit.GeneratedDAGOpts{
		// Max size of a block
		ChunkSize: 1024,
		// Max links from a block to other blocks
		Maxlinks: 10,
	}
	carv1FilePath, _ := kit.CreateRandomCARv1(t, 5, 100*1024, dagOpts)
	res, err := client.ClientImport(ctx, api.FileRef{Path: carv1FilePath, IsCAR: true})
	require.NoError(t, err)

	// Get the blockstore for the file
	bs, err := bsa.Get(res.Root)
	require.NoError(t, err)

	// Get all CIDs from the file
	sc := car.NewSelectiveCar(ctx, bs, []car.Dag{{Root: res.Root, Selector: shared.AllSelector()}})
	prepared, err := sc.Prepare()
	require.NoError(t, err)
	cids := prepared.Cids()
	for i, c := range cids {
		blk, err := bs.Get(c)
		require.NoError(t, err)

		nd, err := ipld.Decode(blk)
		require.NoError(t, err)

		t.Log(i, c, len(nd.Links()))
	}

	// Create a storage deal
	dp := dh.DefaultStartDealParams()
	dp.Data.Root = res.Root
	dp.DealStartEpoch = startEpoch
	dp.EpochPrice = abi.NewTokenAmount(62500000) // minimum asking price
	dealCid := dh.StartDeal(ctx, dp)

	// Wait for the deal to be sealed
	dh.WaitDealSealed(ctx, dealCid, false, false, nil)

	ask, err := miner.MarketGetRetrievalAsk(ctx)
	require.NoError(t, err)
	ask.PricePerByte = abi.NewTokenAmount(0)
	ask.UnsealPrice = abi.NewTokenAmount(0)
	err = miner.MarketSetRetrievalAsk(ctx, ask)
	require.NoError(t, err)

	// Fetch the deal data
	info, err := client.ClientGetDealInfo(ctx, *dealCid)
	require.NoError(t, err)

	// Make retrievals against CIDs at different levels in the DAG
	cidIndices := []int{1, 11, 27, 32, 47}
	for _, val := range cidIndices {
		t.Logf("performing retrieval for cid at index %d", val)

		targetCid := cids[val]
		offer, err := client.ClientMinerQueryOffer(ctx, miner.ActorAddr, targetCid, &info.PieceCID)
		require.NoError(t, err)
		require.Empty(t, offer.Err)

		// retrieve in a CAR file and ensure roots match
		outputCar := dh.PerformRetrievalForOffer(ctx, true, offer)
		_, err = os.Stat(outputCar)
		require.NoError(t, err)
		f, err := os.Open(outputCar)
		require.NoError(t, err)
		ch, _, _ := car.ReadHeader(bufio.NewReader(f))
		require.EqualValues(t, ch.Roots[0], targetCid)
		require.NoError(t, f.Close())

		// create CAR from original file starting at targetCid and ensure it matches the retrieved CAR file.
		tmp, err := os.CreateTemp(t.TempDir(), "randcarv1")
		require.NoError(t, err)
		rd, err := blockstore.OpenReadOnly(carv1FilePath, blockstore.UseWholeCIDs(true))
		require.NoError(t, err)
		err = car.NewSelectiveCar(
			ctx,
			rd,
			[]car.Dag{{
				Root:     targetCid,
				Selector: shared.AllSelector(),
			}},
		).Write(tmp)
		require.NoError(t, tmp.Close())
		require.NoError(t, rd.Close())

		kit.AssertFilesEqual(t, tmp.Name(), outputCar)
		t.Log("car files match")
	}
}

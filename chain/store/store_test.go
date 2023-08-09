// stm: #unit
package store_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

func BenchmarkGetRandomness(b *testing.B) {
	//stm: @CHAIN_GEN_NEXT_TIPSET_001
	//stm: @CHAIN_STATE_GET_RANDOMNESS_FROM_TICKETS_001
	cg, err := gen.NewGenerator()
	if err != nil {
		b.Fatal(err)
	}

	var last *types.TipSet
	for i := 0; i < 2000; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			b.Fatal(err)
		}

		last = ts.TipSet.TipSet()
	}

	r, err := cg.YieldRepo()
	if err != nil {
		b.Fatal(err)
	}

	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		b.Fatal(err)
	}

	bs, err := lr.Blockstore(context.TODO(), repo.UniversalBlockstore)
	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		if c, ok := bs.(io.Closer); ok {
			if err := c.Close(); err != nil {
				b.Logf("WARN: failed to close blockstore: %s", err)
			}
		}
	}()

	mds, err := lr.Datastore(context.Background(), "/metadata")
	if err != nil {
		b.Fatal(err)
	}

	cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := cg.StateManager().GetRandomnessFromTickets(context.TODO(), crypto.DomainSeparationTag_SealRandomness, 500, nil, last.Key())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestChainExportImport(t *testing.T) {
	//stm: @CHAIN_GEN_NEXT_TIPSET_001
	//stm: @CHAIN_STORE_IMPORT_001
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	var last *types.TipSet
	for i := 0; i < 100; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

		last = ts.TipSet.TipSet()
	}

	buf := new(bytes.Buffer)
	if err := cg.ChainStore().Export(context.TODO(), last, 0, false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, datastore.NewMapDatastore(), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, err := cs.Import(context.TODO(), buf)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(last) {
		t.Fatal("imported chain differed from exported chain")
	}
}

// Test to check if tipset key cids are being stored on snapshot
func TestChainImportTipsetKeyCid(t *testing.T) {

	ctx := context.Background()
	cg, err := gen.NewGenerator()
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	var last *types.TipSet
	var tsKeys []types.TipSetKey
	for i := 0; i < 10; i++ {
		ts, err := cg.NextTipSet()
		require.NoError(t, err)
		last = ts.TipSet.TipSet()
		tsKeys = append(tsKeys, last.Key())
	}

	if err := cg.ChainStore().Export(ctx, last, last.Height(), false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, datastore.NewMapDatastore(), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, err := cs.Import(ctx, buf)
	require.NoError(t, err)

	require.Truef(t, root.Equals(last), "imported chain differed from exported chain")

	err = cs.SetHead(ctx, last)
	require.NoError(t, err)

	for _, tsKey := range tsKeys {
		_, err := cs.LoadTipSet(ctx, tsKey)
		require.NoError(t, err)

		tsCid, err := tsKey.Cid()
		require.NoError(t, err)
		_, err = cs.ChainLocalBlockstore().Get(ctx, tsCid)
		require.NoError(t, err)

	}
}

func TestChainExportImportFull(t *testing.T) {
	//stm: @CHAIN_GEN_NEXT_TIPSET_001
	//stm: @CHAIN_STORE_IMPORT_001, @CHAIN_STORE_EXPORT_001, @CHAIN_STORE_SET_HEAD_001
	//stm: @CHAIN_STORE_GET_TIPSET_BY_HEIGHT_001
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	var last *types.TipSet
	for i := 0; i < 100; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

		last = ts.TipSet.TipSet()
	}

	buf := new(bytes.Buffer)
	if err := cg.ChainStore().Export(context.TODO(), last, last.Height(), false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	ds := datastore.NewMapDatastore()
	cs := store.NewChainStore(nbs, nbs, ds, filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, err := cs.Import(context.TODO(), buf)
	if err != nil {
		t.Fatal(err)
	}

	err = cs.SetHead(context.Background(), last)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(last) {
		t.Fatal("imported chain differed from exported chain")
	}

	sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), nil, filcns.DefaultUpgradeSchedule(), cg.BeaconSchedule(), ds, index.DummyMsgIndex)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		ts, err := cs.GetTipsetByHeight(context.TODO(), abi.ChainEpoch(i), nil, false)
		if err != nil {
			t.Fatal(err)
		}

		st, err := sm.ParentState(ts)
		if err != nil {
			t.Fatal(err)
		}

		// touches a bunch of actors
		_, err = sm.GetCirculatingSupply(context.TODO(), abi.ChainEpoch(i), st)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEquivocations(t *testing.T) {
	ctx := context.Background()
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	var last *types.TipSet
	for i := 0; i < 10; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

		last = ts.TipSet.TipSet()
	}

	require.NotEmpty(t, last.Blocks())
	blk1 := *last.Blocks()[0]

	// quick check: asking to form tipset at latest height just returns head
	bestHead, bestHeadWeight, err := cg.ChainStore().FormHeaviestTipSetForHeight(ctx, last.Height())
	require.NoError(t, err)
	require.Equal(t, last.Key(), bestHead.Key())
	require.Contains(t, last.Cids(), blk1.Cid())
	expectedWeight, err := cg.ChainStore().Weight(ctx, bestHead)
	require.NoError(t, err)
	require.Equal(t, expectedWeight, bestHeadWeight)

	// add another block by a different miner -- it should get included in the best tipset
	blk2 := blk1
	blk1Miner, err := address.IDFromAddress(blk2.Miner)
	require.NoError(t, err)
	blk2.Miner, err = address.NewIDAddress(blk1Miner + 50)
	require.NoError(t, err)
	addBlockToTracker(t, cg.ChainStore(), &blk2)

	bestHead, bestHeadWeight, err = cg.ChainStore().FormHeaviestTipSetForHeight(ctx, last.Height())
	require.NoError(t, err)
	for _, blkCid := range last.Cids() {
		require.Contains(t, bestHead.Cids(), blkCid)
	}
	require.Contains(t, bestHead.Cids(), blk2.Cid())
	expectedWeight, err = cg.ChainStore().Weight(ctx, bestHead)
	require.NoError(t, err)
	require.Equal(t, expectedWeight, bestHeadWeight)

	// add another block by a different miner, but on a different tipset -- it should NOT get included
	blk3 := blk1
	blk3.Miner, err = address.NewIDAddress(blk1Miner + 100)
	require.NoError(t, err)
	blk3.Parents = append(blk3.Parents, blk1.Cid())
	addBlockToTracker(t, cg.ChainStore(), &blk3)

	bestHead, bestHeadWeight, err = cg.ChainStore().FormHeaviestTipSetForHeight(ctx, last.Height())
	require.NoError(t, err)
	for _, blkCid := range last.Cids() {
		require.Contains(t, bestHead.Cids(), blkCid)
	}
	require.Contains(t, bestHead.Cids(), blk2.Cid())
	require.NotContains(t, bestHead.Cids(), blk3.Cid())
	expectedWeight, err = cg.ChainStore().Weight(ctx, bestHead)
	require.NoError(t, err)
	require.Equal(t, expectedWeight, bestHeadWeight)

	// add another block by the same miner as blk1 -- it should NOT get included, and blk1 should be excluded too
	blk4 := blk1
	blk4.Timestamp = blk1.Timestamp + 1
	addBlockToTracker(t, cg.ChainStore(), &blk4)

	bestHead, bestHeadWeight, err = cg.ChainStore().FormHeaviestTipSetForHeight(ctx, last.Height())
	require.NoError(t, err)
	for _, blkCid := range last.Cids() {
		if blkCid != blk1.Cid() {
			require.Contains(t, bestHead.Cids(), blkCid)
		}
	}
	require.NotContains(t, bestHead.Cids(), blk4.Cid())
	require.NotContains(t, bestHead.Cids(), blk1.Cid())
	expectedWeight, err = cg.ChainStore().Weight(ctx, bestHead)
	require.NoError(t, err)
	require.Equal(t, expectedWeight, bestHeadWeight)

	// check that after all of that, the chainstore's head has NOT changed
	require.Equal(t, last.Key(), cg.ChainStore().GetHeaviestTipSet().Key())

	// NOW, after all that, notify the chainstore to refresh its head
	require.NoError(t, cg.ChainStore().RefreshHeaviestTipSet(ctx, blk1.Height+1))

	previousHead := last
	newHead := cg.ChainStore().GetHeaviestTipSet()
	// the newHead should be at the same height as the previousHead
	require.Equal(t, previousHead.Height(), newHead.Height())
	// the newHead should NOT be the same as the previousHead
	require.NotEqual(t, previousHead.Key(), newHead.Key())
	// specifically, it should not contain any blocks by blk1Miner
	for _, b := range newHead.Blocks() {
		require.NotEqual(t, blk1.Miner, b.Miner)
	}
}

func addBlockToTracker(t *testing.T, cs *store.ChainStore, blk *types.BlockHeader) {
	blk2Ts, err := types.NewTipSet([]*types.BlockHeader{blk})
	require.NoError(t, err)
	require.NoError(t, cs.PersistTipsets(context.TODO(), []*types.TipSet{blk2Ts}))
	require.NoError(t, cs.AddToTipSetTracker(context.TODO(), blk))
}

package store_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3/certs"
	"github.com/filecoin-project/go-f3/certstore"
	"github.com/filecoin-project/go-f3/gpbft"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
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

	for b.Loop() {
		_, err := cg.StateManager().GetRandomnessFromTickets(context.TODO(), crypto.DomainSeparationTag_SealRandomness, 500, nil, last.Key())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestChainExportImport(t *testing.T) {
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
	if err := cg.ChainStore().ExportV1(context.TODO(), last, 0, false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, datastore.NewMapDatastore(), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, _, err := cs.Import(context.TODO(), nil, buf)
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

	if err := cg.ChainStore().ExportV1(ctx, last, last.Height(), false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, datastore.NewMapDatastore(), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, _, err := cs.Import(ctx, nil, buf)
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
	if err := cg.ChainStore().ExportV1(context.TODO(), last, last.Height(), false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	ds := datastore.NewMapDatastore()
	cs := store.NewChainStore(nbs, nbs, ds, filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, _, err := cs.Import(context.TODO(), nil, buf)
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

	sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), nil, filcns.DefaultUpgradeSchedule(), cg.BeaconSchedule(), ds, nil)
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

func TestChainExportImportWithF3Data(t *testing.T) {
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

	f3ds := datastore.NewMapDatastore()
	pt, ptCid := testPowerTable(10)
	supp := gpbft.SupplementalData{PowerTable: ptCid}
	cst, err := certstore.CreateStore(context.TODO(), f3ds, 0, pt)
	if err != nil {
		t.Fatal(err)
	}

	var lc *certs.FinalityCertificate
	for i := uint64(0); i < 100; i++ {
		cert := makeCert(i, supp)
		err = cst.Put(context.TODO(), cert)
		if err != nil {
			t.Fatal(err)
		}
		lc = cert
	}

	certStore, err := certstore.OpenStore(context.TODO(), f3ds)
	if err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	if err := cg.ChainStore().ExportV2(context.TODO(), last, last.Height(), false, certStore, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	ds := datastore.NewMapDatastore()
	cs := store.NewChainStore(nbs, nbs, ds, filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	nf3ds := datastore.NewMapDatastore()
	root, _, err := cs.Import(context.TODO(), nf3ds, buf)
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

	sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), nil, filcns.DefaultUpgradeSchedule(), cg.BeaconSchedule(), ds, nil)
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

	prefix := store.F3DatastorePrefix()
	f3DsWrapper := namespace.Wrap(nf3ds, prefix)
	importedCertStore, err := certstore.OpenStore(context.Background(), f3DsWrapper)
	if err != nil {
		t.Fatal(err)
	}

	latestCert := importedCertStore.Latest()

	require.NotNil(t, latestCert)
	require.Equal(t, lc.GPBFTInstance, latestCert.GPBFTInstance)
	require.Equal(t, lc.ECChain.TipSets, latestCert.ECChain.TipSets)
	require.Equal(t, lc.SupplementalData, latestCert.SupplementalData)
	require.Equal(t, lc.Signature, latestCert.Signature)
	require.Equal(t, lc.PowerTableDelta, latestCert.PowerTableDelta)
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

	mTs, err := cg.NextTipSetFromMiners(last, []address.Address{last.Blocks()[0].Miner}, 0)
	require.NoError(t, err)
	require.Equal(t, 1, len(mTs.TipSet.TipSet().Cids()))
	last = mTs.TipSet.TipSet()

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
	blk1Parent, err := cg.ChainStore().GetBlock(ctx, blk3.Parents[0])
	require.NoError(t, err)
	blk3.Parents = blk1Parent.Parents
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

	originalHead := *last
	newHead := cg.ChainStore().GetHeaviestTipSet()
	// the newHead should be at the same height as the originalHead
	require.Equal(t, originalHead.Height(), newHead.Height())
	// the newHead should NOT be the same as the originalHead
	require.NotEqual(t, originalHead.Key(), newHead.Key())
	// specifically, it should not contain any blocks by blk1Miner
	for _, b := range newHead.Blocks() {
		require.NotEqual(t, blk1.Miner, b.Miner)
	}

	// now have blk2's Miner equivocate too! this causes us to switch to a tipset with a different parent!
	blk5 := blk2
	blk5.Timestamp = blk5.Timestamp + 1
	addBlockToTracker(t, cg.ChainStore(), &blk5)

	// notify the chainstore to refresh its head
	require.NoError(t, cg.ChainStore().RefreshHeaviestTipSet(ctx, blk1.Height+1))
	newHead = cg.ChainStore().GetHeaviestTipSet()
	// the newHead should still be at the same height as the originalHead
	require.Equal(t, originalHead.Height(), newHead.Height())
	// BUT it should no longer have the same parents -- only blk3's miner is good, and they mined on a different tipset
	require.Equal(t, 1, len(newHead.Blocks()))
	require.Equal(t, blk3.Cid(), newHead.Cids()[0])
	require.NotEqual(t, originalHead.Parents(), newHead.Parents())

	// now have blk3's Miner equivocate too! this causes us to switch to a previous epoch entirely :(
	blk6 := blk3
	blk6.Timestamp = blk6.Timestamp + 1
	addBlockToTracker(t, cg.ChainStore(), &blk6)

	// trying to form a tipset at our previous height leads to emptiness
	tryTs, tryTsWeight, err := cg.ChainStore().FormHeaviestTipSetForHeight(ctx, blk1.Height)
	require.NoError(t, err)
	require.Nil(t, tryTs)
	require.True(t, tryTsWeight.IsZero())

	// notify the chainstore to refresh its head
	require.NoError(t, cg.ChainStore().RefreshHeaviestTipSet(ctx, blk1.Height+1))
	newHead = cg.ChainStore().GetHeaviestTipSet()
	// the newHead should now be one epoch behind originalHead
	require.Greater(t, originalHead.Height(), newHead.Height())

	// next, we create a new tipset with only one block after many null rounds
	headAfterNulls, err := cg.NextTipSetFromMiners(newHead, []address.Address{newHead.Blocks()[0].Miner}, 15)
	require.NoError(t, err)
	require.Equal(t, 1, len(headAfterNulls.TipSet.Blocks))

	// now, we disqualify the block in this tipset because of equivocation
	blkAfterNulls := headAfterNulls.TipSet.TipSet().Blocks()[0]
	equivocatedBlkAfterNulls := *blkAfterNulls
	equivocatedBlkAfterNulls.Timestamp = blkAfterNulls.Timestamp + 1
	addBlockToTracker(t, cg.ChainStore(), &equivocatedBlkAfterNulls)

	// try to form a tipset at this height -- it should be empty
	tryTs2, tryTsWeight2, err := cg.ChainStore().FormHeaviestTipSetForHeight(ctx, blkAfterNulls.Height)
	require.NoError(t, err)
	require.Nil(t, tryTs2)
	require.True(t, tryTsWeight2.IsZero())

	// now we "notify" at this height -- it should lead to no head change because there's no formable head in near epochs
	require.NoError(t, cg.ChainStore().RefreshHeaviestTipSet(ctx, blkAfterNulls.Height))
	require.True(t, headAfterNulls.TipSet.TipSet().Equals(cg.ChainStore().GetHeaviestTipSet()))
}

func addBlockToTracker(t *testing.T, cs *store.ChainStore, blk *types.BlockHeader) {
	blk2Ts, err := types.NewTipSet([]*types.BlockHeader{blk})
	require.NoError(t, err)
	require.NoError(t, cs.PersistTipsets(context.TODO(), []*types.TipSet{blk2Ts}))
	require.NoError(t, cs.AddToTipSetTracker(context.TODO(), blk))
}

// copy from go-f3
func testPowerTable(entries int64) (gpbft.PowerEntries, cid.Cid) {
	powerTable := make(gpbft.PowerEntries, entries)

	for i := range powerTable {
		powerTable[i] = gpbft.PowerEntry{
			ID:     gpbft.ActorID(i + 1),
			Power:  gpbft.NewStoragePower(int64(len(powerTable)*2 - i/2)),
			PubKey: []byte("fake key"),
		}
	}
	k, err := certs.MakePowerTableCID(powerTable)
	if err != nil {
		panic(err)
	}
	return powerTable, k
}

// copy from go-f3
func makeCert(instance uint64, supp gpbft.SupplementalData) *certs.FinalityCertificate {
	return &certs.FinalityCertificate{
		GPBFTInstance:    instance,
		SupplementalData: supp,
		ECChain: &gpbft.ECChain{
			TipSets: []*gpbft.TipSet{
				{Epoch: 0, Key: gpbft.TipSetKey("tsk0"), PowerTable: supp.PowerTable},
			},
		},
	}
}

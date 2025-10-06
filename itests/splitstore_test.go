package itests

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	"github.com/filecoin-project/go-state-types/exitcode"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	power6 "github.com/filecoin-project/specs-actors/v6/actors/builtin/power"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore/splitstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// Startup a node with hotstore and discard coldstore.  Compact once and return
func TestHotstoreCompactsOnce(t *testing.T) {

	ctx := context.Background()
	// disable sync checking because efficient itests require that the node is out of sync : /
	splitstore.CheckSyncGap = false
	opts := []interface{}{kit.MockProofs(), kit.SplitstoreDiscard()}
	full, genesisMiner, ens := kit.EnsembleMinimal(t, opts...)
	bm := ens.InterconnectAll().BeginMining(4 * time.Millisecond)[0]
	_ = full
	_ = genesisMiner
	_ = bm

	waitForCompaction(ctx, t, 1, full)
	require.NoError(t, genesisMiner.Stop(ctx))
}

// create some unreachable state
// and check that compaction carries it away
func TestHotstoreCompactCleansGarbage(t *testing.T) {
	ctx := context.Background()
	// disable sync checking because efficient itests require that the node is out of sync : /
	splitstore.CheckSyncGap = false
	opts := []interface{}{kit.MockProofs(), kit.SplitstoreDiscard()}
	full, genesisMiner, ens := kit.EnsembleMinimal(t, opts...)
	bm := ens.InterconnectAll().BeginMining(4 * time.Millisecond)[0]
	_ = full
	_ = genesisMiner

	// create garbage
	g := NewGarbager(ctx, t, full)
	// state
	garbageS, eS := g.Drop(ctx)
	// message
	garbageM, eM := g.Message(ctx)
	e := eM
	if eS > eM {
		e = eS
	}
	assert.True(g.t, g.Exists(ctx, garbageS), "Garbage state not found in splitstore")
	assert.True(g.t, g.Exists(ctx, garbageM), "Garbage message not found in splitstore")

	// calculate next compaction where we should actually see cleanup

	// pause, check for compacting and get compaction info
	// we do this to remove the (very unlikely) race where compaction index
	// and compaction epoch are in the middle of update, or a whole compaction
	// runs between the two
	for {
		bm.Pause()
		if splitStoreCompacting(ctx, t, full) {
			bm.Restart()
			time.Sleep(3 * time.Second)
		} else {
			break
		}
	}
	lastCompactionEpoch := splitStoreBaseEpoch(ctx, t, full)
	garbageCompactionIndex := splitStoreCompactionIndex(ctx, t, full) + 1
	boundary := lastCompactionEpoch + splitstore.CompactionThreshold - splitstore.CompactionBoundary

	for e > boundary {
		boundary += splitstore.CompactionThreshold - splitstore.CompactionBoundary
		garbageCompactionIndex++
	}
	bm.Restart()

	// wait for compaction to occur
	waitForCompaction(ctx, t, garbageCompactionIndex, full)

	// check that garbage is cleaned up
	assert.False(t, g.Exists(ctx, garbageS), "Garbage state still exists in blockstore")
	assert.False(t, g.Exists(ctx, garbageM), "Garbage message still exists in blockstore")
}

// Create unreachable state
// Check that it moves to coldstore
// Prune coldstore and check that it is deleted
func TestColdStorePrune(t *testing.T) {
	ctx := context.Background()
	// disable sync checking because efficient itests require that the node is out of sync : /
	splitstore.CheckSyncGap = false
	opts := []interface{}{kit.MockProofs(), kit.SplitstoreUniversal(), kit.FsRepo()}
	full, genesisMiner, ens := kit.EnsembleMinimal(t, opts...)
	bm := ens.InterconnectAll().BeginMining(4 * time.Millisecond)[0]
	_ = full
	_ = genesisMiner

	// create garbage
	g := NewGarbager(ctx, t, full)
	// state
	garbageS, eS := g.Drop(ctx)
	// message
	garbageM, eM := g.Message(ctx)
	e := eM
	if eS > eM {
		e = eS
	}
	assert.True(g.t, g.Exists(ctx, garbageS), "Garbage state not found in splitstore")
	assert.True(g.t, g.Exists(ctx, garbageM), "Garbage message not found in splitstore")

	// calculate next compaction where we should actually see cleanup

	// pause, check for compacting and get compaction info
	// we do this to remove the (very unlikely) race where compaction index
	// and compaction epoch are in the middle of update, or a whole compaction
	// runs between the two
	for {
		bm.Pause()
		if splitStoreCompacting(ctx, t, full) {
			bm.Restart()
			time.Sleep(3 * time.Second)
		} else {
			break
		}
	}
	lastCompactionEpoch := splitStoreBaseEpoch(ctx, t, full)
	garbageCompactionIndex := splitStoreCompactionIndex(ctx, t, full) + 1
	boundary := lastCompactionEpoch + splitstore.CompactionThreshold - splitstore.CompactionBoundary

	for e > boundary {
		boundary += splitstore.CompactionThreshold - splitstore.CompactionBoundary
		garbageCompactionIndex++
	}
	bm.Restart()

	// wait for compaction to occur
	waitForCompaction(ctx, t, garbageCompactionIndex, full)

	bm.Pause()

	// This data should now be moved to the coldstore.
	// Access it without hotview to keep it there while checking that it still exists
	// Only state compute uses hot view so garbager Exists backed by ChainReadObj is all good
	assert.True(g.t, g.Exists(ctx, garbageS), "Garbage state not found in splitstore")
	assert.True(g.t, g.Exists(ctx, garbageM), "Garbage message not found in splitstore")
	bm.Restart()

	// wait for compaction to finish and pause to make sure it doesn't start to avoid racing
	for {
		bm.Pause()
		if splitStoreCompacting(ctx, t, full) {
			bm.Restart()
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	pruneOpts := api.PruneOpts{RetainState: int64(0), MovingGC: false}
	require.NoError(t, full.ChainPrune(ctx, pruneOpts))
	bm.Restart()
	waitForPrune(ctx, t, 1, full)
	assert.False(g.t, g.Exists(ctx, garbageS), "Garbage state should be removed from cold store after prune but it's still there")
	assert.True(g.t, g.Exists(ctx, garbageM), "Garbage message should be on the cold store after prune")
}

func TestMessagesMode(t *testing.T) {
	ctx := context.Background()
	// disable sync checking because efficient itests require that the node is out of sync : /
	splitstore.CheckSyncGap = false
	opts := []interface{}{kit.MockProofs(), kit.SplitstoreMessges(), kit.FsRepo()}
	full, genesisMiner, ens := kit.EnsembleMinimal(t, opts...)
	bm := ens.InterconnectAll().BeginMining(4 * time.Millisecond)[0]
	_ = full
	_ = genesisMiner

	// create garbage
	g := NewGarbager(ctx, t, full)
	// state
	garbageS, eS := g.Drop(ctx)
	// message
	garbageM, eM := g.Message(ctx)
	e := eM
	if eS > eM {
		e = eS
	}
	assert.True(g.t, g.Exists(ctx, garbageS), "Garbage state not found in splitstore")
	assert.True(g.t, g.Exists(ctx, garbageM), "Garbage message not found in splitstore")

	// calculate next compaction where we should actually see cleanup

	// pause, check for compacting and get compaction info
	// we do this to remove the (very unlikely) race where compaction index
	// and compaction epoch are in the middle of update, or a whole compaction
	// runs between the two
	for {
		bm.Pause()
		if splitStoreCompacting(ctx, t, full) {
			bm.Restart()
			time.Sleep(3 * time.Second)
		} else {
			break
		}
	}
	lastCompactionEpoch := splitStoreBaseEpoch(ctx, t, full)
	garbageCompactionIndex := splitStoreCompactionIndex(ctx, t, full) + 1
	boundary := lastCompactionEpoch + splitstore.CompactionThreshold - splitstore.CompactionBoundary

	for e > boundary {
		boundary += splitstore.CompactionThreshold - splitstore.CompactionBoundary
		garbageCompactionIndex++
	}
	bm.Restart()

	// wait for compaction to occur
	waitForCompaction(ctx, t, garbageCompactionIndex, full)

	bm.Pause()

	// Messages should be moved to the coldstore
	// State should be gced
	// Access it without hotview to keep it there while checking that it still exists
	// Only state compute uses hot view so garbager Exists backed by ChainReadObj is all good
	assert.False(g.t, g.Exists(ctx, garbageS), "Garbage state not found in splitstore")
	assert.True(g.t, g.Exists(ctx, garbageM), "Garbage message not found in splitstore")
}

func TestCompactRetainsTipSetRef(t *testing.T) {
	ctx := context.Background()
	// disable sync checking because efficient itests require that the node is out of sync : /
	splitstore.CheckSyncGap = false
	opts := []interface{}{kit.MockProofs(), kit.SplitstoreDiscard()}
	full, genesisMiner, ens := kit.EnsembleMinimal(t, opts...)
	bm := ens.InterconnectAll().BeginMining(4 * time.Millisecond)[0]
	_ = genesisMiner
	_ = bm

	check, err := full.ChainHead(ctx)
	require.NoError(t, err)
	e := check.Height()
	checkRef, err := check.Key().Cid()
	require.NoError(t, err)
	assert.True(t, ipldExists(ctx, t, checkRef, full)) // reference to tipset key should be persisted before compaction

	// Determine index of compaction that covers tipset "check" and wait for compaction
	for {
		bm.Pause()
		if splitStoreCompacting(ctx, t, full) {
			bm.Restart()
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
	lastCompactionEpoch := splitStoreBaseEpoch(ctx, t, full)
	garbageCompactionIndex := splitStoreCompactionIndex(ctx, t, full) + 1
	boundary := lastCompactionEpoch + splitstore.CompactionThreshold - splitstore.CompactionBoundary

	for e > boundary {
		boundary += splitstore.CompactionThreshold - splitstore.CompactionBoundary
		garbageCompactionIndex++
	}
	bm.Restart()

	// wait for compaction to occur
	waitForCompaction(ctx, t, garbageCompactionIndex, full)
	assert.True(t, ipldExists(ctx, t, checkRef, full)) // reference to tipset key should be persisted after compaction
	bm.Stop()
}

func waitForCompaction(ctx context.Context, t *testing.T, cIdx int64, n *kit.TestFullNode) {
	for {
		if splitStoreCompactionIndex(ctx, t, n) >= cIdx {
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func waitForPrune(ctx context.Context, t *testing.T, pIdx int64, n *kit.TestFullNode) {
	for {
		if splitStorePruneIndex(ctx, t, n) >= pIdx {
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func splitStoreCompacting(ctx context.Context, t *testing.T, n *kit.TestFullNode) bool {
	info, err := n.ChainBlockstoreInfo(ctx)
	require.NoError(t, err)
	compactingRaw, ok := info["compacting"]
	require.True(t, ok, "compactions not on blockstore info")
	compacting, ok := compactingRaw.(bool)
	require.True(t, ok, "compacting key on blockstore info wrong type")
	return compacting
}

func splitStoreBaseEpoch(ctx context.Context, t *testing.T, n *kit.TestFullNode) abi.ChainEpoch {
	info, err := n.ChainBlockstoreInfo(ctx)
	require.NoError(t, err)
	baseRaw, ok := info["base epoch"]
	require.True(t, ok, "'base epoch' not on blockstore info")
	base, ok := baseRaw.(abi.ChainEpoch)
	require.True(t, ok, "base epoch key on blockstore info wrong type")
	return base
}

func splitStoreCompactionIndex(ctx context.Context, t *testing.T, n *kit.TestFullNode) int64 {
	info, err := n.ChainBlockstoreInfo(ctx)
	require.NoError(t, err)
	compact, ok := info["compactions"]
	require.True(t, ok, "compactions not on blockstore info")
	compactionIndex, ok := compact.(int64)
	require.True(t, ok, "compaction key on blockstore info wrong type")
	return compactionIndex
}

func splitStorePruneIndex(ctx context.Context, t *testing.T, n *kit.TestFullNode) int64 {
	info, err := n.ChainBlockstoreInfo(ctx)
	require.NoError(t, err)
	prune, ok := info["prunes"]
	require.True(t, ok, "prunes not on blockstore info")
	pruneIndex, ok := prune.(int64)
	require.True(t, ok, "prune key on blockstore info wrong type")
	return pruneIndex
}

func ipldExists(ctx context.Context, t *testing.T, c cid.Cid, n *kit.TestFullNode) bool {
	found, err := n.ChainHasObj(ctx, c)
	if err != nil {
		t.Fatalf("ChainHasObj failure: %s", err)
	}
	return found
}

// Create on chain unreachable garbage for a network to exercise splitstore
// one garbage cid created at a time
//
// It works by rewriting an internally maintained miner actor's peer ID
type Garbager struct {
	t      *testing.T
	node   *kit.TestFullNode
	latest trashID

	// internal tracking
	maddr4Data address.Address
}

type trashID uint8

func NewGarbager(ctx context.Context, t *testing.T, n *kit.TestFullNode) *Garbager {
	// create miner actor for writing garbage

	g := &Garbager{
		t:          t,
		node:       n,
		latest:     0,
		maddr4Data: address.Undef,
	}
	g.createMiner4Data(ctx)
	g.newPeerID(ctx)
	return g
}

// Drop returns the cid referencing the dropped garbage and the chain epoch of the drop
func (g *Garbager) Drop(ctx context.Context) (cid.Cid, abi.ChainEpoch) {
	// record existing with mInfoCidAtEpoch
	c := g.mInfoCid(ctx)

	// update trashID and create newPeerID, dropping miner info cid c in the process
	// wait for message and return the chain height that the drop occurred at
	g.latest++
	return c, g.newPeerID(ctx)
}

// Message returns the cid referencing a message and the chain epoch it went on chain
func (g *Garbager) Message(ctx context.Context) (cid.Cid, abi.ChainEpoch) {
	mw := g.createMiner(ctx)
	return mw.Message, mw.Height
}

// Exists checks whether the cid is reachable through the node
func (g *Garbager) Exists(ctx context.Context, c cid.Cid) bool {
	// check chain get / blockstore get
	_, err := g.node.ChainReadObj(ctx, c)
	if ipld.IsNotFound(err) {
		return false
	} else if err != nil {
		g.t.Fatalf("ChainReadObj failure on existence check: %s", err)
		return false // unreachable
	}
	return true
}

func (g *Garbager) newPeerID(ctx context.Context) abi.ChainEpoch {
	dataStr := fmt.Sprintf("Garbager-Data-%d", g.latest)
	dataID := []byte(dataStr)
	params, err := actors.SerializeParams(&miner2.ChangePeerIDParams{NewID: dataID})
	require.NoError(g.t, err)

	msg := &types.Message{
		To:     g.maddr4Data,
		From:   g.node.DefaultKey.Address,
		Method: builtin.MethodsMiner.ChangePeerID,
		Params: params,
		Value:  types.NewInt(0),
	}

	signed, err2 := g.node.MpoolPushMessage(ctx, msg, nil)
	require.NoError(g.t, err2)

	mw, err2 := g.node.StateWaitMsg(ctx, signed.Cid(), buildconstants.MessageConfidence, api.LookbackNoLimit, true)
	require.NoError(g.t, err2)
	require.Equal(g.t, exitcode.Ok, mw.Receipt.ExitCode)
	return mw.Height
}

func (g *Garbager) mInfoCid(ctx context.Context) cid.Cid {
	ts, err := g.node.ChainHead(ctx)
	require.NoError(g.t, err)

	act, err := g.node.StateGetActor(ctx, g.maddr4Data, ts.Key())
	require.NoError(g.t, err)
	raw, err := g.node.ChainReadObj(ctx, act.Head)
	require.NoError(g.t, err)
	var mSt miner16.State
	require.NoError(g.t, mSt.UnmarshalCBOR(bytes.NewReader(raw)))

	//	return infoCid
	return mSt.Info
}

func (g *Garbager) createMiner4Data(ctx context.Context) {
	require.True(g.t, g.maddr4Data == address.Undef, "garbager miner actor already created")
	mw := g.createMiner(ctx)
	var retval power6.CreateMinerReturn
	require.NoError(g.t, retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return)))
	g.maddr4Data = retval.IDAddress
}

func (g *Garbager) createMiner(ctx context.Context) *lapi.MsgLookup {
	owner, err := g.node.WalletDefaultAddress(ctx)
	require.NoError(g.t, err)
	worker := owner

	params, err := actors.SerializeParams(&power6.CreateMinerParams{
		Owner:               owner,
		Worker:              worker,
		WindowPoStProofType: abi.RegisteredPoStProof_StackedDrgWindow32GiBV1_1,
	})
	require.NoError(g.t, err)

	createStorageMinerMsg := &types.Message{
		To:    power.Address,
		From:  worker,
		Value: types.FromFil(1),

		Method: power.Methods.CreateMiner,
		Params: params,
	}

	signed, err := g.node.MpoolPushMessage(ctx, createStorageMinerMsg, nil)
	require.NoError(g.t, err)
	mw, err := g.node.StateWaitMsg(ctx, signed.Cid(), buildconstants.MessageConfidence, lapi.LookbackNoLimit, true)
	require.NoError(g.t, err)
	require.True(g.t, mw.Receipt.ExitCode == 0, "garbager's internal create miner message failed")
	return mw
}

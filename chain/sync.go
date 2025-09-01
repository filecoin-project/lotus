package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Gurpartap/async"
	"github.com/hashicorp/go-multierror"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/pubsub"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/node/modules/dtypes"

	// named msgarray here to make it clear that these are the types used by
	// messages, regardless of specs-actors version.
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/exchange"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/metrics"
)

var (
	// LocalIncoming is the _local_ pubsub (unrelated to libp2p pubsub) topic
	// where the Syncer publishes candidate chain heads to be synced.
	LocalIncoming = "incoming"

	log = logging.Logger("chain")

	concurrentSyncRequests = exchange.ShufflePeersPrefix
	syncRequestBatchSize   = 8
	syncRequestRetries     = 5
)

// Syncer is in charge of running the chain synchronization logic. As such, it
// is tasked with these functions, amongst others:
//
//   - Fast-forwards the chain as it learns of new TipSets from the network via
//     the SyncManager.
//   - Applies the fork choice rule to select the correct side when confronted
//     with a fork in the network.
//   - Requests block headers and messages from other peers when not available
//     in our BlockStore.
//   - Tracks blocks marked as bad in a cache.
//   - Keeps the BlockStore and ChainStore consistent with our view of the world,
//     the latter of which in turn informs other components when a reorg has been
//     committed.
//
// The Syncer does not run workers itself. It's mainly concerned with
// ensuring a consistent state of chain consensus. The reactive and network-
// interfacing processes are part of other components, such as the SyncManager
// (which owns the sync scheduler and sync workers), ChainExchange, the HELLO
// protocol, and the gossipsub block propagation layer.
//
// {hint/concept} The fork-choice rule as it currently stands is: "pick the
// chain with the heaviest weight, so long as it hasn’t deviated one finality
// threshold from our head (900 epochs, parameter determined by spec-actors)".
type Syncer struct {
	// The interface for accessing and putting tipsets into local storage
	store *store.ChainStore

	// handle to the random beacon for verification
	beacon beacon.Schedule

	// the state manager handles making state queries
	sm *stmgr.StateManager

	consensus consensus.Consensus

	// The known Genesis tipset
	Genesis *types.TipSet

	// TipSets known to be invalid
	bad *BadBlockCache

	// handle to the block sync service
	Exchange exchange.Client

	self peer.ID

	syncmgr SyncManager

	connmgr connmgr.ConnManager

	incoming *pubsub.PubSub

	receiptTracker *blockReceiptTracker

	tickerCtxCancel context.CancelFunc

	ds dtypes.MetadataDS
}

type SyncManagerCtor func(syncFn SyncFunc) SyncManager

type Genesis *types.TipSet

func LoadGenesis(ctx context.Context, sm *stmgr.StateManager) (Genesis, error) {
	gen, err := sm.ChainStore().GetGenesis(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting genesis block: %w", err)
	}

	return types.NewTipSet([]*types.BlockHeader{gen})
}

// NewSyncer creates a new Syncer object.
func NewSyncer(ds dtypes.MetadataDS,
	sm *stmgr.StateManager,
	exchange exchange.Client,
	syncMgrCtor SyncManagerCtor,
	connmgr connmgr.ConnManager,
	self peer.ID,
	beacon beacon.Schedule,
	gent Genesis,
	consensus consensus.Consensus) (*Syncer, error) {

	s := &Syncer{
		ds:             ds,
		beacon:         beacon,
		bad:            NewBadBlockCache(),
		Genesis:        gent,
		consensus:      consensus,
		Exchange:       exchange,
		store:          sm.ChainStore(),
		sm:             sm,
		self:           self,
		receiptTracker: newBlockReceiptTracker(),
		connmgr:        connmgr,

		incoming: pubsub.New(50),
	}

	s.syncmgr = syncMgrCtor(s.Sync)
	return s, nil
}

func (syncer *Syncer) Start() {
	tickerCtx, tickerCtxCancel := context.WithCancel(context.Background())
	syncer.syncmgr.Start()

	syncer.tickerCtxCancel = tickerCtxCancel

	go syncer.runMetricsTricker(tickerCtx)
}

func (syncer *Syncer) runMetricsTricker(tickerCtx context.Context) {
	genesisTime := time.Unix(int64(syncer.Genesis.MinTimestamp()), 0)
	ticker := build.Clock.Ticker(time.Duration(buildconstants.BlockDelaySecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sinceGenesis := build.Clock.Now().Sub(genesisTime)
			expectedHeight := int64(sinceGenesis.Seconds()) / int64(buildconstants.BlockDelaySecs)

			stats.Record(tickerCtx, metrics.ChainNodeHeightExpected.M(expectedHeight))
		case <-tickerCtx.Done():
			return
		}
	}
}

func (syncer *Syncer) Stop() {
	syncer.syncmgr.Stop()
	syncer.tickerCtxCancel()
}

// InformNewHead informs the syncer about a new potential tipset
// This should be called when connecting to new peers, and additionally
// when receiving new blocks from the network
func (syncer *Syncer) InformNewHead(from peer.ID, fts *store.FullTipSet) bool {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic in InformNewHead: %s", err)
		}
	}()

	ctx := context.Background()
	if fts == nil {
		log.Errorf("got nil tipset in InformNewHead")
		return false
	}

	if !syncer.consensus.IsEpochInConsensusRange(fts.TipSet().Height()) {
		log.Infof("received block outside of consensus range at height %d", fts.TipSet().Height())
		return false
	}

	for _, b := range fts.Blocks {
		if reason, ok := syncer.bad.Has(b.Cid()); ok {
			log.Warnf("InformNewHead called on block marked as bad: %s (reason: %s)", b.Cid(), reason)
			return false
		}
		if err := syncer.ValidateMsgMeta(b); err != nil {
			log.Warnf("invalid block received: %s", err)
			return false
		}
	}

	syncer.incoming.Pub(fts.TipSet().Blocks(), LocalIncoming)

	// TODO: IMPORTANT(GARBAGE) this needs to be put in the 'temporary' side of
	// the blockstore
	if err := syncer.store.PersistTipsets(ctx, []*types.TipSet{fts.TipSet()}); err != nil {
		log.Warn("failed to persist incoming block header: ", err)
		return false
	}

	syncer.Exchange.AddPeer(from)

	hts := syncer.store.GetHeaviestTipSet()
	bestPweight := hts.ParentWeight()
	targetWeight := fts.TipSet().ParentWeight()
	if targetWeight.LessThan(bestPweight) {
		var miners []string
		for _, blk := range fts.TipSet().Blocks() {
			miners = append(miners, blk.Miner.String())
		}
		log.Debugw("incoming tipset does not appear to be better than our best chain, ignoring for now", "miners", miners, "bestPweight", bestPweight, "bestTS", hts.Cids(), "incomingWeight", targetWeight, "incomingTS", fts.TipSet().Cids())
		return false
	}

	syncer.syncmgr.SetPeerHead(ctx, from, fts.TipSet())
	return true
}

// IncomingBlocks spawns a goroutine that subscribes to the local eventbus to
// receive new block headers as they arrive from the network, and sends them to
// the returned channel.
//
// These blocks have not necessarily been incorporated to our view of the chain.
func (syncer *Syncer) IncomingBlocks(ctx context.Context) (<-chan *types.BlockHeader, error) {
	sub := syncer.incoming.Sub(LocalIncoming)
	out := make(chan *types.BlockHeader, 10)

	go func() {
		defer syncer.incoming.Unsub(sub)

		for {
			select {
			case r := <-sub:
				hs := r.([]*types.BlockHeader)
				for _, h := range hs {
					select {
					case out <- h:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// ValidateMsgMeta performs structural and content hash validation of the
// messages within this block. If validation passes, it stores the messages in
// the underlying IPLD block store.
func (syncer *Syncer) ValidateMsgMeta(fblk *types.FullBlock) error {
	if msgc := len(fblk.BlsMessages) + len(fblk.SecpkMessages); msgc > buildconstants.BlockMessageLimit {
		return xerrors.Errorf("block %s has too many messages (%d)", fblk.Header.Cid(), msgc)
	}

	// TODO: IMPORTANT(GARBAGE). These message puts and the msgmeta
	// computation need to go into the 'temporary' side of the blockstore when
	// we implement that

	// We use a temporary bstore here to avoid writing intermediate pieces
	// into the blockstore.
	blockstore := bstore.NewMemory()
	cst := cbor.NewCborStore(blockstore)
	ctx := context.Background()
	var bcids, scids []cid.Cid

	for _, m := range fblk.BlsMessages {
		c, err := store.PutMessage(ctx, blockstore, m)
		if err != nil {
			return xerrors.Errorf("putting bls message to blockstore after msgmeta computation: %w", err)
		}
		bcids = append(bcids, c)
	}

	for _, m := range fblk.SecpkMessages {
		c, err := store.PutMessage(ctx, blockstore, m)
		if err != nil {
			return xerrors.Errorf("putting bls message to blockstore after msgmeta computation: %w", err)
		}
		scids = append(scids, c)
	}

	// Compute the root CID of the combined message trie.
	smroot, err := computeMsgMeta(cst, bcids, scids)
	if err != nil {
		return xerrors.Errorf("validating msgmeta, compute failed: %w", err)
	}

	// Check that the message trie root matches with what's in the block.
	if fblk.Header.Messages != smroot {
		return xerrors.Errorf("messages in full block did not match msgmeta root in header (%s != %s)", fblk.Header.Messages, smroot)
	}

	// Finally, flush.
	return vm.Copy(context.TODO(), blockstore, syncer.store.ChainBlockstore(), smroot)
}

func (syncer *Syncer) LocalPeer() peer.ID {
	return syncer.self
}

func (syncer *Syncer) ChainStore() *store.ChainStore {
	return syncer.store
}

func (syncer *Syncer) InformNewBlock(from peer.ID, blk *types.FullBlock) bool {
	// TODO: search for other blocks that could form a tipset with this block
	// and then send that tipset to InformNewHead

	fts := &store.FullTipSet{Blocks: []*types.FullBlock{blk}}
	return syncer.InformNewHead(from, fts)
}

func copyBlockstore(ctx context.Context, from, to bstore.Blockstore) error {
	ctx, span := trace.StartSpan(ctx, "copyBlockstore")
	defer span.End()

	cids, err := from.AllKeysChan(ctx)
	if err != nil {
		return err
	}

	// TODO: should probably expose better methods on the blockstore for this operation
	var blks []blocks.Block
	for c := range cids {
		b, err := from.Get(ctx, c)
		if err != nil {
			return err
		}

		blks = append(blks, b)
	}

	if err := to.PutMany(ctx, blks); err != nil {
		return err
	}

	return nil
}

// TODO: this function effectively accepts unchecked input from the network,
// either validate it here, or ensure that its validated elsewhere (maybe make
// sure the blocksync code checks it?)
// maybe this code should actually live in blocksync??
func zipTipSetAndMessages(bs cbor.IpldStore, ts *types.TipSet, allbmsgs []*types.Message, allsmsgs []*types.SignedMessage, bmi, smi [][]uint64) (*store.FullTipSet, error) {
	if len(ts.Blocks()) != len(smi) || len(ts.Blocks()) != len(bmi) {
		return nil, fmt.Errorf("msgincl length didnt match tipset size")
	}

	if err := checkMsgMeta(ts, allbmsgs, allsmsgs, bmi, smi); err != nil {
		return nil, err
	}

	fts := &store.FullTipSet{}
	for bi, b := range ts.Blocks() {

		var smsgs []*types.SignedMessage
		for _, m := range smi[bi] {
			smsgs = append(smsgs, allsmsgs[m])
		}

		var bmsgs []*types.Message
		for _, m := range bmi[bi] {
			bmsgs = append(bmsgs, allbmsgs[m])
		}

		fb := &types.FullBlock{
			Header:        b,
			BlsMessages:   bmsgs,
			SecpkMessages: smsgs,
		}

		fts.Blocks = append(fts.Blocks, fb)
	}

	return fts, nil
}

// computeMsgMeta computes the root CID of the combined arrays of message CIDs
// of both types (BLS and Secpk).
func computeMsgMeta(bs cbor.IpldStore, bmsgCids, smsgCids []cid.Cid) (cid.Cid, error) {
	// block headers use adt0
	store := blockadt.WrapStore(context.TODO(), bs)
	bmArr := blockadt.MakeEmptyArray(store)
	smArr := blockadt.MakeEmptyArray(store)

	for i, m := range bmsgCids {
		c := cbg.CborCid(m)
		if err := bmArr.Set(uint64(i), &c); err != nil {
			return cid.Undef, err
		}
	}

	for i, m := range smsgCids {
		c := cbg.CborCid(m)
		if err := smArr.Set(uint64(i), &c); err != nil {
			return cid.Undef, err
		}
	}

	bmroot, err := bmArr.Root()
	if err != nil {
		return cid.Undef, err
	}

	smroot, err := smArr.Root()
	if err != nil {
		return cid.Undef, err
	}

	mrcid, err := store.Put(store.Context(), &types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to put msgmeta: %w", err)
	}

	return mrcid, nil
}

// FetchTipSet tries to load the provided tipset from the store, and falls back
// to the network (client) by querying the supplied peer if not found
// locally.
//
// {hint/usage} This is used from the HELLO protocol, to fetch the greeting
// peer's heaviest tipset if we don't have it.
func (syncer *Syncer) FetchTipSet(ctx context.Context, p peer.ID, tsk types.TipSetKey) (*store.FullTipSet, error) {
	if fts, err := syncer.tryLoadFullTipSet(ctx, tsk); err == nil {
		return fts, nil
	}

	// fall back to the network.
	return syncer.Exchange.GetFullTipSet(ctx, p, tsk)
}

// tryLoadFullTipSet queries the tipset in the ChainStore, and returns a full
// representation of it containing FullBlocks. If ALL blocks are not found
// locally, it errors entirely with blockstore.ErrNotFound.
func (syncer *Syncer) tryLoadFullTipSet(ctx context.Context, tsk types.TipSetKey) (*store.FullTipSet, error) {
	ts, err := syncer.store.LoadTipSet(ctx, tsk)
	if err != nil {
		return nil, err
	}

	fts := &store.FullTipSet{}
	for _, b := range ts.Blocks() {
		bmsgs, smsgs, err := syncer.store.MessagesForBlock(ctx, b)
		if err != nil {
			return nil, err
		}

		fb := &types.FullBlock{
			Header:        b,
			BlsMessages:   bmsgs,
			SecpkMessages: smsgs,
		}
		fts.Blocks = append(fts.Blocks, fb)
	}

	return fts, nil
}

// Sync tries to advance our view of the chain to `maybeHead`. It does nothing
// if our current head is heavier than the requested tipset, or if we're already
// at the requested head, or if the head is the genesis.
//
// Most of the heavy-lifting logic happens in syncer#collectChain. Refer to the
// godocs on that method for a more detailed view.
func (syncer *Syncer) Sync(ctx context.Context, maybeHead *types.TipSet) error {
	ctx, span := trace.StartSpan(ctx, "chain.Sync")
	defer span.End()

	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("tipset", fmt.Sprint(maybeHead.Cids())),
			trace.Int64Attribute("height", int64(maybeHead.Height())),
		)
	}

	hts := syncer.store.GetHeaviestTipSet()

	// noop pre-checks
	if hts.ParentWeight().GreaterThan(maybeHead.ParentWeight()) {
		return nil
	}
	if syncer.Genesis.Equals(maybeHead) || hts.Equals(maybeHead) {
		return nil
	}

	if maybeHead.Height() == hts.Height() {
		// check if maybeHead is fully contained in headTipSet
		// meaning we already synced all the blocks that are a part of maybeHead
		// if that is the case, there is nothing for us to do
		// we need to exit out early, otherwise checkpoint-fork logic might wrongly reject it
		fullyContained := true
		for _, c := range maybeHead.Cids() {
			if !hts.Contains(c) {
				fullyContained = false
				break
			}
		}
		if fullyContained {
			return nil
		}
	}
	// end of noop prechecks

	if err := syncer.collectChain(ctx, maybeHead, hts, false); err != nil {
		span.AddAttributes(trace.StringAttribute("col_error", err.Error()))
		span.SetStatus(trace.Status{
			Code:    13,
			Message: err.Error(),
		})
		return xerrors.Errorf("collectChain failed: %w", err)
	}

	// At this point we have accepted and synced to the new `maybeHead`
	// (`StageSyncComplete`).
	if err := syncer.store.RefreshHeaviestTipSet(ctx, maybeHead.Height()); err != nil {
		span.AddAttributes(trace.StringAttribute("put_error", err.Error()))
		span.SetStatus(trace.Status{
			Code:    13,
			Message: err.Error(),
		})
		return xerrors.Errorf("failed to put synced tipset to chainstore: %w", err)
	}

	peers := syncer.receiptTracker.GetPeers(maybeHead)
	if len(peers) > 0 {
		syncer.connmgr.TagPeer(peers[0], "new-block", 40)

		for _, p := range peers[1:] {
			syncer.connmgr.TagPeer(p, "new-block", 25)
		}
	}

	return nil
}

func isPermanent(err error) bool {
	return !errors.Is(err, consensus.ErrTemporal)
}

func (syncer *Syncer) ValidateTipSet(ctx context.Context, fts *store.FullTipSet, useCache bool) error {
	ctx, span := trace.StartSpan(ctx, "validateTipSet")
	defer span.End()

	span.AddAttributes(trace.Int64Attribute("height", int64(fts.TipSet().Height())))

	ts := fts.TipSet()
	if ts.Equals(syncer.Genesis) {
		return nil
	}

	var futures []async.ErrorFuture
	for _, b := range fts.Blocks {
		b := b // rebind to a scoped variable

		futures = append(futures, async.Err(func() error {
			if err := syncer.ValidateBlock(ctx, b, useCache); err != nil {
				if isPermanent(err) {
					syncer.bad.Add(b.Cid(), NewBadBlockReason([]cid.Cid{b.Cid()}, "%s", err.Error()))
				}
				return xerrors.Errorf("validating block %s: %w", b.Cid(), err)
			}

			if err := syncer.sm.ChainStore().AddToTipSetTracker(ctx, b.Header); err != nil {
				return xerrors.Errorf("failed to add validated header to tipset tracker: %w", err)
			}
			return nil
		}))
	}
	for _, f := range futures {
		if err := f.AwaitContext(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ValidateBlock should match up with 'Semantical Validation' in validation.md in the spec
func (syncer *Syncer) ValidateBlock(ctx context.Context, b *types.FullBlock, useCache bool) (err error) {
	defer func() {
		// b.Cid() could panic for empty blocks that are used in tests.
		if rerr := recover(); rerr != nil {
			err = xerrors.Errorf("validate block panic: %w", rerr)
			return
		}
	}()

	if useCache {
		isValidated, err := syncer.store.IsBlockValidated(ctx, b.Cid())
		if err != nil {
			return xerrors.Errorf("check block validation cache %s: %w", b.Cid(), err)
		}

		if isValidated {
			return nil
		}
	}

	validationStart := build.Clock.Now()
	defer func() {
		stats.Record(ctx, metrics.BlockValidationDurationMilliseconds.M(metrics.SinceInMilliseconds(validationStart)))
		log.Infow("block validation", "took", time.Since(validationStart), "height", b.Header.Height, "age", time.Since(time.Unix(int64(b.Header.Timestamp), 0)))
	}()

	ctx, span := trace.StartSpan(ctx, "validateBlock")
	defer span.End()

	if err := syncer.consensus.ValidateBlock(ctx, b); err != nil {
		return err
	}

	if useCache {
		if err := syncer.store.MarkBlockAsValidated(ctx, b.Cid()); err != nil {
			return xerrors.Errorf("caching block validation %s: %w", b.Cid(), err)
		}
	}

	return nil
}

type syncStateKey struct{}

func extractSyncState(ctx context.Context) *SyncerState {
	v := ctx.Value(syncStateKey{})
	if v != nil {
		return v.(*SyncerState)
	}
	return nil
}

// collectHeaders collects the headers from the blocks between any two tipsets.
//
// `incoming` is the heaviest/projected/target tipset we have learned about, and
// `known` is usually an anchor tipset we already have in our view of the chain
// (which could be the genesis).
//
// collectHeaders checks if portions of the chain are in our ChainStore; falling
// down to the network to retrieve the missing parts. If during the process, any
// portion we receive is in our denylist (bad list), we short-circuit.
//
// {hint/usage}: This is used by collectChain, which is in turn called from the
// main Sync method (Syncer#Sync), so it's a pretty central method.
//
// {hint/logic}: The logic of this method is as follows:
//
//  1. Check that the from tipset is not linked to a parent block known to be
//     bad.
//  2. Check the consistency of beacon entries in the from tipset. We check
//     total equality of the BeaconEntries in each block.
//  3. Traverse the chain backwards, for each tipset:
//     3a. Load it from the chainstore; if found, it move on to its parent.
//     3b. Query our peers via client in batches, requesting up to a
//     maximum of 500 tipsets every time.
//
// Once we've concluded, if we find a mismatching tipset at the height where the
// anchor tipset should be, we are facing a fork, and we invoke Syncer#syncFork
// to resolve it. Refer to the godocs there.
//
// All throughout the process, we keep checking if the received blocks are in
// the deny list, and short-circuit the process if so.
func (syncer *Syncer) collectHeaders(ctx context.Context, incoming *types.TipSet, known *types.TipSet, ignoreCheckpoint bool) ([]*types.TipSet, error) {
	ctx, span := trace.StartSpan(ctx, "collectHeaders")
	defer span.End()
	ss := extractSyncState(ctx)

	span.AddAttributes(
		trace.Int64Attribute("incomingHeight", int64(incoming.Height())),
		trace.Int64Attribute("knownHeight", int64(known.Height())),
	)

	// Check if the parents of the from block are in the denylist.
	// i.e. if a fork of the chain has been requested that we know to be bad.
	for _, pcid := range incoming.Parents().Cids() {
		if reason, ok := syncer.bad.Has(pcid); ok {
			newReason := reason.Linked("linked to %s", pcid)
			for _, b := range incoming.Cids() {
				syncer.bad.Add(b, newReason)
			}
			return nil, xerrors.Errorf("chain linked to block marked previously as bad (%s, %s) (reason: %s)", incoming.Cids(), pcid, reason)
		}
	}

	{
		// ensure consistency of beacon entries
		targetBE := incoming.Blocks()[0].BeaconEntries
		sorted := sort.SliceIsSorted(targetBE, func(i, j int) bool {
			return targetBE[i].Round < targetBE[j].Round
		})
		if !sorted {
			syncer.bad.Add(incoming.Cids()[0], NewBadBlockReason(incoming.Cids(), "wrong order of beacon entries"))
			return nil, xerrors.Errorf("wrong order of beacon entries")
		}

		for _, bh := range incoming.Blocks()[1:] {
			if len(targetBE) != len(bh.BeaconEntries) {
				// cannot mark bad, I think @Kubuxu
				return nil, xerrors.Errorf("tipset contained different number for beacon entries")
			}
			for i, be := range bh.BeaconEntries {
				if targetBE[i].Round != be.Round || !bytes.Equal(targetBE[i].Data, be.Data) {
					// cannot mark bad, I think @Kubuxu
					return nil, xerrors.Errorf("tipset contained different beacon entries")
				}
			}

		}
	}

	blockSet := []*types.TipSet{incoming}

	// Parent of the new (possibly better) tipset that we need to fetch next.
	at := incoming.Parents()

	// we want to sync all the blocks until the height above our
	// best tipset so far
	untilHeight := known.Height() + 1

	ss.SetHeight(blockSet[len(blockSet)-1].Height())

	var acceptedBlocks []cid.Cid

loop:
	for blockSet[len(blockSet)-1].Height() > untilHeight {
		for _, bc := range at.Cids() {
			if reason, ok := syncer.bad.Has(bc); ok {
				newReason := reason.Linked("change contained %s", bc)
				for _, b := range acceptedBlocks {
					syncer.bad.Add(b, newReason)
				}

				return nil, xerrors.Errorf("chain contained block marked previously as bad (%s, %s) (reason: %s)", incoming.Cids(), bc, reason)
			}
		}

		// If, for some reason, we have a suffix of the chain locally, handle that here
		ts, err := syncer.store.LoadTipSet(ctx, at)
		if err == nil {
			acceptedBlocks = append(acceptedBlocks, at.Cids()...)

			blockSet = append(blockSet, ts)
			at = ts.Parents()
			continue
		}
		if !ipld.IsNotFound(err) {
			log.Warnf("loading local tipset: %s", err)
		}

		// NB: GetBlocks validates that the blocks are in-fact the ones we
		// requested, and that they are correctly linked to one another. It does
		// not validate any state transitions.
		window := 500
		if gap := int(blockSet[len(blockSet)-1].Height() - untilHeight); gap < window {
			window = gap
		}
		blks, err := syncer.Exchange.GetBlocks(ctx, at, window)
		if err != nil {
			// Most likely our peers aren't fully synced yet, but forwarded
			// new block message (ideally we'd find better peers)

			log.Errorf("failed to get blocks: %+v", err)

			span.AddAttributes(trace.StringAttribute("error", err.Error()))

			// This error will only be logged above,
			return nil, xerrors.Errorf("failed to get blocks: %w", err)
		}
		log.Info("Got blocks: ", blks[0].Height(), len(blks))

		// Check that the fetched segment of the chain matches what we already
		// have. Since we fetch from the head backwards our reassembled chain
		// is sorted in reverse here: we have a child -> parent order, our last
		// tipset then should be child of the first tipset retrieved.
		// FIXME: The reassembly logic should be part of the `client`
		//  service, the consumer should not be concerned with the
		//  `MaxRequestLength` limitation, it should just be able to request
		//  an segment of arbitrary length. The same burden is put on
		//  `syncFork()` which needs to be aware this as well.
		if blockSet[len(blockSet)-1].IsChildOf(blks[0]) == false {
			return nil, xerrors.Errorf("retrieved segments of the chain are not connected at heights %d/%d",
				blockSet[len(blockSet)-1].Height(), blks[0].Height())
			// A successful `GetBlocks()` call is guaranteed to fetch at least
			// one tipset so the access `blks[0]` is safe.
		}

		for _, b := range blks {
			if b.Height() < untilHeight {
				break loop
			}
			for _, bc := range b.Cids() {
				if reason, ok := syncer.bad.Has(bc); ok {
					newReason := reason.Linked("change contained %s", bc)
					for _, b := range acceptedBlocks {
						syncer.bad.Add(b, newReason)
					}

					return nil, xerrors.Errorf("chain contained block marked previously as bad (%s, %s) (reason: %s)", incoming.Cids(), bc, reason)
				}
			}
			blockSet = append(blockSet, b)
		}

		acceptedBlocks = append(acceptedBlocks, at.Cids()...)

		ss.SetHeight(blks[len(blks)-1].Height())
		at = blks[len(blks)-1].Parents()
	}

	base := blockSet[len(blockSet)-1]
	if base.Equals(known) {
		blockSet = blockSet[:len(blockSet)-1]
		base = blockSet[len(blockSet)-1]
	}

	if base.IsChildOf(known) {
		// common case: receiving blocks that are building on top of our best tipset
		return blockSet, nil
	}

	knownParent, err := syncer.store.LoadTipSet(ctx, known.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to load next local tipset: %w", err)
	}

	if !ignoreCheckpoint {
		if chkpt := syncer.store.GetCheckpoint(); chkpt != nil && base.Height() <= chkpt.Height() {
			for _, b := range incoming.Blocks() {
				syncer.bad.Add(b.Cid(), NewBadBlockReason(incoming.Cids(), "diverges from checkpoint"))
			}
			return nil, xerrors.Errorf("merge point affecting the checkpoint: %w", ErrForkCheckpoint)
		}
	}

	if base.IsChildOf(knownParent) {
		// common case: receiving a block that's potentially part of the same tipset as our best block
		return blockSet, nil
	}

	// We have now ascertained that this is *not* a 'fast forward'
	log.Warnf("(fork detected) synced header chain (%s - %d) does not link to our best block (%s - %d)", incoming.Cids(), incoming.Height(), known.Cids(), known.Height())
	fork, err := syncer.syncFork(ctx, base, known, ignoreCheckpoint)
	if err != nil {
		if errors.Is(err, ErrForkTooLong) || errors.Is(err, ErrForkCheckpoint) {
			// TODO: we're marking this block bad in the same way that we mark invalid blocks bad. Maybe distinguish?
			log.Warn("adding forked chain to our bad tipset cache")
			for _, b := range incoming.Blocks() {
				syncer.bad.Add(b.Cid(), NewBadBlockReason(incoming.Cids(), "fork past finality"))
			}
		}
		return nil, xerrors.Errorf("failed to sync fork: %w", err)
	}

	blockSet = append(blockSet, fork...)

	return blockSet, nil
}

var ErrForkTooLong = fmt.Errorf("fork longer than threshold")
var ErrForkCheckpoint = fmt.Errorf("fork would require us to diverge from checkpointed block")

// syncFork tries to obtain the chain fragment that links a fork into a common
// ancestor in our view of the chain.
//
// If the fork is too long (policy.ChainFinality), or would cause us to diverge from the checkpoint (ErrForkCheckpoint),
// we add the entire subchain to the denylist. Else, we find the common ancestor, and add the missing chain
// fragment until the fork point to the returned []TipSet.
func (syncer *Syncer) syncFork(ctx context.Context, incoming *types.TipSet, known *types.TipSet, ignoreCheckpoint bool) ([]*types.TipSet, error) {

	var chkpt *types.TipSet
	if !ignoreCheckpoint {
		chkpt = syncer.store.GetCheckpoint()
		if known.Equals(chkpt) {
			return nil, ErrForkCheckpoint
		}
	}

	incomingParentsTsk := incoming.Parents()
	commonParent := false
	for _, incomingParent := range incomingParentsTsk.Cids() {
		if known.Contains(incomingParent) {
			commonParent = true
		}
	}

	if commonParent {
		// known contains at least one of incoming's Parents => the common ancestor is known's Parents (incoming's Grandparents)
		// in this case, we need to return {incoming.Parents()}
		incomingParents, err := syncer.store.LoadTipSet(ctx, incomingParentsTsk)
		if err != nil {
			// fallback onto the network
			tips, err := syncer.Exchange.GetBlocks(ctx, incoming.Parents(), 1)
			if err != nil {
				return nil, xerrors.Errorf("failed to fetch incomingParents from the network: %w", err)
			}

			if len(tips) == 0 {
				return nil, xerrors.Errorf("network didn't return any tipsets")
			}

			incomingParents = tips[0]
		}

		return []*types.TipSet{incomingParents}, nil
	}

	// TODO: Does this mean we always ask for ForkLengthThreshold blocks from the network, even if we just need, like, 2? Yes.
	// Would it not be better to ask in smaller chunks, given that an ~ForkLengthThreshold is very rare?
	tips, err := syncer.Exchange.GetBlocks(ctx, incoming.Parents(), int(policy.ChainFinality))
	if err != nil {
		return nil, err
	}

	nts, err := syncer.store.LoadTipSet(ctx, known.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to load next local tipset: %w", err)
	}
	// Track the fork length on our side of the synced chain to enforce
	// `ForkLengthThreshold`. Initialized to 1 because we already walked back
	// one tipset from `known` (our synced head).
	forkLengthInHead := 1

	for cur := 0; cur < len(tips); {
		if nts.Height() == 0 {
			if !syncer.Genesis.Equals(nts) {
				return nil, xerrors.Errorf("somehow synced chain that linked back to a different genesis (bad genesis: %s)", nts.Key())
			}
			return nil, xerrors.Errorf("synced chain forked at genesis, refusing to sync; incoming: %s", incoming.Cids())
		}

		if nts.Equals(tips[cur]) {
			return tips[:cur+1], nil
		}

		if nts.Height() < tips[cur].Height() {
			cur++
		} else {
			// Walk back one block in our synced chain to try to meet the fork's
			// height.
			forkLengthInHead++
			if forkLengthInHead > int(policy.ChainFinality) {
				return nil, ErrForkTooLong
			}

			// We will be forking away from nts, check that it isn't checkpointed
			if nts.Equals(chkpt) {
				return nil, ErrForkCheckpoint
			}

			nts, err = syncer.store.LoadTipSet(ctx, nts.Parents())
			if err != nil {
				return nil, xerrors.Errorf("loading next local tipset: %w", err)
			}
		}
	}

	return nil, ErrForkTooLong
}

func (syncer *Syncer) syncMessagesAndCheckState(ctx context.Context, headers []*types.TipSet) error {
	ss := extractSyncState(ctx)
	ss.SetHeight(headers[len(headers)-1].Height())

	return syncer.iterFullTipsets(ctx, headers, func(ctx context.Context, fts *store.FullTipSet) error {
		log.Debugw("validating tipset", "height", fts.TipSet().Height(), "size", len(fts.TipSet().Cids()))
		if err := syncer.ValidateTipSet(ctx, fts, true); err != nil {
			log.Errorf("failed to validate tipset: %+v", err)
			return xerrors.Errorf("message processing failed: %w", err)
		}

		stats.Record(ctx, metrics.ChainNodeWorkerHeight.M(int64(fts.TipSet().Height())))
		ss.SetHeight(fts.TipSet().Height())

		return nil
	})
}

// fills out each of the given tipsets with messages and calls the callback with it
func (syncer *Syncer) iterFullTipsets(ctx context.Context, headers []*types.TipSet, cb func(context.Context, *store.FullTipSet) error) error {
	ss := extractSyncState(ctx)
	ctx, span := trace.StartSpan(ctx, "iterFullTipsets")
	defer span.End()

	span.AddAttributes(trace.Int64Attribute("num_headers", int64(len(headers))))

	for i := len(headers) - 1; i >= 0; {
		fts, err := syncer.store.TryFillTipSet(ctx, headers[i])
		if err != nil {
			return err
		}
		if fts != nil {
			if err := cb(ctx, fts); err != nil {
				return err
			}
			i--
			continue
		}

		batchSize := concurrentSyncRequests * syncRequestBatchSize
		if i < batchSize {
			batchSize = i + 1
		}

		ss.SetStage(api.StageFetchingMessages)
		startOffset := i + 1 - batchSize
		bstout, batchErr := syncer.fetchMessages(ctx, headers[startOffset:startOffset+batchSize], startOffset)
		ss.SetStage(api.StageMessages)

		if batchErr != nil {
			return xerrors.Errorf("failed to fetch messages: %w", batchErr)
		}

		for bsi := 0; bsi < len(bstout); bsi++ {
			// temp storage so we don't persist data we don't want to
			bs := bstore.NewMemory()
			blks := cbor.NewCborStore(bs)

			this := headers[i-bsi]
			bstip := bstout[len(bstout)-(bsi+1)]
			fts, err := zipTipSetAndMessages(blks, this, bstip.Bls, bstip.Secpk, bstip.BlsIncludes, bstip.SecpkIncludes)
			if err != nil {
				log.Warnw("zipping failed", "error", err, "bsi", bsi, "i", i,
					"height", this.Height(),
					"next-height", i+batchSize)
				return xerrors.Errorf("message processing failed: %w", err)
			}

			if err := cb(ctx, fts); err != nil {
				return err
			}

			if err := persistMessages(ctx, bs, bstip); err != nil {
				return err
			}

			if err := copyBlockstore(ctx, bs, syncer.store.ChainBlockstore()); err != nil {
				return xerrors.Errorf("message processing failed: %w", err)
			}
		}

		i -= batchSize
	}

	return nil
}

func checkMsgMeta(ts *types.TipSet, allbmsgs []*types.Message, allsmsgs []*types.SignedMessage, bmi, smi [][]uint64) error {
	for bi, b := range ts.Blocks() {
		if msgc := len(bmi[bi]) + len(smi[bi]); msgc > buildconstants.BlockMessageLimit {
			return fmt.Errorf("block %q has too many messages (%d)", b.Cid(), msgc)
		}

		var smsgCids []cid.Cid
		for _, m := range smi[bi] {
			smsgCids = append(smsgCids, allsmsgs[m].Cid())
		}

		var bmsgCids []cid.Cid
		for _, m := range bmi[bi] {
			bmsgCids = append(bmsgCids, allbmsgs[m].Cid())
		}

		mrcid, err := computeMsgMeta(cbor.NewCborStore(bstore.NewMemory()), bmsgCids, smsgCids)
		if err != nil {
			return err
		}

		if b.Messages != mrcid {
			return fmt.Errorf("messages didnt match message root in header for ts %s", ts.Key())
		}
	}

	return nil
}

func (syncer *Syncer) fetchMessages(ctx context.Context, headers []*types.TipSet, startOffset int) ([]*exchange.CompactedMessages, error) {
	batchSize := len(headers)
	batch := make([]*exchange.CompactedMessages, batchSize)

	var wg sync.WaitGroup
	var mx sync.Mutex
	var batchErr error

	start := build.Clock.Now()

	for j := 0; j < batchSize; j += syncRequestBatchSize {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()

			nreq := syncRequestBatchSize
			if j+nreq > batchSize {
				nreq = batchSize - j
			}

			failed := false
			for offset := 0; !failed && offset < nreq; {
				nextI := j + offset
				lastI := j + nreq

				var requestErr error
				var requestResult []*exchange.CompactedMessages
				for retry := 0; requestResult == nil && retry < syncRequestRetries; retry++ {
					if retry > 0 {
						log.Infof("fetching messages at %d (retry %d)", startOffset+nextI, retry)
					} else {
						log.Infof("fetching messages at %d", startOffset+nextI)
					}

					result, err := syncer.Exchange.GetChainMessages(ctx, headers[nextI:lastI])
					if err != nil {
						requestErr = multierror.Append(requestErr, err)
					} else {
						isGood := true
						for index, cm := range result {
							ts := headers[nextI+index]
							if err := checkMsgMeta(ts, cm.Bls, cm.Secpk, cm.BlsIncludes, cm.SecpkIncludes); err != nil {
								log.Errorf("fetched messages not as expected: %s", err)
								isGood = false
								break
							}
						}

						if isGood {
							requestResult = result
						}
					}
				}

				mx.Lock()
				if requestResult != nil {
					copy(batch[j+offset:], requestResult)
					offset += len(requestResult)
				} else {
					log.Errorf("error fetching messages at %d: %s", nextI, requestErr)
					batchErr = multierror.Append(batchErr, requestErr)
					failed = true
				}
				mx.Unlock()
			}
		}(j)
	}
	wg.Wait()

	if batchErr != nil {
		return nil, batchErr
	}

	log.Infof("fetching messages for %d tipsets at %d done; took %s", batchSize, startOffset, build.Clock.Since(start))

	return batch, nil
}

func persistMessages(ctx context.Context, bs bstore.Blockstore, bst *exchange.CompactedMessages) error {
	_, span := trace.StartSpan(ctx, "persistMessages")
	defer span.End()

	for _, m := range bst.Bls {
		//log.Infof("putting BLS message: %s", m.Cid())
		if _, err := store.PutMessage(ctx, bs, m); err != nil {
			log.Errorf("failed to persist messages: %+v", err)
			return xerrors.Errorf("BLS message processing failed: %w", err)
		}
	}
	for _, m := range bst.Secpk {
		if m.Signature.Type != crypto.SigTypeSecp256k1 && m.Signature.Type != crypto.SigTypeDelegated {
			return xerrors.Errorf("unknown signature type on message %s: %q", m.Cid(), m.Signature.Type)
		}
		//log.Infof("putting secp256k1 message: %s", m.Cid())
		if _, err := store.PutMessage(ctx, bs, m); err != nil {
			log.Errorf("failed to persist messages: %+v", err)
			return xerrors.Errorf("secp256k1 message processing failed: %w", err)
		}
	}

	return nil
}

// collectChain tries to advance our view of the chain to the purported head.
//
// It goes through various stages:
//
//  1. StageHeaders: we proceed in the sync process by requesting block headers
//     from our peers, moving back from their heads, until we reach a tipset
//     that we have in common (such a common tipset must exist, thought it may
//     simply be the genesis block).
//
//     If the common tipset is our head, we treat the sync as a "fast-forward",
//     else we must drop part of our chain to connect to the peer's head
//     (referred to as "forking").
//
//  2. StagePersistHeaders: now that we've collected the missing headers,
//     augmented by those on the other side of a fork, we persist them to the
//     BlockStore.
//
//  3. StageMessages: having acquired the headers and found a common tipset,
//     we then move forward, requesting the full blocks, including the messages.
func (syncer *Syncer) collectChain(ctx context.Context, ts *types.TipSet, hts *types.TipSet, ignoreCheckpoint bool) error {
	ctx, span := trace.StartSpan(ctx, "collectChain")
	defer span.End()
	ss := extractSyncState(ctx)

	ss.Init(hts, ts)

	headers, err := syncer.collectHeaders(ctx, ts, hts, ignoreCheckpoint)
	if err != nil {
		ss.Error(err)
		return err
	}

	span.AddAttributes(trace.Int64Attribute("syncChainLength", int64(len(headers))))

	if !headers[0].Equals(ts) {
		return xerrors.Errorf("collectChain synced %s, wanted to sync %s", headers[0].Cids(), ts.Cids())
	}

	ss.SetStage(api.StagePersistHeaders)

	// Write tipsets from oldest to newest.
	if err := syncer.store.PersistTipsets(ctx, headers); err != nil {
		err = xerrors.Errorf("failed to persist synced tipset to the chainstore: %w", err)
		ss.Error(err)
		return err
	}

	ss.SetStage(api.StageMessages)

	if err := syncer.syncMessagesAndCheckState(ctx, headers); err != nil {
		err = xerrors.Errorf("collectChain syncMessages: %w", err)
		ss.Error(err)
		return err
	}

	ss.SetStage(api.StageSyncComplete)
	log.Debugw("new tipset", "height", ts.Height(), "tipset", types.LogCids(ts.Cids()))

	return nil
}

func (syncer *Syncer) State() []SyncerStateSnapshot {
	return syncer.syncmgr.State()
}

// MarkBad manually adds a block to the "bad blocks" cache.
func (syncer *Syncer) MarkBad(blk cid.Cid) {
	syncer.bad.Add(blk, NewBadBlockReason([]cid.Cid{blk}, "manually marked bad"))
}

// UnmarkBad manually adds a block to the "bad blocks" cache.
func (syncer *Syncer) UnmarkBad(blk cid.Cid) {
	syncer.bad.Remove(blk)
}

func (syncer *Syncer) UnmarkAllBad() {
	syncer.bad.Purge()
}

func (syncer *Syncer) CheckBadBlockCache(blk cid.Cid) (string, bool) {
	bbr, ok := syncer.bad.Has(blk)
	return bbr.String(), ok
}

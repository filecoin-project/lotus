package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Gurpartap/async"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"github.com/whyrusleeping/pubsub"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	blst "github.com/supranational/blst/bindings/go"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/lib/sigs/bls"
	"github.com/filecoin-project/lotus/metrics"
)

// Blocks that are more than MaxHeightDrift epochs above
//the theoretical max height based on systime are quickly rejected
const MaxHeightDrift = 5

var log = logging.Logger("chain")

var LocalIncoming = "incoming"

// Syncer is in charge of running the chain synchronization logic. As such, it
// is tasked with these functions, amongst others:
//
//  * Fast-forwards the chain as it learns of new TipSets from the network via
//    the SyncManager.
//  * Applies the fork choice rule to select the correct side when confronted
//    with a fork in the network.
//  * Requests block headers and messages from other peers when not available
//    in our BlockStore.
//  * Tracks blocks marked as bad in a cache.
//  * Keeps the BlockStore and ChainStore consistent with our view of the world,
//    the latter of which in turn informs other components when a reorg has been
//    committed.
//
// The Syncer does not run workers itself. It's mainly concerned with
// ensuring a consistent state of chain consensus. The reactive and network-
// interfacing processes are part of other components, such as the SyncManager
// (which owns the sync scheduler and sync workers), BlockSync, the HELLO
// protocol, and the gossipsub block propagation layer.
//
// {hint/concept} The fork-choice rule as it currently stands is: "pick the
// chain with the heaviest weight, so long as it hasn’t deviated one finality
// threshold from our head (900 epochs, parameter determined by spec-actors)".
type Syncer struct {
	// The interface for accessing and putting tipsets into local storage
	store *store.ChainStore

	// handle to the random beacon for verification
	beacon beacon.RandomBeacon

	// the state manager handles making state queries
	sm *stmgr.StateManager

	// The known Genesis tipset
	Genesis *types.TipSet

	// TipSets known to be invalid
	bad *BadBlockCache

	// handle to the block sync service
	Bsync *blocksync.BlockSync

	self peer.ID

	syncmgr *SyncManager

	connmgr connmgr.ConnManager

	incoming *pubsub.PubSub

	receiptTracker *blockReceiptTracker

	verifier ffiwrapper.Verifier
}

// NewSyncer creates a new Syncer object.
func NewSyncer(sm *stmgr.StateManager, bsync *blocksync.BlockSync, connmgr connmgr.ConnManager, self peer.ID, beacon beacon.RandomBeacon, verifier ffiwrapper.Verifier) (*Syncer, error) {
	gen, err := sm.ChainStore().GetGenesis()
	if err != nil {
		return nil, xerrors.Errorf("getting genesis block: %w", err)
	}

	gent, err := types.NewTipSet([]*types.BlockHeader{gen})
	if err != nil {
		return nil, err
	}

	s := &Syncer{
		beacon:         beacon,
		bad:            NewBadBlockCache(),
		Genesis:        gent,
		Bsync:          bsync,
		store:          sm.ChainStore(),
		sm:             sm,
		self:           self,
		receiptTracker: newBlockReceiptTracker(),
		connmgr:        connmgr,
		verifier:       verifier,

		incoming: pubsub.New(50),
	}

	if build.InsecurePoStValidation {
		log.Warn("*********************************************************************************************")
		log.Warn(" [INSECURE-POST-VALIDATION] Insecure test validation is enabled. If you see this outside of a test, it is a severe bug! ")
		log.Warn("*********************************************************************************************")
	}

	s.syncmgr = NewSyncManager(s.Sync)
	return s, nil
}

func (syncer *Syncer) Start() {
	syncer.syncmgr.Start()
}

func (syncer *Syncer) Stop() {
	syncer.syncmgr.Stop()
}

// InformNewHead informs the syncer about a new potential tipset
// This should be called when connecting to new peers, and additionally
// when receiving new blocks from the network
func (syncer *Syncer) InformNewHead(from peer.ID, fts *store.FullTipSet) bool {
	ctx := context.Background()
	if fts == nil {
		log.Errorf("got nil tipset in InformNewHead")
		return false
	}

	if syncer.IsEpochBeyondCurrMax(fts.TipSet().Height()) {
		log.Errorf("Received block with impossibly large height %d", fts.TipSet().Height())
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

	if from == syncer.self {
		// TODO: this is kindof a hack...
		log.Debug("got block from ourselves")

		if err := syncer.Sync(ctx, fts.TipSet()); err != nil {
			log.Errorf("failed to sync our own block %s: %+v", fts.TipSet().Cids(), err)
			return false
		}

		return true
	}

	// TODO: IMPORTANT(GARBAGE) this needs to be put in the 'temporary' side of
	// the blockstore
	if err := syncer.store.PersistBlockHeaders(fts.TipSet().Blocks()...); err != nil {
		log.Warn("failed to persist incoming block header: ", err)
		return false
	}

	syncer.Bsync.AddPeer(from)

	bestPweight := syncer.store.GetHeaviestTipSet().ParentWeight()
	targetWeight := fts.TipSet().ParentWeight()
	if targetWeight.LessThan(bestPweight) {
		var miners []string
		for _, blk := range fts.TipSet().Blocks() {
			miners = append(miners, blk.Miner.String())
		}
		log.Infof("incoming tipset from %s does not appear to be better than our best chain, ignoring for now", miners)
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
		defer syncer.incoming.Unsub(sub, LocalIncoming)

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
	if msgc := len(fblk.BlsMessages) + len(fblk.SecpkMessages); msgc > build.BlockMessageLimit {
		return xerrors.Errorf("block %s has too many messages (%d)", fblk.Header.Cid(), msgc)
	}

	// Collect the CIDs of both types of messages separately: BLS and Secpk.
	var bcids, scids []cid.Cid
	for _, m := range fblk.BlsMessages {
		bcids = append(bcids, m.Cid())
	}

	for _, m := range fblk.SecpkMessages {
		scids = append(scids, m.Cid())
	}

	// TODO: IMPORTANT(GARBAGE). These message puts and the msgmeta
	// computation need to go into the 'temporary' side of the blockstore when
	// we implement that
	blockstore := syncer.store.Blockstore()

	bs := cbor.NewCborStore(blockstore)

	// Compute the root CID of the combined message trie.
	smroot, err := computeMsgMeta(bs, bcids, scids)
	if err != nil {
		return xerrors.Errorf("validating msgmeta, compute failed: %w", err)
	}

	// Check that the message trie root matches with what's in the block.
	if fblk.Header.Messages != smroot {
		return xerrors.Errorf("messages in full block did not match msgmeta root in header (%s != %s)", fblk.Header.Messages, smroot)
	}

	for _, m := range fblk.BlsMessages {
		_, err := store.PutMessage(blockstore, m)
		if err != nil {
			return xerrors.Errorf("putting bls message to blockstore after msgmeta computation: %w", err)
		}
	}

	for _, m := range fblk.SecpkMessages {
		_, err := store.PutMessage(blockstore, m)
		if err != nil {
			return xerrors.Errorf("putting bls message to blockstore after msgmeta computation: %w", err)
		}
	}

	return nil
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

func copyBlockstore(from, to bstore.Blockstore) error {
	cids, err := from.AllKeysChan(context.TODO())
	if err != nil {
		return err
	}

	for c := range cids {
		b, err := from.Get(c)
		if err != nil {
			return err
		}

		if err := to.Put(b); err != nil {
			return err
		}
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

	fts := &store.FullTipSet{}
	for bi, b := range ts.Blocks() {
		if msgc := len(bmi[bi]) + len(smi[bi]); msgc > build.BlockMessageLimit {
			return nil, fmt.Errorf("block %q has too many messages (%d)", b.Cid(), msgc)
		}

		var smsgs []*types.SignedMessage
		var smsgCids []cid.Cid
		for _, m := range smi[bi] {
			smsgs = append(smsgs, allsmsgs[m])
			smsgCids = append(smsgCids, allsmsgs[m].Cid())
		}

		var bmsgs []*types.Message
		var bmsgCids []cid.Cid
		for _, m := range bmi[bi] {
			bmsgs = append(bmsgs, allbmsgs[m])
			bmsgCids = append(bmsgCids, allbmsgs[m].Cid())
		}

		mrcid, err := computeMsgMeta(bs, bmsgCids, smsgCids)
		if err != nil {
			return nil, err
		}

		if b.Messages != mrcid {
			return nil, fmt.Errorf("messages didnt match message root in header for ts %s", ts.Key())
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
	store := adt.WrapStore(context.TODO(), bs)
	bmArr := adt.MakeEmptyArray(store)
	smArr := adt.MakeEmptyArray(store)

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
// to the network (BlockSync) by querying the supplied peer if not found
// locally.
//
// {hint/usage} This is used from the HELLO protocol, to fetch the greeting
// peer's heaviest tipset if we don't have it.
func (syncer *Syncer) FetchTipSet(ctx context.Context, p peer.ID, tsk types.TipSetKey) (*store.FullTipSet, error) {
	if fts, err := syncer.tryLoadFullTipSet(tsk); err == nil {
		return fts, nil
	}

	// fall back to the network.
	return syncer.Bsync.GetFullTipSet(ctx, p, tsk)
}

// tryLoadFullTipSet queries the tipset in the ChainStore, and returns a full
// representation of it containing FullBlocks. If ALL blocks are not found
// locally, it errors entirely with blockstore.ErrNotFound.
func (syncer *Syncer) tryLoadFullTipSet(tsk types.TipSetKey) (*store.FullTipSet, error) {
	ts, err := syncer.store.LoadTipSet(tsk)
	if err != nil {
		return nil, err
	}

	fts := &store.FullTipSet{}
	for _, b := range ts.Blocks() {
		bmsgs, smsgs, err := syncer.store.MessagesForBlock(b)
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

	if syncer.store.GetHeaviestTipSet().ParentWeight().GreaterThan(maybeHead.ParentWeight()) {
		return nil
	}

	if syncer.Genesis.Equals(maybeHead) || syncer.store.GetHeaviestTipSet().Equals(maybeHead) {
		return nil
	}

	if err := syncer.collectChain(ctx, maybeHead); err != nil {
		span.AddAttributes(trace.StringAttribute("col_error", err.Error()))
		span.SetStatus(trace.Status{
			Code:    13,
			Message: err.Error(),
		})
		return xerrors.Errorf("collectChain failed: %w", err)
	}

	// At this point we have accepted and synced to the new `maybeHead`
	// (`StageSyncComplete`).
	if err := syncer.store.PutTipSet(ctx, maybeHead); err != nil {
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
	return !errors.Is(err, ErrTemporal)
}

func (syncer *Syncer) ValidateTipSet(ctx context.Context, fts *store.FullTipSet) error {
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
			if err := syncer.ValidateBlock(ctx, b); err != nil {
				if isPermanent(err) {
					syncer.bad.Add(b.Cid(), NewBadBlockReason([]cid.Cid{b.Cid()}, err.Error()))
				}
				return xerrors.Errorf("validating block %s: %w", b.Cid(), err)
			}

			if err := syncer.sm.ChainStore().AddToTipSetTracker(b.Header); err != nil {
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

func (syncer *Syncer) minerIsValid(ctx context.Context, maddr address.Address, baseTs *types.TipSet) error {
	var spast power.State

	_, err := syncer.sm.LoadActorState(ctx, builtin.StoragePowerActorAddr, &spast, baseTs)
	if err != nil {
		return err
	}

	cm, err := adt.AsMap(syncer.store.Store(ctx), spast.Claims)
	if err != nil {
		return err
	}

	var claim power.Claim
	exist, err := cm.Get(adt.AddrKey(maddr), &claim)
	if err != nil {
		return err
	}
	if !exist {
		return xerrors.New("miner isn't valid")
	}
	return nil
}

var ErrTemporal = errors.New("temporal error")

func blockSanityChecks(h *types.BlockHeader) error {
	if h.ElectionProof == nil {
		return xerrors.Errorf("block cannot have nil election proof")
	}

	if h.Ticket == nil {
		return xerrors.Errorf("block cannot have nil ticket")
	}

	if h.BlockSig == nil {
		return xerrors.Errorf("block had nil signature")
	}

	if h.BLSAggregate == nil {
		return xerrors.Errorf("block had nil bls aggregate signature")
	}

	return nil
}

// ValidateBlock should match up with 'Semantical Validation' in validation.md in the spec
func (syncer *Syncer) ValidateBlock(ctx context.Context, b *types.FullBlock) (err error) {
	defer func() {
		// b.Cid() could panic for empty blocks that are used in tests.
		if rerr := recover(); rerr != nil {
			err = xerrors.Errorf("validate block panic: %w", rerr)
			return
		}
	}()

	isValidated, err := syncer.store.IsBlockValidated(ctx, b.Cid())
	if err != nil {
		return xerrors.Errorf("check block validation cache %s: %w", b.Cid(), err)
	}

	if isValidated {
		return nil
	}

	validationStart := build.Clock.Now()
	defer func() {
		stats.Record(ctx, metrics.BlockValidationDurationMilliseconds.M(metrics.SinceInMilliseconds(validationStart)))
		log.Infow("block validation", "took", time.Since(validationStart), "height", b.Header.Height)
	}()

	ctx, span := trace.StartSpan(ctx, "validateBlock")
	defer span.End()

	if err := blockSanityChecks(b.Header); err != nil {
		return xerrors.Errorf("incoming header failed basic sanity checks: %w", err)
	}

	h := b.Header

	baseTs, err := syncer.store.LoadTipSet(types.NewTipSetKey(h.Parents...))
	if err != nil {
		return xerrors.Errorf("load parent tipset failed (%s): %w", h.Parents, err)
	}

	lbts, err := stmgr.GetLookbackTipSetForRound(ctx, syncer.sm, baseTs, h.Height)
	if err != nil {
		return xerrors.Errorf("failed to get lookback tipset for block: %w", err)
	}

	lbst, _, err := syncer.sm.TipSetState(ctx, lbts)
	if err != nil {
		return xerrors.Errorf("failed to compute lookback tipset state: %w", err)
	}

	prevBeacon, err := syncer.store.GetLatestBeaconEntry(baseTs)
	if err != nil {
		return xerrors.Errorf("failed to get latest beacon entry: %w", err)
	}

	// fast checks first
	nulls := h.Height - (baseTs.Height() + 1)
	if tgtTs := baseTs.MinTimestamp() + build.BlockDelaySecs*uint64(nulls+1); h.Timestamp != tgtTs {
		return xerrors.Errorf("block has wrong timestamp: %d != %d", h.Timestamp, tgtTs)
	}

	now := uint64(build.Clock.Now().Unix())
	if h.Timestamp > now+build.AllowableClockDriftSecs {
		return xerrors.Errorf("block was from the future (now=%d, blk=%d): %w", now, h.Timestamp, ErrTemporal)
	}
	if h.Timestamp > now {
		log.Warn("Got block from the future, but within threshold", h.Timestamp, build.Clock.Now().Unix())
	}

	msgsCheck := async.Err(func() error {
		if err := syncer.checkBlockMessages(ctx, b, baseTs); err != nil {
			return xerrors.Errorf("block had invalid messages: %w", err)
		}
		return nil
	})

	minerCheck := async.Err(func() error {
		if err := syncer.minerIsValid(ctx, h.Miner, baseTs); err != nil {
			return xerrors.Errorf("minerIsValid failed: %w", err)
		}
		return nil
	})

	baseFeeCheck := async.Err(func() error {
		baseFee, err := syncer.store.ComputeBaseFee(ctx, baseTs)
		if err != nil {
			return xerrors.Errorf("computing base fee: %w", err)
		}
		if types.BigCmp(baseFee, b.Header.ParentBaseFee) != 0 {
			return xerrors.Errorf("base fee doesn't match: %s (header) != %s (computed)",
				b.Header.ParentBaseFee, baseFee)
		}
		return nil
	})
	pweight, err := syncer.store.Weight(ctx, baseTs)
	if err != nil {
		return xerrors.Errorf("getting parent weight: %w", err)
	}

	if types.BigCmp(pweight, b.Header.ParentWeight) != 0 {
		return xerrors.Errorf("parrent weight different: %s (header) != %s (computed)",
			b.Header.ParentWeight, pweight)
	}

	// Stuff that needs stateroot / worker address
	stateroot, precp, err := syncer.sm.TipSetState(ctx, baseTs)
	if err != nil {
		return xerrors.Errorf("get tipsetstate(%d, %s) failed: %w", h.Height, h.Parents, err)
	}

	if stateroot != h.ParentStateRoot {
		msgs, err := syncer.store.MessagesForTipset(baseTs)
		if err != nil {
			log.Error("failed to load messages for tipset during tipset state mismatch error: ", err)
		} else {
			log.Warn("Messages for tipset with mismatching state:")
			for i, m := range msgs {
				mm := m.VMMessage()
				log.Warnf("Message[%d]: from=%s to=%s method=%d params=%x", i, mm.From, mm.To, mm.Method, mm.Params)
			}
		}

		return xerrors.Errorf("parent state root did not match computed state (%s != %s)", stateroot, h.ParentStateRoot)
	}

	if precp != h.ParentMessageReceipts {
		return xerrors.Errorf("parent receipts root did not match computed value (%s != %s)", precp, h.ParentMessageReceipts)
	}

	waddr, err := stmgr.GetMinerWorkerRaw(ctx, syncer.sm, lbst, h.Miner)
	if err != nil {
		return xerrors.Errorf("GetMinerWorkerRaw failed: %w", err)
	}

	winnerCheck := async.Err(func() error {
		if h.ElectionProof.WinCount < 1 {
			return xerrors.Errorf("block is not claiming to be a winner")
		}

		hp, err := stmgr.MinerHasMinPower(ctx, syncer.sm, h.Miner, lbts)
		if err != nil {
			return xerrors.Errorf("determining if miner has min power failed: %w", err)
		}

		if !hp {
			return xerrors.New("block's miner does not meet minimum power threshold")
		}

		rBeacon := *prevBeacon
		if len(h.BeaconEntries) != 0 {
			rBeacon = h.BeaconEntries[len(h.BeaconEntries)-1]
		}
		buf := new(bytes.Buffer)
		if err := h.Miner.MarshalCBOR(buf); err != nil {
			return xerrors.Errorf("failed to marshal miner address to cbor: %w", err)
		}

		vrfBase, err := store.DrawRandomness(rBeacon.Data, crypto.DomainSeparationTag_ElectionProofProduction, h.Height, buf.Bytes())
		if err != nil {
			return xerrors.Errorf("could not draw randomness: %w", err)
		}

		if err := VerifyElectionPoStVRF(ctx, waddr, vrfBase, h.ElectionProof.VRFProof); err != nil {
			return xerrors.Errorf("validating block election proof failed: %w", err)
		}

		slashed, err := stmgr.GetMinerSlashed(ctx, syncer.sm, baseTs, h.Miner)
		if err != nil {
			return xerrors.Errorf("failed to check if block miner was slashed: %w", err)
		}

		if slashed {
			return xerrors.Errorf("received block was from slashed or invalid miner")
		}

		mpow, tpow, err := stmgr.GetPowerRaw(ctx, syncer.sm, lbst, h.Miner)
		if err != nil {
			return xerrors.Errorf("failed getting power: %w", err)
		}

		j := h.ElectionProof.ComputeWinCount(mpow.QualityAdjPower, tpow.QualityAdjPower)
		if h.ElectionProof.WinCount != j {
			return xerrors.Errorf("miner claims wrong number of wins: miner: %d, computed: %d", h.ElectionProof.WinCount, j)
		}

		return nil
	})

	blockSigCheck := async.Err(func() error {
		if err := sigs.CheckBlockSignature(ctx, h, waddr); err != nil {
			return xerrors.Errorf("check block signature failed: %w", err)
		}
		return nil
	})

	beaconValuesCheck := async.Err(func() error {
		if os.Getenv("LOTUS_IGNORE_DRAND") == "_yes_" {
			return nil
		}

		if err := beacon.ValidateBlockValues(syncer.beacon, h, *prevBeacon); err != nil {
			return xerrors.Errorf("failed to validate blocks random beacon values: %w", err)
		}
		return nil
	})

	tktsCheck := async.Err(func() error {
		buf := new(bytes.Buffer)
		if err := h.Miner.MarshalCBOR(buf); err != nil {
			return xerrors.Errorf("failed to marshal miner address to cbor: %w", err)
		}

		beaconBase := *prevBeacon
		if len(h.BeaconEntries) == 0 {
			buf.Write(baseTs.MinTicket().VRFProof)
		} else {
			beaconBase = h.BeaconEntries[len(h.BeaconEntries)-1]
		}

		vrfBase, err := store.DrawRandomness(beaconBase.Data, crypto.DomainSeparationTag_TicketProduction, h.Height-build.TicketRandomnessLookback, buf.Bytes())
		if err != nil {
			return xerrors.Errorf("failed to compute vrf base for ticket: %w", err)
		}

		err = VerifyElectionPoStVRF(ctx, waddr, vrfBase, h.Ticket.VRFProof)
		if err != nil {
			return xerrors.Errorf("validating block tickets failed: %w", err)
		}
		return nil
	})

	wproofCheck := async.Err(func() error {
		if err := syncer.VerifyWinningPoStProof(ctx, h, *prevBeacon, lbst, waddr); err != nil {
			return xerrors.Errorf("invalid election post: %w", err)
		}
		return nil
	})

	await := []async.ErrorFuture{
		minerCheck,
		tktsCheck,
		blockSigCheck,
		beaconValuesCheck,
		wproofCheck,
		winnerCheck,
		msgsCheck,
		baseFeeCheck,
	}

	var merr error
	for _, fut := range await {
		if err := fut.AwaitContext(ctx); err != nil {
			merr = multierror.Append(merr, err)
		}
	}
	if merr != nil {
		mulErr := merr.(*multierror.Error)
		mulErr.ErrorFormat = func(es []error) string {
			if len(es) == 1 {
				return fmt.Sprintf("1 error occurred:\n\t* %+v\n\n", es[0])
			}

			points := make([]string, len(es))
			for i, err := range es {
				points[i] = fmt.Sprintf("* %+v", err)
			}

			return fmt.Sprintf(
				"%d errors occurred:\n\t%s\n\n",
				len(es), strings.Join(points, "\n\t"))
		}
		return mulErr
	}

	if err := syncer.store.MarkBlockAsValidated(ctx, b.Cid()); err != nil {
		return xerrors.Errorf("caching block validation %s: %w", b.Cid(), err)
	}

	return nil
}

func (syncer *Syncer) VerifyWinningPoStProof(ctx context.Context, h *types.BlockHeader, prevBeacon types.BeaconEntry, lbst cid.Cid, waddr address.Address) error {
	if build.InsecurePoStValidation {
		if len(h.WinPoStProof) == 0 {
			return xerrors.Errorf("[INSECURE-POST-VALIDATION] No winning post proof given")
		}

		if string(h.WinPoStProof[0].ProofBytes) == "valid proof" {
			return nil
		}
		return xerrors.Errorf("[INSECURE-POST-VALIDATION] winning post was invalid")
	}

	buf := new(bytes.Buffer)
	if err := h.Miner.MarshalCBOR(buf); err != nil {
		return xerrors.Errorf("failed to marshal miner address: %w", err)
	}

	rbase := prevBeacon
	if len(h.BeaconEntries) > 0 {
		rbase = h.BeaconEntries[len(h.BeaconEntries)-1]
	}

	rand, err := store.DrawRandomness(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, h.Height, buf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to get randomness for verifying winningPost proof: %w", err)
	}

	mid, err := address.IDFromAddress(h.Miner)
	if err != nil {
		return xerrors.Errorf("failed to get ID from miner address %s: %w", h.Miner, err)
	}

	sectors, err := stmgr.GetSectorsForWinningPoSt(ctx, syncer.verifier, syncer.sm, lbst, h.Miner, rand)
	if err != nil {
		return xerrors.Errorf("getting winning post sector set: %w", err)
	}

	ok, err := ffiwrapper.ProofVerifier.VerifyWinningPoSt(ctx, abi.WinningPoStVerifyInfo{
		Randomness:        rand,
		Proofs:            h.WinPoStProof,
		ChallengedSectors: sectors,
		Prover:            abi.ActorID(mid),
	})
	if err != nil {
		return xerrors.Errorf("failed to verify election post: %w", err)
	}

	if !ok {
		log.Errorf("invalid winning post (block: %s, %x; %v)", h.Cid(), rand, sectors)
		return xerrors.Errorf("winning post was invalid")
	}

	return nil
}

// TODO: We should extract this somewhere else and make the message pool and miner use the same logic
func (syncer *Syncer) checkBlockMessages(ctx context.Context, b *types.FullBlock, baseTs *types.TipSet) error {
	{
		var sigCids []cid.Cid // this is what we get for people not wanting the marshalcbor method on the cid type
		var pubks [][]byte

		for _, m := range b.BlsMessages {
			sigCids = append(sigCids, m.Cid())

			pubk, err := syncer.sm.GetBlsPublicKey(ctx, m.From, baseTs)
			if err != nil {
				return xerrors.Errorf("failed to load bls public to validate block: %w", err)
			}

			pubks = append(pubks, pubk)
		}

		if err := syncer.verifyBlsAggregate(ctx, b.Header.BLSAggregate, sigCids, pubks); err != nil {
			return xerrors.Errorf("bls aggregate signature was invalid: %w", err)
		}
	}

	nonces := make(map[address.Address]uint64)

	stateroot, _, err := syncer.sm.TipSetState(ctx, baseTs)
	if err != nil {
		return err
	}

	cst := cbor.NewCborStore(syncer.store.Blockstore())
	st, err := state.LoadStateTree(cst, stateroot)
	if err != nil {
		return xerrors.Errorf("failed to load base state tree: %w", err)
	}

	pl := vm.PricelistByEpoch(baseTs.Height())
	var sumGasLimit int64
	checkMsg := func(msg types.ChainMsg) error {
		m := msg.VMMessage()

		// Phase 1: syntactic validation, as defined in the spec
		minGas := pl.OnChainMessage(msg.ChainLength())
		if err := m.ValidForBlockInclusion(minGas.Total()); err != nil {
			return err
		}

		// ValidForBlockInclusion checks if any single message does not exceed BlockGasLimit
		// So below is overflow safe
		sumGasLimit += m.GasLimit
		if sumGasLimit > build.BlockGasLimit {
			return xerrors.Errorf("block gas limit exceeded")
		}

		// Phase 2: (Partial) semantic validation:
		// the sender exists and is an account actor, and the nonces make sense
		if _, ok := nonces[m.From]; !ok {
			// `GetActor` does not validate that this is an account actor.
			act, err := st.GetActor(m.From)
			if err != nil {
				return xerrors.Errorf("failed to get actor: %w", err)
			}

			if !act.IsAccountActor() {
				return xerrors.New("Sender must be an account actor")
			}
			nonces[m.From] = act.Nonce
		}

		if nonces[m.From] != m.Nonce {
			return xerrors.Errorf("wrong nonce (exp: %d, got: %d)", nonces[m.From], m.Nonce)
		}
		nonces[m.From]++

		return nil
	}

	store := adt.WrapStore(ctx, cst)

	bmArr := adt.MakeEmptyArray(store)
	for i, m := range b.BlsMessages {
		if err := checkMsg(m); err != nil {
			return xerrors.Errorf("block had invalid bls message at index %d: %w", i, err)
		}

		c := cbg.CborCid(m.Cid())
		if err := bmArr.Set(uint64(i), &c); err != nil {
			return xerrors.Errorf("failed to put bls message at index %d: %w", i, err)
		}
	}

	smArr := adt.MakeEmptyArray(store)
	for i, m := range b.SecpkMessages {
		if err := checkMsg(m); err != nil {
			return xerrors.Errorf("block had invalid secpk message at index %d: %w", i, err)
		}

		// `From` being an account actor is only validated inside the `vm.ResolveToKeyAddr` call
		// in `StateManager.ResolveToKeyAddress` here (and not in `checkMsg`).
		kaddr, err := syncer.sm.ResolveToKeyAddress(ctx, m.Message.From, baseTs)
		if err != nil {
			return xerrors.Errorf("failed to resolve key addr: %w", err)
		}

		if err := sigs.Verify(&m.Signature, kaddr, m.Message.Cid().Bytes()); err != nil {
			return xerrors.Errorf("secpk message %s has invalid signature: %w", m.Cid(), err)
		}

		c := cbg.CborCid(m.Cid())
		if err := smArr.Set(uint64(i), &c); err != nil {
			return xerrors.Errorf("failed to put secpk message at index %d: %w", i, err)
		}
	}

	bmroot, err := bmArr.Root()
	if err != nil {
		return err
	}

	smroot, err := smArr.Root()
	if err != nil {
		return err
	}

	mrcid, err := cst.Put(ctx, &types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})
	if err != nil {
		return err
	}

	if b.Header.Messages != mrcid {
		return fmt.Errorf("messages didnt match message root in header")
	}

	return nil
}

func (syncer *Syncer) verifyBlsAggregate(ctx context.Context, sig *crypto.Signature, msgs []cid.Cid, pubks [][]byte) error {
	_, span := trace.StartSpan(ctx, "syncer.verifyBlsAggregate")
	defer span.End()
	span.AddAttributes(
		trace.Int64Attribute("msgCount", int64(len(msgs))),
	)

	msgsS := make([]blst.Message, len(msgs))
	for i := 0; i < len(msgs); i++ {
		msgsS[i] = msgs[i].Bytes()
	}

	if len(msgs) == 0 {
		return nil
	}

	valid := new(bls.Signature).AggregateVerifyCompressed(sig.Data, pubks,
		msgsS, []byte(bls.DST))
	if !valid {
		return xerrors.New("bls aggregate signature failed to verify")
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
//  	3a. Load it from the chainstore; if found, it move on to its parent.
//      3b. Query our peers via BlockSync in batches, requesting up to a
//      maximum of 500 tipsets every time.
//
// Once we've concluded, if we find a mismatching tipset at the height where the
// anchor tipset should be, we are facing a fork, and we invoke Syncer#syncFork
// to resolve it. Refer to the godocs there.
//
// All throughout the process, we keep checking if the received blocks are in
// the deny list, and short-circuit the process if so.
func (syncer *Syncer) collectHeaders(ctx context.Context, incoming *types.TipSet, known *types.TipSet) ([]*types.TipSet, error) {
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
		// ensure consistency of beacon entires
		targetBE := incoming.Blocks()[0].BeaconEntries
		sorted := sort.SliceIsSorted(targetBE, func(i, j int) bool {
			return targetBE[i].Round < targetBE[j].Round
		})
		if !sorted {
			syncer.bad.Add(incoming.Cids()[0], NewBadBlockReason(incoming.Cids(), "wrong order of beacon entires"))
			return nil, xerrors.Errorf("wrong order of beacon entires")
		}

		for _, bh := range incoming.Blocks()[1:] {
			if len(targetBE) != len(bh.BeaconEntries) {
				// cannot mark bad, I think @Kubuxu
				return nil, xerrors.Errorf("tipset contained different number for beacon entires")
			}
			for i, be := range bh.BeaconEntries {
				if targetBE[i].Round != be.Round || !bytes.Equal(targetBE[i].Data, be.Data) {
					// cannot mark bad, I think @Kubuxu
					return nil, xerrors.Errorf("tipset contained different beacon entires")
				}
			}

		}
	}

	blockSet := []*types.TipSet{incoming}

	at := incoming.Parents()

	// we want to sync all the blocks until the height above the block we have
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
		ts, err := syncer.store.LoadTipSet(at)
		if err == nil {
			acceptedBlocks = append(acceptedBlocks, at.Cids()...)

			blockSet = append(blockSet, ts)
			at = ts.Parents()
			continue
		}
		if !xerrors.Is(err, bstore.ErrNotFound) {
			log.Warn("loading local tipset: %s", err)
		}

		// NB: GetBlocks validates that the blocks are in-fact the ones we
		// requested, and that they are correctly linked to one another. It does
		// not validate any state transitions.
		window := 500
		if gap := int(blockSet[len(blockSet)-1].Height() - untilHeight); gap < window {
			window = gap
		}
		blks, err := syncer.Bsync.GetBlocks(ctx, at, window)
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
		// FIXME: The reassembly logic should be part of the `BlockSync`
		//  service, the consumer should not be concerned with the
		//  `MaxRequestLength` limitation, it should just be able to request
		//  an segment of arbitrary length. The same burden is put on
		//  `syncFork()` which needs to be aware this as well.
		if blockSet[len(blockSet)-1].IsChildOf(blks[0]) == false {
			return nil, xerrors.Errorf("retrieved segments of the chain are not connected at heights %d/%d",
				blockSet[len(blockSet)-1].Height(), blks[0].Height())
			// A successful `GetBlocks()` call is guaranteed to fetch at least
			// one tipset so the acess `blks[0]` is safe.
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
	if base.Parents() == known.Parents() {
		// common case: receiving a block thats potentially part of the same tipset as our best block
		return blockSet, nil
	}

	if types.CidArrsEqual(base.Parents().Cids(), known.Cids()) {
		// common case: receiving blocks that are building on top of our best tipset
		return blockSet, nil
	}

	// We have now ascertained that this is *not* a 'fast forward'
	log.Warnf("(fork detected) synced header chain (%s - %d) does not link to our best block (%s - %d)", incoming.Cids(), incoming.Height(), known.Cids(), known.Height())
	fork, err := syncer.syncFork(ctx, base, known)
	if err != nil {
		if xerrors.Is(err, ErrForkTooLong) {
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

// syncFork tries to obtain the chain fragment that links a fork into a common
// ancestor in our view of the chain.
//
// If the fork is too long (build.ForkLengthThreshold), we add the entire subchain to the
// denylist. Else, we find the common ancestor, and add the missing chain
// fragment until the fork point to the returned []TipSet.
func (syncer *Syncer) syncFork(ctx context.Context, incoming *types.TipSet, known *types.TipSet) ([]*types.TipSet, error) {
	tips, err := syncer.Bsync.GetBlocks(ctx, incoming.Parents(), int(build.ForkLengthThreshold))
	if err != nil {
		return nil, err
	}

	nts, err := syncer.store.LoadTipSet(known.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to load next local tipset: %w", err)
	}

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
			nts, err = syncer.store.LoadTipSet(nts.Parents())
			if err != nil {
				return nil, xerrors.Errorf("loading next local tipset: %w", err)
			}
		}
	}
	return nil, ErrForkTooLong
}

func (syncer *Syncer) syncMessagesAndCheckState(ctx context.Context, headers []*types.TipSet) error {
	ss := extractSyncState(ctx)
	ss.SetHeight(0)

	return syncer.iterFullTipsets(ctx, headers, func(ctx context.Context, fts *store.FullTipSet) error {
		log.Debugw("validating tipset", "height", fts.TipSet().Height(), "size", len(fts.TipSet().Cids()))
		if err := syncer.ValidateTipSet(ctx, fts); err != nil {
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
	ctx, span := trace.StartSpan(ctx, "iterFullTipsets")
	defer span.End()

	span.AddAttributes(trace.Int64Attribute("num_headers", int64(len(headers))))

	windowSize := 200
	for i := len(headers) - 1; i >= 0; {
		fts, err := syncer.store.TryFillTipSet(headers[i])
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

		batchSize := windowSize
		if i < batchSize {
			batchSize = i
		}

		nextI := (i + 1) - batchSize // want to fetch batchSize values, 'i' points to last one we want to fetch, so its 'inclusive' of our request, thus we need to add one to our request start index

		var bstout []*blocksync.CompactedMessages
		for len(bstout) < batchSize {
			next := headers[nextI]

			nreq := batchSize - len(bstout)
			bstips, err := syncer.Bsync.GetChainMessages(ctx, next, uint64(nreq))
			if err != nil {
				return xerrors.Errorf("message processing failed: %w", err)
			}

			bstout = append(bstout, bstips...)
			nextI += len(bstips)
		}

		for bsi := 0; bsi < len(bstout); bsi++ {
			// temp storage so we don't persist data we dont want to
			bs := bstore.NewTemporary()
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

			if err := persistMessages(bs, bstip); err != nil {
				return err
			}

			if err := copyBlockstore(bs, syncer.store.Blockstore()); err != nil {
				return xerrors.Errorf("message processing failed: %w", err)
			}
		}
		i -= batchSize
	}

	return nil
}

func persistMessages(bs bstore.Blockstore, bst *blocksync.CompactedMessages) error {
	for _, m := range bst.Bls {
		//log.Infof("putting BLS message: %s", m.Cid())
		if _, err := store.PutMessage(bs, m); err != nil {
			log.Errorf("failed to persist messages: %+v", err)
			return xerrors.Errorf("BLS message processing failed: %w", err)
		}
	}
	for _, m := range bst.Secpk {
		if m.Signature.Type != crypto.SigTypeSecp256k1 {
			return xerrors.Errorf("unknown signature type on message %s: %q", m.Cid(), m.Signature.Type)
		}
		//log.Infof("putting secp256k1 message: %s", m.Cid())
		if _, err := store.PutMessage(bs, m); err != nil {
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
//	2. StagePersistHeaders: now that we've collected the missing headers,
//     augmented by those on the other side of a fork, we persist them to the
//     BlockStore.
//
//  3. StageMessages: having acquired the headers and found a common tipset,
//     we then move forward, requesting the full blocks, including the messages.
func (syncer *Syncer) collectChain(ctx context.Context, ts *types.TipSet) error {
	ctx, span := trace.StartSpan(ctx, "collectChain")
	defer span.End()
	ss := extractSyncState(ctx)

	ss.Init(syncer.store.GetHeaviestTipSet(), ts)

	headers, err := syncer.collectHeaders(ctx, ts, syncer.store.GetHeaviestTipSet())
	if err != nil {
		ss.Error(err)
		return err
	}

	span.AddAttributes(trace.Int64Attribute("syncChainLength", int64(len(headers))))

	if !headers[0].Equals(ts) {
		log.Errorf("collectChain headers[0] should be equal to sync target. Its not: %s != %s", headers[0].Cids(), ts.Cids())
	}

	ss.SetStage(api.StagePersistHeaders)

	toPersist := make([]*types.BlockHeader, 0, len(headers)*int(build.BlocksPerEpoch))
	for _, ts := range headers {
		toPersist = append(toPersist, ts.Blocks()...)
	}
	if err := syncer.store.PersistBlockHeaders(toPersist...); err != nil {
		err = xerrors.Errorf("failed to persist synced blocks to the chainstore: %w", err)
		ss.Error(err)
		return err
	}
	toPersist = nil

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

func VerifyElectionPoStVRF(ctx context.Context, worker address.Address, rand []byte, evrf []byte) error {
	if build.InsecurePoStValidation {
		return nil
	}
	return gen.VerifyVRF(ctx, worker, rand, evrf)
}

func (syncer *Syncer) State() []SyncerState {
	var out []SyncerState
	for _, ss := range syncer.syncmgr.syncStates {
		out = append(out, ss.Snapshot())
	}
	return out
}

// MarkBad manually adds a block to the "bad blocks" cache.
func (syncer *Syncer) MarkBad(blk cid.Cid) {
	syncer.bad.Add(blk, NewBadBlockReason([]cid.Cid{blk}, "manually marked bad"))
}

func (syncer *Syncer) CheckBadBlockCache(blk cid.Cid) (string, bool) {
	bbr, ok := syncer.bad.Has(blk)
	return bbr.String(), ok
}

func (syncer *Syncer) getLatestBeaconEntry(_ context.Context, ts *types.TipSet) (*types.BeaconEntry, error) {
	cur := ts
	for i := 0; i < 20; i++ {
		cbe := cur.Blocks()[0].BeaconEntries
		if len(cbe) > 0 {
			return &cbe[len(cbe)-1], nil
		}

		if cur.Height() == 0 {
			return nil, xerrors.Errorf("made it back to genesis block without finding beacon entry")
		}

		next, err := syncer.store.LoadTipSet(cur.Parents())
		if err != nil {
			return nil, xerrors.Errorf("failed to load parents when searching back for latest beacon entry: %w", err)
		}
		cur = next
	}

	return nil, xerrors.Errorf("found NO beacon entries in the 20 blocks prior to given tipset")
}

func (syncer *Syncer) IsEpochBeyondCurrMax(epoch abi.ChainEpoch) bool {
	g, err := syncer.store.GetGenesis()
	if err != nil {
		return false
	}

	now := uint64(build.Clock.Now().Unix())
	return epoch > (abi.ChainEpoch((now-g.Timestamp)/build.BlockDelaySecs) + MaxHeightDrift)
}

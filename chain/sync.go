package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Gurpartap/async"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"github.com/whyrusleeping/pubsub"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	bls "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	amt "github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/lotus/metrics"
)

var log = logging.Logger("chain")

var LocalIncoming = "incoming"

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

	bestPweight := syncer.store.GetHeaviestTipSet().Blocks()[0].ParentWeight
	targetWeight := fts.TipSet().Blocks()[0].ParentWeight
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

func (syncer *Syncer) ValidateMsgMeta(fblk *types.FullBlock) error {
	if msgc := len(fblk.BlsMessages) + len(fblk.SecpkMessages); msgc > build.BlockMessageLimit {
		return xerrors.Errorf("block %s has too many messages (%d)", fblk.Header.Cid(), msgc)
	}

	var bcids, scids []cbg.CBORMarshaler
	for _, m := range fblk.BlsMessages {
		c := cbg.CborCid(m.Cid())
		bcids = append(bcids, &c)
	}

	for _, m := range fblk.SecpkMessages {
		c := cbg.CborCid(m.Cid())
		scids = append(scids, &c)
	}

	// TODO: IMPORTANT(GARBAGE). These message puts and the msgmeta
	// computation need to go into the 'temporary' side of the blockstore when
	// we implement that
	blockstore := syncer.store.Blockstore()

	bs := cbor.NewCborStore(blockstore)
	smroot, err := computeMsgMeta(bs, bcids, scids)
	if err != nil {
		return xerrors.Errorf("validating msgmeta, compute failed: %w", err)
	}

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
		var smsgs []*types.SignedMessage
		var smsgCids []cbg.CBORMarshaler
		for _, m := range smi[bi] {
			smsgs = append(smsgs, allsmsgs[m])
			c := cbg.CborCid(allsmsgs[m].Cid())
			smsgCids = append(smsgCids, &c)
		}

		var bmsgs []*types.Message
		var bmsgCids []cbg.CBORMarshaler
		for _, m := range bmi[bi] {
			bmsgs = append(bmsgs, allbmsgs[m])
			c := cbg.CborCid(allbmsgs[m].Cid())
			bmsgCids = append(bmsgCids, &c)
		}

		if msgc := len(bmsgCids) + len(smsgCids); msgc > build.BlockMessageLimit {
			return nil, fmt.Errorf("block %q has too many messages (%d)", b.Cid(), msgc)
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

func computeMsgMeta(bs cbor.IpldStore, bmsgCids, smsgCids []cbg.CBORMarshaler) (cid.Cid, error) {
	ctx := context.TODO()
	bmroot, err := amt.FromArray(ctx, bs, bmsgCids)
	if err != nil {
		return cid.Undef, err
	}

	smroot, err := amt.FromArray(ctx, bs, smsgCids)
	if err != nil {
		return cid.Undef, err
	}

	mrcid, err := bs.Put(ctx, &types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to put msgmeta: %w", err)
	}

	return mrcid, nil
}

func (syncer *Syncer) FetchTipSet(ctx context.Context, p peer.ID, tsk types.TipSetKey) (*store.FullTipSet, error) {
	if fts, err := syncer.tryLoadFullTipSet(tsk); err == nil {
		return fts, nil
	}

	return syncer.Bsync.GetFullTipSet(ctx, p, tsk)
}

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

	for _, b := range fts.Blocks {
		if err := syncer.ValidateBlock(ctx, b); err != nil {
			if isPermanent(err) {
				syncer.bad.Add(b.Cid(), err.Error())
			}
			return xerrors.Errorf("validating block %s: %w", b.Cid(), err)
		}

		if err := syncer.sm.ChainStore().AddToTipSetTracker(b.Header); err != nil {
			return xerrors.Errorf("failed to add validated header to tipset tracker: %w", err)
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

// Should match up with 'Semantical Validation' in validation.md in the spec
func (syncer *Syncer) ValidateBlock(ctx context.Context, b *types.FullBlock) error {
	ctx, span := trace.StartSpan(ctx, "validateBlock")
	defer span.End()
	if build.InsecurePoStValidation {
		log.Warn("insecure test validation is enabled, if you see this outside of a test, it is a severe bug!")
	}

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

	//nulls := h.Height - (baseTs.Height() + 1)

	// fast checks first

	now := uint64(time.Now().Unix())
	if h.Timestamp > now+build.AllowableClockDrift {
		return xerrors.Errorf("block was from the future (now=%d, blk=%d): %w", now, h.Timestamp, ErrTemporal)
	}
	if h.Timestamp > now {
		log.Warn("Got block from the future, but within threshold", h.Timestamp, time.Now().Unix())
	}

	if h.Timestamp < baseTs.MinTimestamp()+(build.BlockDelay*uint64(h.Height-baseTs.Height())) {
		log.Warn("timestamp funtimes: ", h.Timestamp, baseTs.MinTimestamp(), h.Height, baseTs.Height())
		diff := (baseTs.MinTimestamp() + (build.BlockDelay * uint64(h.Height-baseTs.Height()))) - h.Timestamp

		return xerrors.Errorf("block was generated too soon (h.ts:%d < base.mints:%d + BLOCK_DELAY:%d * deltaH:%d; diff %d)", h.Timestamp, baseTs.MinTimestamp(), build.BlockDelay, h.Height-baseTs.Height(), diff)
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
		rBeacon := *prevBeacon
		if len(h.BeaconEntries) != 0 {
			rBeacon = h.BeaconEntries[len(h.BeaconEntries)-1]
		}
		buf := new(bytes.Buffer)
		if err := h.Miner.MarshalCBOR(buf); err != nil {
			return xerrors.Errorf("failed to marshal miner address to cbor: %w", err)
		}

		//TODO: DST from spec actors when it is there
		vrfBase, err := store.DrawRandomness(rBeacon.Data, crypto.DomainSeparationTag_ElectionProofProduction, h.Height, buf.Bytes())
		if err != nil {
			return xerrors.Errorf("could not draw randomness: %w", err)
		}

		if err := gen.VerifyVRF(ctx, waddr, vrfBase, h.ElectionProof.VRFProof); err != nil {
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

		if !types.IsTicketWinner(h.ElectionProof.VRFProof, mpow.QualityAdjPower, tpow.QualityAdjPower) {
			return xerrors.Errorf("miner created a block but was not a winner")
		}

		return nil
	})

	blockSigCheck := async.Err(func() error {
		if err := sigs.CheckBlockSignature(h, ctx, waddr); err != nil {
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

		err = gen.VerifyVRF(ctx, waddr, vrfBase, h.Ticket.VRFProof)
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
	}

	return merr
}

func (syncer *Syncer) VerifyWinningPoStProof(ctx context.Context, h *types.BlockHeader, prevBeacon types.BeaconEntry, lbst cid.Cid, waddr address.Address) error {
	if build.InsecurePoStValidation {
		if len(h.WinPoStProof) == 0 {
			return xerrors.Errorf("[TESTING] No winning post proof given")
		}

		if string(h.WinPoStProof[0].ProofBytes) == "valid proof" {
			return nil
		}
		return xerrors.Errorf("[TESTING] winning post was invalid")
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
		log.Errorf("invalid winning post (%x; %v)", rand, sectors)
		return xerrors.Errorf("winning post was invalid")
	}

	return nil
}

func (syncer *Syncer) checkBlockMessages(ctx context.Context, b *types.FullBlock, baseTs *types.TipSet) error {
	{
		var sigCids []cid.Cid // this is what we get for people not wanting the marshalcbor method on the cid type
		var pubks []bls.PublicKey

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

	checkMsg := func(m *types.Message) error {
		if m.To == address.Undef {
			return xerrors.New("'To' address cannot be empty")
		}

		if _, ok := nonces[m.From]; !ok {
			// `GetActor` does not validate that this is an account actor.
			act, err := st.GetActor(m.From)
			if err != nil {
				return xerrors.Errorf("failed to get actor: %w", err)
			}
			nonces[m.From] = act.Nonce
		}

		if nonces[m.From] != m.Nonce {
			return xerrors.Errorf("wrong nonce (exp: %d, got: %d)", nonces[m.From], m.Nonce)
		}
		nonces[m.From]++

		return nil
	}

	var blsCids []cbg.CBORMarshaler

	for i, m := range b.BlsMessages {
		if err := checkMsg(m); err != nil {
			return xerrors.Errorf("block had invalid bls message at index %d: %w", i, err)
		}

		c := cbg.CborCid(m.Cid())
		blsCids = append(blsCids, &c)
	}

	var secpkCids []cbg.CBORMarshaler
	for i, m := range b.SecpkMessages {
		if err := checkMsg(&m.Message); err != nil {
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
		secpkCids = append(secpkCids, &c)
	}

	bmroot, err := amt.FromArray(ctx, cst, blsCids)
	if err != nil {
		return xerrors.Errorf("failed to build amt from bls msg cids: %w", err)
	}

	smroot, err := amt.FromArray(ctx, cst, secpkCids)
	if err != nil {
		return xerrors.Errorf("failed to build amt from bls msg cids: %w", err)
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

func (syncer *Syncer) verifyBlsAggregate(ctx context.Context, sig *crypto.Signature, msgs []cid.Cid, pubks []bls.PublicKey) error {
	_, span := trace.StartSpan(ctx, "syncer.verifyBlsAggregate")
	defer span.End()
	span.AddAttributes(
		trace.Int64Attribute("msgCount", int64(len(msgs))),
	)

	var wg sync.WaitGroup

	digests := make([]bls.Digest, len(msgs))
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for j := 0; (j*10)+w < len(msgs); j++ {
				digests[j*10+w] = bls.Hash(bls.Message(msgs[j*10+w].Bytes()))
			}
		}(i)
	}
	wg.Wait()

	var bsig bls.Signature
	copy(bsig[:], sig.Data)
	if !bls.Verify(&bsig, digests, pubks) {
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

func (syncer *Syncer) collectHeaders(ctx context.Context, from *types.TipSet, to *types.TipSet) ([]*types.TipSet, error) {
	ctx, span := trace.StartSpan(ctx, "collectHeaders")
	defer span.End()
	ss := extractSyncState(ctx)

	span.AddAttributes(
		trace.Int64Attribute("fromHeight", int64(from.Height())),
		trace.Int64Attribute("toHeight", int64(to.Height())),
	)

	markBad := func(fmts string, args ...interface{}) {
		for _, b := range from.Cids() {
			syncer.bad.Add(b, fmt.Sprintf(fmts, args...))
		}
	}

	for _, pcid := range from.Parents().Cids() {
		if reason, ok := syncer.bad.Has(pcid); ok {
			markBad("linked to %s", pcid)
			return nil, xerrors.Errorf("chain linked to block marked previously as bad (%s, %s) (reason: %s)", from.Cids(), pcid, reason)
		}
	}

	{
		// ensure consistency of beacon entires
		targetBE := from.Blocks()[0].BeaconEntries
		sorted := sort.SliceIsSorted(targetBE, func(i, j int) bool {
			return targetBE[i].Round < targetBE[j].Round
		})
		if !sorted {
			syncer.bad.Add(from.Cids()[0], "wrong order of beacon entires")
			return nil, xerrors.Errorf("wrong order of beacon entires")
		}

		for _, bh := range from.Blocks()[1:] {
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

	blockSet := []*types.TipSet{from}

	at := from.Parents()

	// we want to sync all the blocks until the height above the block we have
	untilHeight := to.Height() + 1

	ss.SetHeight(blockSet[len(blockSet)-1].Height())

	var acceptedBlocks []cid.Cid

loop:
	for blockSet[len(blockSet)-1].Height() > untilHeight {
		for _, bc := range at.Cids() {
			if reason, ok := syncer.bad.Has(bc); ok {
				for _, b := range acceptedBlocks {
					syncer.bad.Add(b, fmt.Sprintf("chain contained %s", bc))
				}

				return nil, xerrors.Errorf("chain contained block marked previously as bad (%s, %s) (reason: %s)", from.Cids(), bc, reason)
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
		// requested, and that they are correctly linked to eachother. It does
		// not validate any state transitions
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

		for _, b := range blks {
			if b.Height() < untilHeight {
				break loop
			}
			for _, bc := range b.Cids() {
				if reason, ok := syncer.bad.Has(bc); ok {
					for _, b := range acceptedBlocks {
						syncer.bad.Add(b, fmt.Sprintf("chain contained %s", bc))
					}

					return nil, xerrors.Errorf("chain contained block marked previously as bad (%s, %s) (reason: %s)", from.Cids(), bc, reason)
				}
			}
			blockSet = append(blockSet, b)
		}

		acceptedBlocks = append(acceptedBlocks, at.Cids()...)

		ss.SetHeight(blks[len(blks)-1].Height())
		at = blks[len(blks)-1].Parents()
	}

	// We have now ascertained that this is *not* a 'fast forward'
	if !types.CidArrsEqual(blockSet[len(blockSet)-1].Parents().Cids(), to.Cids()) {
		last := blockSet[len(blockSet)-1]
		if last.Parents() == to.Parents() {
			// common case: receiving a block thats potentially part of the same tipset as our best block
			return blockSet, nil
		}

		log.Warnf("(fork detected) synced header chain (%s - %d) does not link to our best block (%s - %d)", from.Cids(), from.Height(), to.Cids(), to.Height())
		fork, err := syncer.syncFork(ctx, last, to)
		if err != nil {
			if xerrors.Is(err, ErrForkTooLong) {
				// TODO: we're marking this block bad in the same way that we mark invalid blocks bad. Maybe distinguish?
				log.Warn("adding forked chain to our bad tipset cache")
				for _, b := range from.Blocks() {
					syncer.bad.Add(b.Cid(), "fork past finality")
				}
			}
			return nil, xerrors.Errorf("failed to sync fork: %w", err)
		}

		blockSet = append(blockSet, fork...)
	}

	return blockSet, nil
}

var ErrForkTooLong = fmt.Errorf("fork longer than threshold")

func (syncer *Syncer) syncFork(ctx context.Context, from *types.TipSet, to *types.TipSet) ([]*types.TipSet, error) {
	tips, err := syncer.Bsync.GetBlocks(ctx, from.Parents(), build.ForkLengthThreshold)
	if err != nil {
		return nil, err
	}

	nts, err := syncer.store.LoadTipSet(to.Parents())
	if err != nil {
		return nil, xerrors.Errorf("failed to load next local tipset: %w", err)
	}

	for cur := 0; cur < len(tips); {
		if nts.Height() == 0 {
			if !syncer.Genesis.Equals(nts) {
				return nil, xerrors.Errorf("somehow synced chain that linked back to a different genesis (bad genesis: %s)", nts.Key())
			}
			return nil, xerrors.Errorf("synced chain forked at genesis, refusing to sync")
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

		var bstout []*blocksync.BSTipSet
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
			ds := dstore.NewMapDatastore()
			bs := bstore.NewBlockstore(ds)
			blks := cbor.NewCborStore(bs)

			this := headers[i-bsi]
			bstip := bstout[len(bstout)-(bsi+1)]
			fts, err := zipTipSetAndMessages(blks, this, bstip.BlsMessages, bstip.SecpkMessages, bstip.BlsMsgIncludes, bstip.SecpkMsgIncludes)
			if err != nil {
				log.Warnw("zipping failed", "error", err, "bsi", bsi, "i", i,
					"height", this.Height(), "bstip-height", bstip.Blocks[0].Height,
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

func persistMessages(bs bstore.Blockstore, bst *blocksync.BSTipSet) error {
	for _, m := range bst.BlsMessages {
		//log.Infof("putting BLS message: %s", m.Cid())
		if _, err := store.PutMessage(bs, m); err != nil {
			log.Errorf("failed to persist messages: %+v", err)
			return xerrors.Errorf("BLS message processing failed: %w", err)
		}
	}
	for _, m := range bst.SecpkMessages {
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

	toPersist := make([]*types.BlockHeader, 0, len(headers)*build.BlocksPerEpoch)
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

func VerifyElectionPoStVRF(ctx context.Context, evrf []byte, rand []byte, worker address.Address) error {
	if err := gen.VerifyVRF(ctx, worker, rand, evrf); err != nil {
		return xerrors.Errorf("failed to verify post_randomness vrf: %w", err)
	}

	return nil
}

func (syncer *Syncer) State() []SyncerState {
	var out []SyncerState
	for _, ss := range syncer.syncmgr.syncStates {
		out = append(out, ss.Snapshot())
	}
	return out
}

func (syncer *Syncer) MarkBad(blk cid.Cid) {
	syncer.bad.Add(blk, "manually marked bad")
}

func (syncer *Syncer) CheckBadBlockCache(blk cid.Cid) (string, bool) {
	return syncer.bad.Has(blk)
}
func (syncer *Syncer) getLatestBeaconEntry(ctx context.Context, ts *types.TipSet) (*types.BeaconEntry, error) {
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

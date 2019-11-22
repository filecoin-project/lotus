package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Gurpartap/async"
	amt "github.com/filecoin-project/go-amt-ipld"
	"github.com/filecoin-project/go-bls-sigs"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/blocksync"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("chain")

type Syncer struct {
	// The heaviest known tipset in the network.

	// The interface for accessing and putting tipsets into local storage
	store *store.ChainStore

	// the state manager handles making state queries
	sm *stmgr.StateManager

	// The known Genesis tipset
	Genesis *types.TipSet

	syncLock sync.Mutex

	// TipSets known to be invalid
	bad *BadBlockCache

	// handle to the block sync service
	Bsync *blocksync.BlockSync

	self peer.ID

	syncState SyncerState

	// peer heads
	// Note: clear cache on disconnects
	peerHeads   map[peer.ID]*types.TipSet
	peerHeadsLk sync.Mutex
}

func NewSyncer(sm *stmgr.StateManager, bsync *blocksync.BlockSync, self peer.ID) (*Syncer, error) {
	gen, err := sm.ChainStore().GetGenesis()
	if err != nil {
		return nil, err
	}

	gent, err := types.NewTipSet([]*types.BlockHeader{gen})
	if err != nil {
		return nil, err
	}

	return &Syncer{
		bad:       NewBadBlockCache(),
		Genesis:   gent,
		Bsync:     bsync,
		peerHeads: make(map[peer.ID]*types.TipSet),
		store:     sm.ChainStore(),
		sm:        sm,
		self:      self,
	}, nil
}

const BootstrapPeerThreshold = 1

// InformNewHead informs the syncer about a new potential tipset
// This should be called when connecting to new peers, and additionally
// when receiving new blocks from the network
func (syncer *Syncer) InformNewHead(from peer.ID, fts *store.FullTipSet) {
	ctx := context.Background()
	if fts == nil {
		panic("bad")
	}

	for _, b := range fts.Blocks {
		if err := syncer.ValidateMsgMeta(b); err != nil {
			log.Warnf("invalid block received: %s", err)
			return
		}
	}

	if from == syncer.self {
		// TODO: this is kindof a hack...
		log.Debug("got block from ourselves")

		if err := syncer.Sync(ctx, fts.TipSet()); err != nil {
			log.Errorf("failed to sync our own block %s: %+v", fts.TipSet().Cids(), err)
		}

		return
	}

	// TODO: IMPORTANT(GARBAGE) this needs to be put in the 'temporary' side of
	// the blockstore
	if err := syncer.store.PersistBlockHeaders(fts.TipSet().Blocks()...); err != nil {
		log.Warn("failed to persist incoming block header: ", err)
		return
	}

	syncer.peerHeadsLk.Lock()
	syncer.peerHeads[from] = fts.TipSet()
	syncer.peerHeadsLk.Unlock()
	syncer.Bsync.AddPeer(from)

	bestPweight := syncer.store.GetHeaviestTipSet().Blocks()[0].ParentWeight
	targetWeight := fts.TipSet().Blocks()[0].ParentWeight
	if targetWeight.LessThan(bestPweight) {
		log.Warn("incoming tipset does not appear to be better than our best chain, ignoring for now")
		return
	}

	go func() {
		if err := syncer.Sync(ctx, fts.TipSet()); err != nil {
			log.Errorf("sync error (curW=%s, targetW=%s): %+v", bestPweight, targetWeight, err)
		}
	}()
}

func (syncer *Syncer) ValidateMsgMeta(fblk *types.FullBlock) error {
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

	bs := amt.WrapBlockstore(blockstore)
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

func (syncer *Syncer) InformNewBlock(from peer.ID, blk *types.FullBlock) {
	// TODO: search for other blocks that could form a tipset with this block
	// and then send that tipset to InformNewHead

	fts := &store.FullTipSet{Blocks: []*types.FullBlock{blk}}
	syncer.InformNewHead(from, fts)
}

func reverse(tips []*types.TipSet) []*types.TipSet {
	out := make([]*types.TipSet, len(tips))
	for i := 0; i < len(tips); i++ {
		out[i] = tips[len(tips)-(i+1)]
	}
	return out
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
func zipTipSetAndMessages(bs amt.Blocks, ts *types.TipSet, allbmsgs []*types.Message, allsmsgs []*types.SignedMessage, bmi, smi [][]uint64) (*store.FullTipSet, error) {
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

		mrcid, err := computeMsgMeta(bs, bmsgCids, smsgCids)
		if err != nil {
			return nil, err
		}

		if b.Messages != mrcid {
			return nil, fmt.Errorf("messages didnt match message root in header")
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

func computeMsgMeta(bs amt.Blocks, bmsgCids, smsgCids []cbg.CBORMarshaler) (cid.Cid, error) {
	bmroot, err := amt.FromArray(bs, bmsgCids)
	if err != nil {
		return cid.Undef, err
	}

	smroot, err := amt.FromArray(bs, smsgCids)
	if err != nil {
		return cid.Undef, err
	}

	mrcid, err := bs.Put(&types.MsgMeta{
		BlsMessages:   bmroot,
		SecpkMessages: smroot,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to put msgmeta: %w", err)
	}

	return mrcid, nil
}

func (syncer *Syncer) selectHead(ctx context.Context, heads map[peer.ID]*types.TipSet) (*types.TipSet, error) {
	var headsArr []*types.TipSet
	for _, ts := range heads {
		headsArr = append(headsArr, ts)
	}

	sel := headsArr[0]
	for i := 1; i < len(headsArr); i++ {
		cur := headsArr[i]

		yes, err := syncer.store.IsAncestorOf(cur, sel)
		if err != nil {
			return nil, err
		}
		if yes {
			continue
		}

		yes, err = syncer.store.IsAncestorOf(sel, cur)
		if err != nil {
			return nil, err
		}
		if yes {
			sel = cur
			continue
		}

		nca, err := syncer.store.NearestCommonAncestor(cur, sel)
		if err != nil {
			return nil, err
		}

		if sel.Height()-nca.Height() > build.ForkLengthThreshold {
			// TODO: handle this better than refusing to sync
			return nil, fmt.Errorf("Conflict exists in heads set")
		}

		curw, err := syncer.store.Weight(ctx, cur)
		if err != nil {
			return nil, err
		}
		selw, err := syncer.store.Weight(ctx, sel)
		if err != nil {
			return nil, err
		}

		if curw.GreaterThan(selw) {
			sel = cur
		}
	}
	return sel, nil
}

func (syncer *Syncer) FetchTipSet(ctx context.Context, p peer.ID, cids []cid.Cid) (*store.FullTipSet, error) {
	if fts, err := syncer.tryLoadFullTipSet(cids); err == nil {
		return fts, nil
	}

	return syncer.Bsync.GetFullTipSet(ctx, p, cids)
}

func (syncer *Syncer) tryLoadFullTipSet(cids []cid.Cid) (*store.FullTipSet, error) {
	ts, err := syncer.store.LoadTipSet(cids)
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

	syncer.syncLock.Lock()
	defer syncer.syncLock.Unlock()

	if syncer.Genesis.Equals(maybeHead) || syncer.store.GetHeaviestTipSet().Equals(maybeHead) {
		return nil
	}

	if err := syncer.collectChain(ctx, maybeHead); err != nil {
		span.AddAttributes(trace.StringAttribute("col_error", err.Error()))
		return xerrors.Errorf("collectChain failed: %w", err)
	}

	if err := syncer.store.PutTipSet(ctx, maybeHead); err != nil {
		span.AddAttributes(trace.StringAttribute("put_error", err.Error()))
		return xerrors.Errorf("failed to put synced tipset to chainstore: %w", err)
	}

	return nil
}

func isPermanent(err error) bool {
	return !errors.Is(err, ErrTemporal)
}

func (syncer *Syncer) ValidateTipSet(ctx context.Context, fts *store.FullTipSet) error {
	ctx, span := trace.StartSpan(ctx, "validateTipSet")
	defer span.End()

	ts := fts.TipSet()
	if ts.Equals(syncer.Genesis) {
		return nil
	}

	for _, b := range fts.Blocks {
		if err := syncer.ValidateBlock(ctx, b); err != nil {
			if isPermanent(err) {
				syncer.bad.Add(b.Cid())
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
	var err error
	enc, err := actors.SerializeParams(&actors.IsValidMinerParam{Addr: maddr})
	if err != nil {
		return err
	}

	ret, err := syncer.sm.Call(ctx, &types.Message{
		To:     actors.StoragePowerAddress,
		From:   maddr,
		Method: actors.SPAMethods.IsValidMiner,
		Params: enc,
	}, baseTs)
	if err != nil {
		return xerrors.Errorf("checking if block miner is valid failed: %w", err)
	}

	if ret.ExitCode != 0 {
		return xerrors.Errorf("StorageMarket.IsValidMiner check failed (exit code %d)", ret.ExitCode)
	}

	if !bytes.Equal(ret.Return, cbg.CborBoolTrue) {
		return xerrors.New("miner isn't valid")
	}

	return nil
}

func (syncer *Syncer) validateTickets(ctx context.Context, mworker address.Address, tickets []*types.Ticket, base *types.TipSet) error {
	ctx, span := trace.StartSpan(ctx, "validateTickets")
	defer span.End()
	span.AddAttributes(trace.Int64Attribute("tickets", int64(len(tickets))))

	if len(tickets) == 0 {
		return xerrors.Errorf("block had no tickets")
	}

	cur := base.MinTicket()
	for i := 0; i < len(tickets); i++ {
		next := tickets[i]

		sig := &types.Signature{
			Type: types.KTBLS,
			Data: next.VRFProof,
		}

		// TODO: ticket signatures should also include miner address
		if err := sig.Verify(mworker, cur.VRFProof); err != nil {
			return xerrors.Errorf("invalid ticket, VRFProof invalid: %w", err)
		}

		cur = next
	}

	return nil
}

var ErrTemporal = errors.New("temporal error")

// Should match up with 'Semantical Validation' in validation.md in the spec
func (syncer *Syncer) ValidateBlock(ctx context.Context, b *types.FullBlock) error {
	ctx, span := trace.StartSpan(ctx, "validateBlock")
	defer span.End()

	h := b.Header

	baseTs, err := syncer.store.LoadTipSet(h.Parents)
	if err != nil {
		return xerrors.Errorf("load parent tipset failed (%s): %w", h.Parents, err)
	}

	// fast checks first
	if h.Timestamp > uint64(time.Now().Unix()+build.AllowableClockDrift) {
		return xerrors.Errorf("block was from the future")
	}

	if h.Timestamp < baseTs.MinTimestamp()+uint64(build.BlockDelay*len(h.Tickets)) {
		log.Warn("timestamp funtimes: ", h.Timestamp, baseTs.MinTimestamp(), len(h.Tickets))
		return xerrors.Errorf("block was generated too soon (h.ts:%d < base.mints:%d + BLOCK_DELAY:%d * tkts.len:%d)", h.Timestamp, baseTs.MinTimestamp(), build.BlockDelay, len(h.Tickets))
	}

	winnerCheck := async.Err(func() error {
		mpow, tpow, err := stmgr.GetPower(ctx, syncer.sm, baseTs, h.Miner)
		if err != nil {
			return xerrors.Errorf("failed getting power: %w", err)
		}

		if !types.PowerCmp(h.ElectionProof, mpow, tpow) {
			return xerrors.Errorf("miner created a block but was not a winner")
		}
		return nil
	})

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

	waddr, err := stmgr.GetMinerWorkerRaw(ctx, syncer.sm, stateroot, h.Miner)
	if err != nil {
		return xerrors.Errorf("GetMinerWorkerRaw failed: %w", err)
	}

	blockSigCheck := async.Err(func() error {
		if err := h.CheckBlockSignature(ctx, waddr); err != nil {
			return xerrors.Errorf("check block signature failed: %w", err)
		}
		return nil
	})

	tktsCheck := async.Err(func() error {
		if err := syncer.validateTickets(ctx, waddr, h.Tickets, baseTs); err != nil {
			return xerrors.Errorf("validating block tickets failed: %w", err)
		}
		return nil
	})

	eproofCheck := async.Err(func() error {
		rand, err := syncer.sm.ChainStore().GetRandomness(ctx, baseTs.Cids(), h.Tickets, build.EcRandomnessLookback)
		if err != nil {
			return xerrors.Errorf("failed to get randomness for verifying election proof: %w", err)
		}

		if err := VerifyElectionProof(ctx, h.ElectionProof, rand, waddr); err != nil {
			return xerrors.Errorf("checking eproof failed: %w", err)
		}
		return nil
	})

	await := []async.ErrorFuture{
		minerCheck,
		tktsCheck,
		blockSigCheck,
		eproofCheck,
		winnerCheck,
		msgsCheck,
	}

	var merr error
	for _, fut := range await {
		if err := fut.AwaitContext(ctx); err != nil {
			merr = multierror.Append(merr, err)
		}
	}

	return merr
}

func (syncer *Syncer) checkBlockMessages(ctx context.Context, b *types.FullBlock, baseTs *types.TipSet) error {
	nonces := make(map[address.Address]uint64)
	balances := make(map[address.Address]types.BigInt)

	stateroot, _, err := syncer.sm.TipSetState(ctx, baseTs)
	if err != nil {
		return err
	}

	cst := hamt.CSTFromBstore(syncer.store.Blockstore())
	st, err := state.LoadStateTree(cst, stateroot)
	if err != nil {
		return xerrors.Errorf("failed to load base state tree: %w", err)
	}

	checkMsg := func(m *types.Message) error {
		if m.To == address.Undef {
			return xerrors.New("'To' address cannot be empty")
		}

		if _, ok := nonces[m.From]; !ok {
			act, err := st.GetActor(m.From)
			if err != nil {
				return xerrors.Errorf("failed to get actor: %w", err)
			}
			nonces[m.From] = act.Nonce
			balances[m.From] = act.Balance
		}

		if nonces[m.From] != m.Nonce {
			return xerrors.Errorf("wrong nonce (exp: %d, got: %d)", nonces[m.From], m.Nonce)
		}
		nonces[m.From]++

		if balances[m.From].LessThan(m.RequiredFunds()) {
			return xerrors.Errorf("not enough funds for message execution")
		}

		balances[m.From] = types.BigSub(balances[m.From], m.RequiredFunds())
		return nil
	}

	bs := amt.WrapBlockstore(syncer.store.Blockstore())
	var blsCids []cbg.CBORMarshaler
	var sigCids []cid.Cid // this is what we get for people not wanting the marshalcbor method on the cid type

	var pubks []bls.PublicKey
	for i, m := range b.BlsMessages {
		if err := checkMsg(m); err != nil {
			return xerrors.Errorf("block had invalid bls message at index %d: %w", i, err)
		}

		sigCids = append(sigCids, m.Cid())
		c := cbg.CborCid(m.Cid())
		blsCids = append(blsCids, &c)

		pubk, err := syncer.sm.GetBlsPublicKey(ctx, m.From, baseTs)
		if err != nil {
			return xerrors.Errorf("failed to load bls public to validate block: %w", err)
		}

		pubks = append(pubks, pubk)
	}

	if err := syncer.verifyBlsAggregate(ctx, b.Header.BLSAggregate, sigCids, pubks); err != nil {
		return xerrors.Errorf("bls aggregate signature was invalid: %w", err)
	}

	var secpkCids []cbg.CBORMarshaler
	for i, m := range b.SecpkMessages {
		if err := checkMsg(&m.Message); err != nil {
			return xerrors.Errorf("block had invalid secpk message at index %d: %w", i, err)
		}

		kaddr, err := syncer.sm.ResolveToKeyAddress(ctx, m.Message.From, baseTs)
		if err != nil {
			return xerrors.Errorf("failed to resolve key addr: %w", err)
		}

		if err := m.Signature.Verify(kaddr, m.Message.Cid().Bytes()); err != nil {
			return xerrors.Errorf("secpk message %s has invalid signature: %w", m.Cid(), err)
		}

		c := cbg.CborCid(m.Cid())
		secpkCids = append(secpkCids, &c)
	}

	bmroot, err := amt.FromArray(bs, blsCids)
	if err != nil {
		return xerrors.Errorf("failed to build amt from bls msg cids: %w", err)
	}

	smroot, err := amt.FromArray(bs, secpkCids)
	if err != nil {
		return xerrors.Errorf("failed to build amt from bls msg cids: %w", err)
	}

	mrcid, err := bs.Put(&types.MsgMeta{
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

func (syncer *Syncer) verifyBlsAggregate(ctx context.Context, sig types.Signature, msgs []cid.Cid, pubks []bls.PublicKey) error {
	ctx, span := trace.StartSpan(ctx, "syncer.verifyBlsAggregate")
	defer span.End()
	span.AddAttributes(
		trace.Int64Attribute("msgCount", int64(len(msgs))),
	)

	var digests []bls.Digest
	for _, c := range msgs {
		digests = append(digests, bls.Hash(bls.Message(c.Bytes())))
	}

	var bsig bls.Signature
	copy(bsig[:], sig.Data)
	if !bls.Verify(&bsig, digests, pubks) {
		return xerrors.New("bls aggregate signature failed to verify")
	}

	return nil
}

func (syncer *Syncer) collectHeaders(ctx context.Context, from *types.TipSet, to *types.TipSet) ([]*types.TipSet, error) {
	ctx, span := trace.StartSpan(ctx, "collectHeaders")
	defer span.End()

	span.AddAttributes(
		trace.Int64Attribute("fromHeight", int64(from.Height())),
		trace.Int64Attribute("toHeight", int64(to.Height())),
	)

	for _, pcid := range from.Parents() {
		if syncer.bad.Has(pcid) {
			for _, b := range from.Cids() {
				syncer.bad.Add(b)
			}
			return nil, xerrors.Errorf("chain linked to block marked previously as bad (%s, %s)", from.Cids(), pcid)
		}
	}

	blockSet := []*types.TipSet{from}

	at := from.Parents()

	// we want to sync all the blocks until the height above the block we have
	untilHeight := to.Height() + 1

	syncer.syncState.SetHeight(blockSet[len(blockSet)-1].Height())

	var acceptedBlocks []cid.Cid

loop:
	for blockSet[len(blockSet)-1].Height() > untilHeight {
		for _, bc := range at {
			if syncer.bad.Has(bc) {
				for _, b := range acceptedBlocks {
					syncer.bad.Add(b)
				}

				return nil, xerrors.Errorf("chain contained block marked previously as bad (%s, %s)", from.Cids(), bc)
			}
		}

		// If, for some reason, we have a suffix of the chain locally, handle that here
		ts, err := syncer.store.LoadTipSet(at)
		if err == nil {
			acceptedBlocks = append(acceptedBlocks, at...)

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
				if syncer.bad.Has(bc) {
					for _, b := range acceptedBlocks {
						syncer.bad.Add(b)
					}

					return nil, xerrors.Errorf("chain contained block marked previously as bad (%s, %s)", from.Cids(), bc)
				}
			}
			blockSet = append(blockSet, b)
		}

		acceptedBlocks = append(acceptedBlocks, at...)

		syncer.syncState.SetHeight(blks[len(blks)-1].Height())
		at = blks[len(blks)-1].Parents()
	}

	// We have now ascertained that this is *not* a 'fast forward'
	if !types.CidArrsEqual(blockSet[len(blockSet)-1].Parents(), to.Cids()) {
		last := blockSet[len(blockSet)-1]
		if types.CidArrsEqual(last.Parents(), to.Parents()) {
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
					syncer.bad.Add(b.Cid())
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
	syncer.syncState.SetHeight(0)

	return syncer.iterFullTipsets(ctx, headers, func(ctx context.Context, fts *store.FullTipSet) error {
		log.Debugw("validating tipset", "height", fts.TipSet().Height(), "size", len(fts.TipSet().Cids()))
		if err := syncer.ValidateTipSet(ctx, fts); err != nil {
			log.Errorf("failed to validate tipset: %+v", err)
			return xerrors.Errorf("message processing failed: %w", err)
		}

		syncer.syncState.SetHeight(fts.TipSet().Height())

		return nil
	})
}

// fills out each of the given tipsets with messages and calls the callback with it
func (syncer *Syncer) iterFullTipsets(ctx context.Context, headers []*types.TipSet, cb func(context.Context, *store.FullTipSet) error) error {
	ctx, span := trace.StartSpan(ctx, "iterFullTipsets")
	defer span.End()

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

		next := headers[i-batchSize]
		bstips, err := syncer.Bsync.GetChainMessages(ctx, next, uint64(batchSize+1))
		if err != nil {
			return xerrors.Errorf("message processing failed: %w", err)
		}

		for bsi := 0; bsi < len(bstips); bsi++ {
			// temp storage so we don't persist data we dont want to
			ds := dstore.NewMapDatastore()
			bs := bstore.NewBlockstore(ds)
			blks := amt.WrapBlockstore(bs)

			this := headers[i-bsi]
			bstip := bstips[len(bstips)-(bsi+1)]
			fts, err := zipTipSetAndMessages(blks, this, bstip.BlsMessages, bstip.SecpkMessages, bstip.BlsMsgIncludes, bstip.SecpkMsgIncludes)
			if err != nil {
				log.Warnw("zipping failed", "error", err, "bsi", bsi, "i", i,
					"height", this.Height(), "bstip-height", bstip.Blocks[0].Height,
					"bstips", bstips, "next-height", i+batchSize)
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
		i -= windowSize
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
		if m.Signature.Type != types.KTSecp256k1 {
			return xerrors.Errorf("unknown signature type on message %s: %q", m.Cid(), m.Signature.TypeCode)
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

	syncer.syncState.Init(syncer.store.GetHeaviestTipSet(), ts)

	headers, err := syncer.collectHeaders(ctx, ts, syncer.store.GetHeaviestTipSet())
	if err != nil {
		return err
	}

	span.AddAttributes(trace.Int64Attribute("syncChainLength", int64(len(headers))))

	if !headers[0].Equals(ts) {
		log.Errorf("collectChain headers[0] should be equal to sync target. Its not: %s != %s", headers[0].Cids(), ts.Cids())
	}

	syncer.syncState.SetStage(api.StagePersistHeaders)

	toPersist := make([]*types.BlockHeader, 0, len(headers)*build.BlocksPerEpoch)
	for _, ts := range headers {
		toPersist = append(toPersist, ts.Blocks()...)
	}
	if err := syncer.store.PersistBlockHeaders(toPersist...); err != nil {
		return xerrors.Errorf("failed to persist synced blocks to the chainstore: %w", err)
	}
	toPersist = nil

	syncer.syncState.SetStage(api.StageMessages)

	if err := syncer.syncMessagesAndCheckState(ctx, headers); err != nil {
		return xerrors.Errorf("collectChain syncMessages: %w", err)
	}

	syncer.syncState.SetStage(api.StageSyncComplete)
	log.Debugw("new tipset", "height", ts.Height(), "tipset", types.LogCids(ts.Cids()))

	return nil
}

func VerifyElectionProof(ctx context.Context, eproof []byte, rand []byte, worker address.Address) error {
	sig := types.Signature{
		Data: eproof,
		Type: types.KTBLS,
	}

	if err := sig.Verify(worker, rand); err != nil {
		return xerrors.Errorf("failed to verify election proof signature: %w", err)
	}

	return nil
}

func (syncer *Syncer) State() SyncerState {
	return syncer.syncState.Snapshot()
}

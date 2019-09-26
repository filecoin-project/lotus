package chain

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/stmgr"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"
	"github.com/filecoin-project/go-lotus/lib/vdf"

	amt "github.com/filecoin-project/go-amt-ipld"
	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
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
	bad BadTipSetCache

	// handle to the block sync service
	Bsync *BlockSync

	self peer.ID

	// peer heads
	// Note: clear cache on disconnects
	peerHeads   map[peer.ID]*types.TipSet
	peerHeadsLk sync.Mutex
}

func NewSyncer(sm *stmgr.StateManager, bsync *BlockSync, self peer.ID) (*Syncer, error) {
	gen, err := sm.ChainStore().GetGenesis()
	if err != nil {
		return nil, err
	}

	gent, err := types.NewTipSet([]*types.BlockHeader{gen})
	if err != nil {
		return nil, err
	}

	return &Syncer{
		Genesis:   gent,
		Bsync:     bsync,
		peerHeads: make(map[peer.ID]*types.TipSet),
		store:     sm.ChainStore(),
		sm:        sm,
		self:      self,
	}, nil
}

type BadTipSetCache struct {
	badBlocks map[cid.Cid]struct{}
}

const BootstrapPeerThreshold = 1

// InformNewHead informs the syncer about a new potential tipset
// This should be called when connecting to new peers, and additionally
// when receiving new blocks from the network
func (syncer *Syncer) InformNewHead(from peer.ID, fts *store.FullTipSet) {
	if fts == nil {
		panic("bad")
	}
	if from == syncer.self {
		// TODO: this is kindof a hack...
		log.Debug("got block from ourselves")

		if err := syncer.Sync(fts); err != nil {
			log.Errorf("failed to sync our own block: %s", err)
		}

		return
	}
	syncer.peerHeadsLk.Lock()
	syncer.peerHeads[from] = fts.TipSet()
	syncer.peerHeadsLk.Unlock()
	syncer.Bsync.AddPeer(from)

	go func() {
		if err := syncer.Sync(fts); err != nil {
			log.Errorf("sync error: %s", err)
		}
	}()
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

		smroot, err := amt.FromArray(bs, smsgCids)
		if err != nil {
			return nil, err
		}

		var bmsgs []*types.Message
		var bmsgCids []cbg.CBORMarshaler
		for _, m := range bmi[bi] {
			bmsgs = append(bmsgs, allbmsgs[m])
			c := cbg.CborCid(allbmsgs[m].Cid())
			bmsgCids = append(bmsgCids, &c)
		}

		bmroot, err := amt.FromArray(bs, bmsgCids)
		if err != nil {
			return nil, err
		}

		mrcid, err := bs.Put(&types.MsgMeta{
			BlsMessages:   bmroot,
			SecpkMessages: smroot,
		})
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

func (syncer *Syncer) selectHead(heads map[peer.ID]*types.TipSet) (*types.TipSet, error) {
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

		if syncer.store.Weight(cur) > syncer.store.Weight(sel) {
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

func (syncer *Syncer) Sync(maybeHead *store.FullTipSet) error {
	syncer.syncLock.Lock()
	defer syncer.syncLock.Unlock()

	ts := maybeHead.TipSet()
	if syncer.Genesis.Equals(ts) || syncer.store.GetHeaviestTipSet().Equals(ts) {
		return nil
	}

	if err := syncer.collectChain(maybeHead); err != nil {
		return xerrors.Errorf("collectChain failed: %w", err)
	}

	if err := syncer.store.PutTipSet(maybeHead); err != nil {
		return errors.Wrap(err, "failed to put synced tipset to chainstore")
	}

	return nil
}

func (syncer *Syncer) ValidateTipSet(ctx context.Context, fts *store.FullTipSet) error {
	ts := fts.TipSet()
	if ts.Equals(syncer.Genesis) {
		return nil
	}

	for _, b := range fts.Blocks {
		if err := syncer.ValidateBlock(ctx, b); err != nil {
			return xerrors.Errorf("validating block %s: %w", b.Cid(), err)
		}
	}
	return nil
}

func (syncer *Syncer) minerIsValid(ctx context.Context, maddr address.Address, baseTs *types.TipSet) error {
	var err error
	enc, err := actors.SerializeParams(&actors.IsMinerParam{Addr: maddr})
	if err != nil {
		return err
	}

	ret, err := syncer.sm.Call(ctx, &types.Message{
		To:     actors.StorageMarketAddress,
		From:   maddr,
		Method: actors.SMAMethods.IsMiner,
		Params: enc,
	}, baseTs)
	if err != nil {
		return xerrors.Errorf("checking if block miner is valid failed: %w", err)
	}

	if ret.ExitCode != 0 {
		return xerrors.Errorf("StorageMarket.IsMiner check failed (exit code %d)", ret.ExitCode)
	}

	// TODO: ensure the miner is currently not late on their PoSt submission (this hasnt landed in the spec yet)

	return nil
}

func (syncer *Syncer) validateTickets(ctx context.Context, mworker address.Address, tickets []*types.Ticket, base *types.TipSet) error {
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

		if err := sig.Verify(mworker, cur.VDFResult); err != nil {
			return xerrors.Errorf("invalid ticket, VRFProof invalid: %w", err)
		}

		// now verify the VDF
		if err := vdf.Verify(next.VRFProof, next.VDFResult, next.VDFProof); err != nil {
			return xerrors.Errorf("ticket %d had invalid VDF: %w", err)
		}

		cur = next
	}

	return nil
}

// Should match up with 'Semantical Validation' in validation.md in the spec
func (syncer *Syncer) ValidateBlock(ctx context.Context, b *types.FullBlock) error {
	h := b.Header
	stateroot, err := syncer.sm.TipSetState(h.Parents)
	if err != nil {
		return xerrors.Errorf("get tipsetstate(%d, %s) failed: %w", h.Height, h.Parents, err)
	}

	baseTs, err := syncer.store.LoadTipSet(h.Parents)
	if err != nil {
		return xerrors.Errorf("load tipset failed: %w", err)
	}

	if h.Timestamp > uint64(time.Now().Unix()+build.AllowableClockDrift) {
		return xerrors.Errorf("block was from the future")
	}

	if h.Timestamp < baseTs.MinTimestamp()+uint64(build.BlockDelay*len(h.Tickets)) {
		log.Warn("timestamp funtimes: ", h.Timestamp, baseTs.MinTimestamp(), len(h.Tickets))
		return xerrors.Errorf("block was generated too soon (h.ts:%d < base.mints:%d + BLOCK_DELAY:%d * tkts.len:%d)", h.Timestamp, baseTs.MinTimestamp(), build.BlockDelay, len(h.Tickets))
	}

	if err := syncer.minerIsValid(ctx, h.Miner, baseTs); err != nil {
		return xerrors.Errorf("minerIsValid failed: %w", err)
	}

	waddr, err := stmgr.GetMinerWorker(ctx, syncer.sm, stateroot, h.Miner)
	if err != nil {
		return xerrors.Errorf("GetMinerWorker failed: %w", err)
	}

	if err := h.CheckBlockSignature(waddr); err != nil {
		return xerrors.Errorf("check block signature failed: %w", err)
	}

	if err := syncer.validateTickets(ctx, waddr, h.Tickets, baseTs); err != nil {
		return xerrors.Errorf("validating block tickets failed: %w", err)
	}

	rand, err := syncer.sm.ChainStore().GetRandomness(ctx, baseTs.Cids(), h.Tickets, build.RandomnessLookback)
	if err != nil {
		return xerrors.Errorf("failed to get randomness for verifying election proof: %w", err)
	}

	if err := VerifyElectionProof(ctx, h.ElectionProof, rand, waddr); err != nil {
		return xerrors.Errorf("checking eproof failed: %w", err)
	}

	mpow, tpow, err := stmgr.GetPower(ctx, syncer.sm, baseTs, h.Miner)
	if err != nil {
		return xerrors.Errorf("failed getting power: %w", err)
	}

	if !types.PowerCmp(h.ElectionProof, mpow, tpow) {
		return xerrors.Errorf("miner created a block but was not a winner")
	}

	r := vm.NewChainRand(syncer.store, baseTs.Cids(), baseTs.Height(), h.Tickets)
	vmi, err := vm.NewVM(stateroot, h.Height, r, h.Miner, syncer.store)
	if err != nil {
		return xerrors.Errorf("failed to instantiate VM: %w", err)
	}

	owner, err := stmgr.GetMinerOwner(ctx, syncer.sm, stateroot, b.Header.Miner)
	if err != nil {
		return xerrors.Errorf("getting miner owner for block miner failed: %w", err)
	}

	if err := vmi.TransferFunds(actors.NetworkAddress, owner, vm.MiningRewardForBlock(baseTs)); err != nil {
		return xerrors.Errorf("fund transfer failed: %w", err)
	}

	var receipts []cbg.CBORMarshaler
	for i, m := range b.BlsMessages {
		receipt, err := vmi.ApplyMessage(ctx, m)
		if err != nil {
			return xerrors.Errorf("failed executing bls message %d in block %s: %w", i, b.Header.Cid(), err)
		}

		receipts = append(receipts, &receipt.MessageReceipt)
	}

	for i, m := range b.SecpkMessages {
		receipt, err := vmi.ApplyMessage(ctx, &m.Message)
		if err != nil {
			return xerrors.Errorf("failed executing secpk message %d in block %s: %w", i, b.Header.Cid(), err)
		}

		receipts = append(receipts, &receipt.MessageReceipt)
	}

	bs := amt.WrapBlockstore(syncer.store.Blockstore())
	recptRoot, err := amt.FromArray(bs, receipts)
	if err != nil {
		return xerrors.Errorf("building receipts amt failed: %w", err)
	}
	if recptRoot != b.Header.MessageReceipts {
		return fmt.Errorf("receipts mismatched")
	}

	final, err := vmi.Flush(context.TODO())
	if err != nil {
		return xerrors.Errorf("failed to flush VM state: %w", err)
	}

	if b.Header.StateRoot != final {
		return fmt.Errorf("final state root does not match block")
	}

	return nil
}

func (syncer *Syncer) collectHeaders(from *types.TipSet, to *types.TipSet) ([]*types.TipSet, error) {
	blockSet := []*types.TipSet{from}

	at := from.Parents()

	// we want to sync all the blocks until the height above the block we have
	untilHeight := to.Height() + 1

	// If, for some reason, we have a suffix of the chain locally, handle that here
	for blockSet[len(blockSet)-1].Height() > untilHeight {
		log.Warn("syncing local: ", at)
		ts, err := syncer.store.LoadTipSet(at)
		if err != nil {
			if err == bstore.ErrNotFound {
				log.Info("tipset not found locally, starting sync: ", at)
				break
			}
			log.Warn("loading local tipset: %s", err)
			continue // TODO: verify
		}

		blockSet = append(blockSet, ts)
		at = ts.Parents()
	}

loop:
	for blockSet[len(blockSet)-1].Height() > untilHeight {
		// NB: GetBlocks validates that the blocks are in-fact the ones we
		// requested, and that they are correctly linked to eachother. It does
		// not validate any state transitions
		window := 10
		if gap := int(blockSet[len(blockSet)-1].Height() - untilHeight); gap < window {
			window = gap
		}
		blks, err := syncer.Bsync.GetBlocks(context.TODO(), at, window)
		if err != nil {
			// Most likely our peers aren't fully synced yet, but forwarded
			// new block message (ideally we'd find better peers)

			log.Error("failed to get blocks: ", err)

			// This error will only be logged above,
			return nil, xerrors.Errorf("failed to get blocks: %w", err)
		}

		for _, b := range blks {
			if b.Height() < untilHeight {
				break loop
			}
			blockSet = append(blockSet, b)
		}

		at = blks[len(blks)-1].Parents()
	}

	// We have now ascertained that this is *not* a 'fast forward'
	if !types.CidArrsEqual(blockSet[len(blockSet)-1].Parents(), to.Cids()) {
		last := blockSet[len(blockSet)-1]
		if types.CidArrsEqual(last.Parents(), to.Parents()) {
			// common case: receiving a block thats potentially part of the same tipset as our best block
			return blockSet, nil
		}

		// TODO: handle the case where we are on a fork and its not a simple fast forward
		// need to walk back to either a common ancestor, or until we hit the fork length threshold.
		return nil, xerrors.Errorf("(fork detected) synced header chain (%s - %d) does not link to our best block (%s - %d)", from.Cids(), from.Height(), to.Cids(), to.Height())

	}

	return blockSet, nil
}

func (syncer *Syncer) syncMessagesAndCheckState(headers []*types.TipSet) error {
	return syncer.iterFullTipsets(headers, func(fts *store.FullTipSet) error {
		log.Debugf("validating tipset (heigt=%d, size=%d)", fts.TipSet().Height(), len(fts.TipSet().Cids()))
		if err := syncer.ValidateTipSet(context.TODO(), fts); err != nil {
			log.Errorf("failed to validate tipset: %s", err)
			return xerrors.Errorf("message processing failed: %w", err)
		}

		return nil
	})
}

// fills out each of the given tipsets with messages and calls the callback with it
func (syncer *Syncer) iterFullTipsets(headers []*types.TipSet, cb func(*store.FullTipSet) error) error {
	beg := len(headers) - 1
	// handle case where we have a prefix of these locally
	for ; beg >= 0; beg-- {
		fts, err := syncer.store.TryFillTipSet(headers[beg])
		if err != nil {
			return err
		}
		if fts == nil {
			break
		}
		if err := cb(fts); err != nil {
			return err
		}
	}
	headers = headers[:beg+1]

	windowSize := 10

	for i := len(headers) - 1; i >= 0; i -= windowSize {

		batchSize := windowSize
		if i < batchSize {
			batchSize = i
		}

		next := headers[i-batchSize]
		bstips, err := syncer.Bsync.GetChainMessages(context.TODO(), next, uint64(batchSize+1))
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
				log.Warn("zipping failed: ", err, bsi, i)
				log.Warn("height: ", this.Height())
				log.Warn("bstip height: ", bstip.Blocks[0].Height)
				log.Warn("bstips: ", bstips)
				log.Warn("next height: ", i+batchSize)
				return xerrors.Errorf("message processing failed: %w", err)
			}

			if err := cb(fts); err != nil {
				return err
			}

			if err := persistMessages(bs, bstip); err != nil {
				return err
			}

			if err := copyBlockstore(bs, syncer.store.Blockstore()); err != nil {
				return xerrors.Errorf("message processing failed: %w", err)
			}
		}

	}

	return nil
}

func persistMessages(bs bstore.Blockstore, bst *BSTipSet) error {
	for _, m := range bst.BlsMessages {
		//log.Infof("putting BLS message: %s", m.Cid())
		if _, err := store.PutMessage(bs, m); err != nil {
			log.Error("failed to persist messages: ", err)
			return xerrors.Errorf("BLS message processing failed: %w", err)
		}
	}
	for _, m := range bst.SecpkMessages {
		if m.Signature.Type != types.KTSecp256k1 {
			return xerrors.Errorf("unknown signature type on message %s: %q", m.Cid(), m.Signature.TypeCode)
		}
		//log.Infof("putting secp256k1 message: %s", m.Cid())
		if _, err := store.PutMessage(bs, m); err != nil {
			log.Error("failed to persist messages: ", err)
			return xerrors.Errorf("secp256k1 message processing failed: %w", err)
		}
	}

	return nil
}

func (syncer *Syncer) collectChain(fts *store.FullTipSet) error {
	headers, err := syncer.collectHeaders(fts.TipSet(), syncer.store.GetHeaviestTipSet())
	if err != nil {
		return err
	}

	for _, ts := range headers {
		for _, b := range ts.Blocks() {
			if err := syncer.store.PersistBlockHeader(b); err != nil {
				return xerrors.Errorf("failed to persist synced blocks to the chainstore: %w", err)
			}
		}
	}

	if err := syncer.syncMessagesAndCheckState(headers); err != nil {
		return xerrors.Errorf("collectChain syncMessages: %w", err)
	}

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

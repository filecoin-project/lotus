package chain

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"

	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/sharray"
)

const ForkLengthThreshold = 20

var log = logging.Logger("chain")

type Syncer struct {
	// The heaviest known tipset in the network.

	// The interface for accessing and putting tipsets into local storage
	store *store.ChainStore

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

func NewSyncer(cs *store.ChainStore, bsync *BlockSync, self peer.ID) (*Syncer, error) {
	gen, err := cs.GetGenesis()
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
		store: cs,
		self:  self,
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
		log.Infof("got block from ourselves")

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

func zipTipSetAndMessages(cst *hamt.CborIpldStore, ts *types.TipSet, messages []*types.SignedMessage, msgincl [][]int) (*store.FullTipSet, error) {
	if len(ts.Blocks()) != len(msgincl) {
		return nil, fmt.Errorf("msgincl length didnt match tipset size")
	}

	fts := &store.FullTipSet{}
	for bi, b := range ts.Blocks() {
		var msgs []*types.SignedMessage
		var msgCids []interface{}
		for _, m := range msgincl[bi] {
			msgs = append(msgs, messages[m])
			msgCids = append(msgCids, messages[m].Cid())
		}

		mroot, err := sharray.Build(context.TODO(), 4, msgCids, cst)
		if err != nil {
			return nil, err
		}

		if b.Messages != mroot {
			return nil, fmt.Errorf("messages didnt match message root in header")
		}

		fb := &types.FullBlock{
			Header:   b,
			Messages: msgs,
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

		if sel.Height()-nca.Height() > ForkLengthThreshold {
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
		messages, err := syncer.store.MessagesForBlock(b)
		if err != nil {
			return nil, err
		}

		fb := &types.FullBlock{
			Header:   b,
			Messages: messages,
		}
		fts.Blocks = append(fts.Blocks, fb)
	}

	return fts, nil
}

func (syncer *Syncer) Sync(maybeHead *store.FullTipSet) error {
	syncer.syncLock.Lock()
	defer syncer.syncLock.Unlock()

	ts := maybeHead.TipSet()
	if syncer.Genesis.Equals(ts) {
		return nil
	}

	if err := syncer.collectChain(maybeHead); err != nil {
		return err
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
			return err
		}
	}
	return nil
}

func (syncer *Syncer) ValidateBlock(ctx context.Context, b *types.FullBlock) error {
	h := b.Header
	stateroot, err := syncer.store.TipSetState(h.Parents)
	if err != nil {
		log.Error("get tipsetstate failed: ", h.Height, h.Parents, err)
		return err
	}
	baseTs, err := syncer.store.LoadTipSet(b.Header.Parents)
	if err != nil {
		return err
	}

	vmi, err := vm.NewVM(stateroot, b.Header.Height, b.Header.Miner, syncer.store)
	if err != nil {
		return err
	}

	if err := vmi.TransferFunds(actors.NetworkAddress, b.Header.Miner, vm.MiningRewardForBlock(baseTs)); err != nil {
		return err
	}

	var receipts []interface{}
	for _, m := range b.Messages {
		receipt, err := vmi.ApplyMessage(ctx, &m.Message)
		if err != nil {
			return err
		}

		receipts = append(receipts, receipt)
	}

	cst := hamt.CSTFromBstore(syncer.store.Blockstore())
	recptRoot, err := sharray.Build(context.TODO(), 4, receipts, cst)
	if err != nil {
		return err
	}
	if recptRoot != b.Header.MessageReceipts {
		return fmt.Errorf("receipts mismatched")
	}

	final, err := vmi.Flush(context.TODO())
	if err != nil {
		return err
	}

	if b.Header.StateRoot != final {
		return fmt.Errorf("final state root does not match block")
	}

	return nil

}

func (syncer *Syncer) collectHeaders(from *types.TipSet, toHeight uint64) ([]*types.TipSet, error) {
	blockSet := []*types.TipSet{from}

	at := from.Parents()

	// If, for some reason, we have a suffix of the chain locally, handle that here
	for blockSet[len(blockSet)-1].Height() > toHeight {
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

	for blockSet[len(blockSet)-1].Height() > toHeight {
		// NB: GetBlocks validates that the blocks are in-fact the ones we
		// requested, and that they are correctly linked to eachother. It does
		// not validate any state transitions
		fmt.Println("Get blocks")
		blks, err := syncer.Bsync.GetBlocks(context.TODO(), at, 10)
		if err != nil {
			// Most likely our peers aren't fully synced yet, but forwarded
			// new block message (ideally we'd find better peers)

			log.Error("failed to get blocks: ", err)

			// This error will only be logged above,
			return nil, xerrors.Errorf("failed to get blocks: %w", err)
		}

		for _, b := range blks {
			blockSet = append(blockSet, b)
		}

		at = blks[len(blks)-1].Parents()
	}

	if toHeight == 0 {
		// hacks. in the case that we request X blocks starting at height X+1, we
		// won't get the Genesis block in the returned blockset. This hacks around it
		if blockSet[len(blockSet)-1].Height() != 0 {
			blockSet = append(blockSet, syncer.Genesis)
		}

		blockSet = reverse(blockSet)

		genesis := blockSet[0]
		if !genesis.Equals(syncer.Genesis) {
			// TODO: handle this...
			log.Errorf("We synced to the wrong chain! %s != %s", genesis, syncer.Genesis)
			panic("We synced to the wrong chain")
		}
	}

	return blockSet, nil
}

func (syncer *Syncer) syncMessagesAndCheckState(headers []*types.TipSet) error {
	return syncer.iterFullTipsets(headers, func(fts *store.FullTipSet) error {
		if err := syncer.ValidateTipSet(context.TODO(), fts); err != nil {
			log.Errorf("failed to validate tipset: %s", err)
			return xerrors.Errorf("message processing failed: %w", err)
		}
		return nil
	})
}

// fills out each of the given tipsets with messages and calls the callback with it
func (syncer *Syncer) iterFullTipsets(headers []*types.TipSet, cb func(*store.FullTipSet) error) error {
	var beg int
	// handle case where we have a prefix of these locally
	for ; beg < len(headers); beg++ {
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
	headers = headers[beg:]

	windowSize := 10

	for i := 0; i < len(headers); i += windowSize {
		// temp storage so we don't persist data we dont want to
		ds := dstore.NewMapDatastore()
		bs := bstore.NewBlockstore(ds)
		cst := hamt.CSTFromBstore(bs)

		batchSize := windowSize
		if i+batchSize >= len(headers) {
			batchSize = (len(headers) - i) - 1
		}

		next := headers[i+batchSize]
		bstips, err := syncer.Bsync.GetChainMessages(context.TODO(), next, uint64(batchSize+1))
		if err != nil {
			log.Errorf("failed to fetch messages: %s", err)
			return xerrors.Errorf("message processing failed: %w", err)
		}

		for bsi := 0; bsi < len(bstips); bsi++ {
			this := headers[i+bsi]
			bstip := bstips[len(bstips)-(bsi+1)]
			fts, err := zipTipSetAndMessages(cst, this, bstip.Messages, bstip.MsgIncludes)
			if err != nil {
				log.Error("zipping failed: ", err, bsi, i)
				log.Error("height: ", this.Height())
				log.Error("bstip height: ", bstip.Blocks[0].Height)
				log.Error("bstips: ", bstips)
				log.Error("next height: ", i+batchSize)
				return xerrors.Errorf("message processing failed: %w", err)
			}

			if err := cb(fts); err != nil {
				return err
			}
		}

		if err := persistMessages(bs, bstips); err != nil {
			return err
		}

		if err := copyBlockstore(bs, syncer.store.Blockstore()); err != nil {
			return xerrors.Errorf("message processing failed: %w", err)
		}
	}

	return nil
}

func persistMessages(bs bstore.Blockstore, bstips []*BSTipSet) error {
	for _, bst := range bstips {
		for _, m := range bst.Messages {
			switch m.Signature.Type {
			case types.KTBLS:
				//log.Infof("putting BLS message: %s", m.Cid())
				if _, err := store.PutMessage(bs, &m.Message); err != nil {
					log.Error("failed to persist messages: ", err)
					return xerrors.Errorf("BLS message processing failed: %w", err)
				}
			case types.KTSecp256k1:
				//log.Infof("putting secp256k1 message: %s", m.Cid())
				if _, err := store.PutMessage(bs, m); err != nil {
					log.Error("failed to persist messages: ", err)
					return xerrors.Errorf("secp256k1 message processing failed: %w", err)
				}
			default:
				return xerrors.Errorf("unknown signature type on message %s: %q", m.Cid(), m.Signature.TypeCode)
			}
		}
	}

	return nil
}

func (syncer *Syncer) collectChain(fts *store.FullTipSet) error {
	curHeight := syncer.store.GetHeaviestTipSet().Height()

	headers, err := syncer.collectHeaders(fts.TipSet(), curHeight)
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
		return err
	}

	return nil
}

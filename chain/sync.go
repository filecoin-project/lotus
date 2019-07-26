package chain

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/chain/vm"

	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/sharray"
)

const ForkLengthThreshold = 20

var log = logging.Logger("chain")

type Syncer struct {
	// The heaviest known tipset in the network.
	head *types.TipSet

	// The interface for accessing and putting tipsets into local storage
	store *store.ChainStore

	// The known Genesis tipset
	Genesis *types.TipSet

	// the current mode the syncer is in
	syncMode SyncMode

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
		syncMode:  Bootstrap,
		Genesis:   gent,
		Bsync:     bsync,
		peerHeads: make(map[peer.ID]*types.TipSet),
		head:      cs.GetHeaviestTipSet(),
		store:     cs,
		self:      self,
	}, nil
}

type SyncMode int

const (
	Unknown = SyncMode(iota)
	Bootstrap
	CaughtUp
)

type BadTipSetCache struct {
	badBlocks map[cid.Cid]struct{}
}

type BlockSet struct {
	tset map[uint64]*types.TipSet
	head *types.TipSet
}

func (bs *BlockSet) Insert(ts *types.TipSet) {
	if bs.tset == nil {
		bs.tset = make(map[uint64]*types.TipSet)
	}

	if bs.head == nil || ts.Height() > bs.head.Height() {
		bs.head = ts
	}
	bs.tset[ts.Height()] = ts
}

func (bs *BlockSet) GetByHeight(h uint64) *types.TipSet {
	return bs.tset[h]
}

func (bs *BlockSet) PersistTo(cs *store.ChainStore) error {
	for _, ts := range bs.tset {
		for _, b := range ts.Blocks() {
			if err := cs.PersistBlockHeader(b); err != nil {
				return err
			}
		}
	}
	return nil
}

func (bs *BlockSet) Head() *types.TipSet {
	return bs.head
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
		syncer.syncLock.Lock()
		defer syncer.syncLock.Unlock()

		if syncer.syncMode == Bootstrap {
			syncer.syncMode = CaughtUp
		}
		if err := syncer.SyncCaughtUp(fts); err != nil {
			log.Errorf("failed to sync our own block: %s", err)
		}

		return
	}
	syncer.peerHeadsLk.Lock()
	syncer.peerHeads[from] = fts.TipSet()
	syncer.peerHeadsLk.Unlock()
	syncer.Bsync.AddPeer(from)

	go func() {
		syncer.syncLock.Lock()
		defer syncer.syncLock.Unlock()

		switch syncer.syncMode {
		case Bootstrap:
			syncer.SyncBootstrap()
		case CaughtUp:
			if err := syncer.SyncCaughtUp(fts); err != nil {
				log.Errorf("sync error: %s", err)
			}
		case Unknown:
			panic("invalid syncer state")
		}
	}()
}

func (syncer *Syncer) GetPeers() []peer.ID {
	syncer.peerHeadsLk.Lock()
	defer syncer.peerHeadsLk.Unlock()
	var out []peer.ID
	for p, _ := range syncer.peerHeads {
		out = append(out, p)
	}
	return out
}

func (syncer *Syncer) InformNewBlock(from peer.ID, blk *types.FullBlock) {
	// TODO: search for other blocks that could form a tipset with this block
	// and then send that tipset to InformNewHead

	fts := &store.FullTipSet{Blocks: []*types.FullBlock{blk}}
	syncer.InformNewHead(from, fts)
}

// SyncBootstrap is used to synchronise your chain when first joining
// the network, or when rejoining after significant downtime.
func (syncer *Syncer) SyncBootstrap() {
	fmt.Println("Sync bootstrap!")
	defer fmt.Println("bye bye sync bootstrap")
	ctx := context.Background()

	if syncer.syncMode == CaughtUp {
		log.Errorf("Called SyncBootstrap while in caught up mode")
		return
	}

	selectedHead, err := syncer.selectHead(syncer.peerHeads)
	if err != nil {
		log.Error("failed to select head: ", err)
		return
	}

	blockSet := []*types.TipSet{selectedHead}
	cur := selectedHead.Cids()

	// If, for some reason, we have a suffix of the chain locally, handle that here
	for blockSet[len(blockSet)-1].Height() > 0 {
		log.Errorf("syncing local: ", cur)
		ts, err := syncer.store.LoadTipSet(cur)
		if err != nil {
			if err == bstore.ErrNotFound {
				log.Error("not found: ", cur)
				break
			}
			log.Errorf("loading local tipset: %s", err)
			return
		}

		blockSet = append(blockSet, ts)
		cur = ts.Parents()
	}

	for blockSet[len(blockSet)-1].Height() > 0 {
		// NB: GetBlocks validates that the blocks are in-fact the ones we
		// requested, and that they are correctly linked to eachother. It does
		// not validate any state transitions
		fmt.Println("Get blocks: ", cur)
		blks, err := syncer.Bsync.GetBlocks(context.TODO(), cur, 10)
		if err != nil {
			log.Error("failed to get blocks: ", err)
			return
		}

		for _, b := range blks {
			blockSet = append(blockSet, b)
		}

		cur = blks[len(blks)-1].Parents()
	}

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
		return
	}

	for _, ts := range blockSet {
		for _, b := range ts.Blocks() {
			if err := syncer.store.PersistBlockHeader(b); err != nil {
				log.Errorf("failed to persist synced blocks to the chainstore: %s", err)
				return
			}
		}
	}

	// Fetch all the messages for all the blocks in this chain

	windowSize := uint64(10)
	for i := uint64(0); i <= selectedHead.Height(); i += windowSize {
		bs := bstore.NewBlockstore(dstore.NewMapDatastore())
		cst := hamt.CSTFromBstore(bs)

		nextHeight := i + windowSize - 1
		if nextHeight > selectedHead.Height() {
			nextHeight = selectedHead.Height()
		}

		next := blockSet[nextHeight]
		bstips, err := syncer.Bsync.GetChainMessages(ctx, next, (nextHeight+1)-i)
		if err != nil {
			log.Errorf("failed to fetch messages: %s", err)
			return
		}

		for bsi := 0; bsi < len(bstips); bsi++ {
			cur := blockSet[i+uint64(bsi)]
			bstip := bstips[len(bstips)-(bsi+1)]
			fmt.Println("that loop: ", bsi, len(bstips))
			fts, err := zipTipSetAndMessages(cst, cur, bstip.Messages, bstip.MsgIncludes)
			if err != nil {
				log.Error("zipping failed: ", err, bsi, i)
				log.Error("height: ", selectedHead.Height())
				log.Error("bstips: ", bstips)
				log.Error("next height: ", nextHeight)
				return
			}

			if err := syncer.ValidateTipSet(context.TODO(), fts); err != nil {
				log.Errorf("failed to validate tipset: %s", err)
				return
			}
		}

		for _, bst := range bstips {
			for _, m := range bst.Messages {
				if _, err := cst.Put(context.TODO(), m); err != nil {
					log.Error("failed to persist messages: ", err)
					return
				}
			}
		}

		if err := copyBlockstore(bs, syncer.store.Blockstore()); err != nil {
			log.Errorf("failed to persist temp blocks: %s", err)
			return
		}
	}

	head := blockSet[len(blockSet)-1]
	log.Errorf("Finished syncing! new head: %s", head.Cids())
	syncer.store.MaybeTakeHeavierTipSet(selectedHead)
	syncer.head = head
	syncer.syncMode = CaughtUp
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
	fmt.Println("zipping messages: ", msgincl)
	fmt.Println("into block: ", ts.Blocks()[0].Height)

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

		fmt.Println("messages: ", msgCids)
		fmt.Println("message root: ", b.Messages, mroot)
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

// SyncCaughtUp is used to stay in sync once caught up to
// the rest of the network.
func (syncer *Syncer) SyncCaughtUp(maybeHead *store.FullTipSet) error {
	ts := maybeHead.TipSet()
	if syncer.Genesis.Equals(ts) {
		return nil
	}

	chain, err := syncer.collectChainCaughtUp(maybeHead)
	if err != nil {
		return err
	}

	for i := len(chain) - 1; i >= 0; i-- {
		ts := chain[i]
		if err := syncer.ValidateTipSet(context.TODO(), ts); err != nil {
			return errors.Wrap(err, "validate tipset failed")
		}

		syncer.store.PutTipSet(ts)
	}

	if err := syncer.store.PutTipSet(maybeHead); err != nil {
		return errors.Wrap(err, "failed to put synced tipset to chainstore")
	}

	if syncer.store.Weight(chain[0].TipSet()) > syncer.store.Weight(syncer.head) {
		fmt.Println("Accepted new head: ", chain[0].Cids())
		syncer.head = chain[0].TipSet()
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

	vm, err := vm.NewVM(stateroot, b.Header.Height, b.Header.Miner, syncer.store)
	if err != nil {
		return err
	}

	if err := vm.TransferFunds(actors.NetworkAddress, b.Header.Miner, miningRewardForBlock(baseTs)); err != nil {
		return err
	}

	var receipts []interface{}
	for _, m := range b.Messages {
		receipt, err := vm.ApplyMessage(ctx, &m.Message)
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

	final, err := vm.Flush(context.TODO())
	if err != nil {
		return err
	}

	if b.Header.StateRoot != final {
		return fmt.Errorf("final state root does not match block")
	}

	return nil

}

func (syncer *Syncer) Punctual(ts *types.TipSet) bool {
	return true
}

func (syncer *Syncer) collectChainCaughtUp(fts *store.FullTipSet) ([]*store.FullTipSet, error) {
	// fetch tipset and messages via bitswap

	chain := []*store.FullTipSet{fts}
	cur := fts.TipSet()

	for {
		ts, err := syncer.store.LoadTipSet(cur.Parents())
		if err != nil {
			log.Errorf("dont have parent blocks for sync tipset: %s", err)
			panic("should do something better, like fetch? or error?")
		}

		return chain, nil // return the chain because we have this last block in our cache already.

		if ts.Equals(syncer.Genesis) {
			break
		}

		/*
			if !syncer.Punctual(ts) {
				syncer.bad.InvalidateChain(chain)
				syncer.bad.InvalidateTipSet(ts)
				return nil, errors.New("tipset forks too far back from head")
			}
		*/

		chain = append(chain, fts)
		log.Error("received unknown chain in caught up mode...")
		panic("for now, we panic...")

		has, err := syncer.store.Contains(ts)
		if err != nil {
			return nil, err
		}
		if has {
			// Store has record of this tipset.
			return chain, nil
		}

		/*
			parent, err := syncer.FetchTipSet(context.TODO(), ts.Parents())
			if err != nil {
				return nil, err
			}
			ts = parent
		*/
	}

	return chain, nil
}

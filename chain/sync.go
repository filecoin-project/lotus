package chain

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
	"github.com/whyrusleeping/sharray"
)

type Syncer struct {
	// The heaviest known tipset in the network.
	head *TipSet

	// The interface for accessing and putting tipsets into local storage
	store *ChainStore

	// The known Genesis tipset
	Genesis *TipSet

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
	peerHeads   map[peer.ID]*TipSet
	peerHeadsLk sync.Mutex
}

func NewSyncer(cs *ChainStore, bsync *BlockSync, self peer.ID) (*Syncer, error) {
	gen, err := cs.GetGenesis()
	if err != nil {
		return nil, err
	}

	gent, err := NewTipSet([]*types.BlockHeader{gen})
	if err != nil {
		return nil, err
	}

	return &Syncer{
		syncMode:  Bootstrap,
		Genesis:   gent,
		Bsync:     bsync,
		peerHeads: make(map[peer.ID]*TipSet),
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
	tset map[uint64]*TipSet
	head *TipSet
}

func (bs *BlockSet) Insert(ts *TipSet) {
	if bs.tset == nil {
		bs.tset = make(map[uint64]*TipSet)
	}

	if bs.head == nil || ts.Height() > bs.head.Height() {
		bs.head = ts
	}
	bs.tset[ts.Height()] = ts
}

func (bs *BlockSet) GetByHeight(h uint64) *TipSet {
	return bs.tset[h]
}

func (bs *BlockSet) PersistTo(cs *ChainStore) error {
	for _, ts := range bs.tset {
		for _, b := range ts.Blocks() {
			if err := cs.persistBlockHeader(b); err != nil {
				return err
			}
		}
	}
	return nil
}

func (bs *BlockSet) Head() *TipSet {
	return bs.head
}

const BootstrapPeerThreshold = 1

// InformNewHead informs the syncer about a new potential tipset
// This should be called when connecting to new peers, and additionally
// when receiving new blocks from the network
func (syncer *Syncer) InformNewHead(from peer.ID, fts *FullTipSet) {
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

	fts := &FullTipSet{Blocks: []*types.FullBlock{blk}}
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

	blockSet := []*TipSet{selectedHead}
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
			if err := syncer.store.persistBlockHeader(b); err != nil {
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

			if err := syncer.ValidateTipSet(fts); err != nil {
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

		if err := copyBlockstore(bs, syncer.store.bs); err != nil {
			log.Errorf("failed to persist temp blocks: %s", err)
			return
		}
	}

	head := blockSet[len(blockSet)-1]
	log.Errorf("Finished syncing! new head: %s", head.Cids())
	syncer.store.maybeTakeHeavierTipSet(selectedHead)
	syncer.head = head
	syncer.syncMode = CaughtUp
}

func reverse(tips []*TipSet) []*TipSet {
	out := make([]*TipSet, len(tips))
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

func zipTipSetAndMessages(cst *hamt.CborIpldStore, ts *TipSet, messages []*types.SignedMessage, msgincl [][]int) (*FullTipSet, error) {
	if len(ts.Blocks()) != len(msgincl) {
		return nil, fmt.Errorf("msgincl length didnt match tipset size")
	}
	fmt.Println("zipping messages: ", msgincl)
	fmt.Println("into block: ", ts.Blocks()[0].Height)

	fts := &FullTipSet{}
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

func (syncer *Syncer) selectHead(heads map[peer.ID]*TipSet) (*TipSet, error) {
	var headsArr []*TipSet
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

func (syncer *Syncer) FetchTipSet(ctx context.Context, p peer.ID, cids []cid.Cid) (*FullTipSet, error) {
	if fts, err := syncer.tryLoadFullTipSet(cids); err == nil {
		return fts, nil
	}

	return syncer.Bsync.GetFullTipSet(ctx, p, cids)
}

func (syncer *Syncer) tryLoadFullTipSet(cids []cid.Cid) (*FullTipSet, error) {
	ts, err := syncer.store.LoadTipSet(cids)
	if err != nil {
		return nil, err
	}

	fts := &FullTipSet{}
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

// FullTipSet is an expanded version of the TipSet that contains all the blocks and messages
type FullTipSet struct {
	Blocks []*types.FullBlock
	tipset *TipSet
	cids   []cid.Cid
}

func NewFullTipSet(blks []*types.FullBlock) *FullTipSet {
	return &FullTipSet{
		Blocks: blks,
	}
}

func (fts *FullTipSet) Cids() []cid.Cid {
	if fts.cids != nil {
		return fts.cids
	}

	var cids []cid.Cid
	for _, b := range fts.Blocks {
		cids = append(cids, b.Cid())
	}
	fts.cids = cids

	return cids
}

func (fts *FullTipSet) TipSet() *TipSet {
	if fts.tipset != nil {
		return fts.tipset
	}

	var headers []*types.BlockHeader
	for _, b := range fts.Blocks {
		headers = append(headers, b.Header)
	}

	ts, err := NewTipSet(headers)
	if err != nil {
		panic(err)
	}

	return ts
}

// SyncCaughtUp is used to stay in sync once caught up to
// the rest of the network.
func (syncer *Syncer) SyncCaughtUp(maybeHead *FullTipSet) error {
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
		if err := syncer.ValidateTipSet(ts); err != nil {
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

func (syncer *Syncer) ValidateTipSet(fts *FullTipSet) error {
	ts := fts.TipSet()
	if ts.Equals(syncer.Genesis) {
		return nil
	}

	for _, b := range fts.Blocks {
		if err := syncer.ValidateBlock(b); err != nil {
			return err
		}
	}
	return nil
}

func (syncer *Syncer) ValidateBlock(b *types.FullBlock) error {
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

	vm, err := NewVM(stateroot, b.Header.Height, b.Header.Miner, syncer.store)
	if err != nil {
		return err
	}

	if err := vm.TransferFunds(actors.NetworkAddress, b.Header.Miner, miningRewardForBlock(baseTs)); err != nil {
		return err
	}

	var receipts []interface{}
	for _, m := range b.Messages {
		receipt, err := vm.ApplyMessage(&m.Message)
		if err != nil {
			return err
		}

		receipts = append(receipts, receipt)
	}

	cst := hamt.CSTFromBstore(syncer.store.bs)
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

func DeductFunds(act *types.Actor, amt types.BigInt) error {
	if types.BigCmp(act.Balance, amt) < 0 {
		return fmt.Errorf("not enough funds")
	}

	act.Balance = types.BigSub(act.Balance, amt)
	return nil
}

func DepositFunds(act *types.Actor, amt types.BigInt) {
	act.Balance = types.BigAdd(act.Balance, amt)
}

func TryCreateAccountActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
	act, err := makeActor(st, addr)
	if err != nil {
		return nil, err
	}

	_, err = st.RegisterNewAddress(addr, act)
	if err != nil {
		return nil, err
	}

	return act, nil
}

func makeActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
	switch addr.Protocol() {
	case address.BLS:
		return NewBLSAccountActor(st, addr)
	case address.SECP256K1:
		return NewSecp256k1AccountActor(st, addr)
	case address.ID:
		return nil, fmt.Errorf("no actor with given ID")
	case address.Actor:
		return nil, fmt.Errorf("no such actor")
	default:
		return nil, fmt.Errorf("address has unsupported protocol: %d", addr.Protocol())
	}
}

func NewBLSAccountActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
	var acstate actors.AccountActorState
	acstate.Address = addr

	c, err := st.Store.Put(context.TODO(), acstate)
	if err != nil {
		return nil, err
	}

	nact := &types.Actor{
		Code:    actors.AccountActorCodeCid,
		Balance: types.NewInt(0),
		Head:    c,
	}

	return nact, nil
}

func NewSecp256k1AccountActor(st *state.StateTree, addr address.Address) (*types.Actor, error) {
	nact := &types.Actor{
		Code:    actors.AccountActorCodeCid,
		Balance: types.NewInt(0),
		Head:    gen.EmptyObjectCid,
	}

	return nact, nil
}

func (syncer *Syncer) Punctual(ts *TipSet) bool {
	return true
}

func (syncer *Syncer) collectChainCaughtUp(fts *FullTipSet) ([]*FullTipSet, error) {
	// fetch tipset and messages via bitswap

	chain := []*FullTipSet{fts}
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

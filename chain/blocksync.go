package chain

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"sync"

	bserv "github.com/ipfs/go-blockservice"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/protocol"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/cborrpc"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

type NewStreamFunc func(context.Context, peer.ID, ...protocol.ID) (inet.Stream, error)

const BlockSyncProtocolID = "/fil/sync/blk/0.0.1"

func init() {
	cbor.RegisterCborType(BlockSyncRequest{})
	cbor.RegisterCborType(BlockSyncResponse{})
	cbor.RegisterCborType(BSTipSet{})
}

type BlockSyncService struct {
	cs *store.ChainStore
}

type BlockSyncRequest struct {
	Start         []cid.Cid
	RequestLength uint64

	Options uint64
}

type BSOptions struct {
	IncludeBlocks   bool
	IncludeMessages bool
}

func ParseBSOptions(optfield uint64) *BSOptions {
	return &BSOptions{
		IncludeBlocks:   optfield&(BSOptBlocks) != 0,
		IncludeMessages: optfield&(BSOptMessages) != 0,
	}
}

const (
	BSOptBlocks   = 1 << 0
	BSOptMessages = 1 << 1
)

type BlockSyncResponse struct {
	Chain []*BSTipSet

	Status  uint
	Message string
}

type BSTipSet struct {
	Blocks []*types.BlockHeader

	Messages    []*types.SignedMessage
	MsgIncludes [][]int
}

func NewBlockSyncService(cs *store.ChainStore) *BlockSyncService {
	return &BlockSyncService{
		cs: cs,
	}
}

func (bss *BlockSyncService) HandleStream(s inet.Stream) {
	defer s.Close()

	var req BlockSyncRequest
	if err := cborrpc.ReadCborRPC(bufio.NewReader(s), &req); err != nil {
		log.Errorf("failed to read block sync request: %s", err)
		return
	}
	log.Infof("block sync request for: %s %d", req.Start, req.RequestLength)

	resp, err := bss.processRequest(&req)
	if err != nil {
		log.Error("failed to process block sync request: ", err)
		return
	}

	if err := cborrpc.WriteCborRPC(s, resp); err != nil {
		log.Error("failed to write back response for handle stream: ", err)
		return
	}
}

func (bss *BlockSyncService) processRequest(req *BlockSyncRequest) (*BlockSyncResponse, error) {
	opts := ParseBSOptions(req.Options)
	chain, err := bss.collectChainSegment(req.Start, req.RequestLength, opts)
	if err != nil {
		log.Error("encountered error while responding to block sync request: ", err)
		return &BlockSyncResponse{
			Status: 203,
		}, nil
	}

	return &BlockSyncResponse{
		Chain:  chain,
		Status: 0,
	}, nil
}

func (bss *BlockSyncService) collectChainSegment(start []cid.Cid, length uint64, opts *BSOptions) ([]*BSTipSet, error) {
	var bstips []*BSTipSet
	cur := start
	for {
		var bst BSTipSet
		ts, err := bss.cs.LoadTipSet(cur)
		if err != nil {
			return nil, err
		}

		if opts.IncludeMessages {
			msgs, mincl, err := bss.gatherMessages(ts)
			if err != nil {
				return nil, err
			}

			bst.Messages = msgs
			bst.MsgIncludes = mincl
		}

		if opts.IncludeBlocks {
			bst.Blocks = ts.Blocks()
		}

		bstips = append(bstips, &bst)

		if uint64(len(bstips)) >= length || ts.Height() == 0 {
			return bstips, nil
		}

		cur = ts.Parents()
	}
}

func (bss *BlockSyncService) gatherMessages(ts *types.TipSet) ([]*types.SignedMessage, [][]int, error) {
	msgmap := make(map[cid.Cid]int)
	var allmsgs []*types.SignedMessage
	var msgincl [][]int

	for _, b := range ts.Blocks() {
		msgs, err := bss.cs.MessagesForBlock(b)
		if err != nil {
			return nil, nil, err
		}
		log.Infof("MESSAGES FOR BLOCK: %d", len(msgs))

		msgindexes := make([]int, 0, len(msgs))
		for _, m := range msgs {
			i, ok := msgmap[m.Cid()]
			if !ok {
				i = len(allmsgs)
				allmsgs = append(allmsgs, m)
				msgmap[m.Cid()] = i
			}

			msgindexes = append(msgindexes, i)
		}
		msgincl = append(msgincl, msgindexes)
	}

	return allmsgs, msgincl, nil
}

type BlockSync struct {
	bserv     bserv.BlockService
	newStream NewStreamFunc

	syncPeersLk sync.Mutex
	syncPeers   map[peer.ID]struct{}
}

func NewBlockSyncClient(bserv bserv.BlockService, h host.Host) *BlockSync {
	return &BlockSync{
		bserv:     bserv,
		newStream: h.NewStream,
		syncPeers: make(map[peer.ID]struct{}),
	}
}

func (bs *BlockSync) getPeers() []peer.ID {
	bs.syncPeersLk.Lock()
	defer bs.syncPeersLk.Unlock()
	var out []peer.ID
	for p := range bs.syncPeers {
		out = append(out, p)
	}
	return out
}

func (bs *BlockSync) processStatus(req *BlockSyncRequest, res *BlockSyncResponse) ([]*types.TipSet, error) {
	switch res.Status {
	case 0: // Success
		return bs.processBlocksResponse(req, res)
	case 101: // Partial Response
		panic("not handled")
	case 201: // req.Start not found
		return nil, fmt.Errorf("not found")
	case 202: // Go Away
		panic("not handled")
	case 203: // Internal Error
		return nil, fmt.Errorf("block sync peer errored: %s", res.Message)
	default:
		return nil, fmt.Errorf("unrecognized response code")
	}
}

func (bs *BlockSync) GetBlocks(ctx context.Context, tipset []cid.Cid, count int) ([]*types.TipSet, error) {
	peers := bs.getPeers()
	perm := rand.Perm(len(peers))
	// TODO: round robin through these peers on error

	req := &BlockSyncRequest{
		Start:         tipset,
		RequestLength: uint64(count),
		Options:       BSOptBlocks,
	}

	var err error
	var res *BlockSyncResponse
	for _, p := range perm {
		res, err = bs.sendRequestToPeer(ctx, peers[p], req)
		if err != nil {
			log.Warnf("BlockSync request failed for peer %s: %s", peers[p].String(), err)
			continue
		}

		ts, err := bs.processStatus(req, res)
		if err == nil {
			return ts, nil
		}
	}
	return nil, xerrors.Errorf("GetBlocks failed with all peers: %w", err)
}

func (bs *BlockSync) GetFullTipSet(ctx context.Context, p peer.ID, h []cid.Cid) (*store.FullTipSet, error) {
	// TODO: round robin through these peers on error

	req := &BlockSyncRequest{
		Start:         h,
		RequestLength: 1,
		Options:       BSOptBlocks | BSOptMessages,
	}

	res, err := bs.sendRequestToPeer(ctx, p, req)
	if err != nil {
		return nil, err
	}

	switch res.Status {
	case 0: // Success
		if len(res.Chain) == 0 {
			return nil, fmt.Errorf("got zero length chain response")
		}
		bts := res.Chain[0]

		return bstsToFullTipSet(bts)
	case 101: // Partial Response
		panic("not handled")
	case 201: // req.Start not found
		return nil, fmt.Errorf("not found")
	case 202: // Go Away
		panic("not handled")
	case 203: // Internal Error
		return nil, fmt.Errorf("block sync peer errored: %s", res.Message)
	default:
		return nil, fmt.Errorf("unrecognized response code")
	}
}

func (bs *BlockSync) GetChainMessages(ctx context.Context, h *types.TipSet, count uint64) ([]*BSTipSet, error) {
	peers := bs.getPeers()
	perm := rand.Perm(len(peers))
	// TODO: round robin through these peers on error

	req := &BlockSyncRequest{
		Start:         h.Cids(),
		RequestLength: count,
		Options:       BSOptMessages,
	}

	res, err := bs.sendRequestToPeer(ctx, peers[perm[0]], req)
	if err != nil {
		return nil, err
	}

	switch res.Status {
	case 0: // Success
		return res.Chain, nil
	case 101: // Partial Response
		panic("not handled")
	case 201: // req.Start not found
		return nil, fmt.Errorf("not found")
	case 202: // Go Away
		panic("not handled")
	case 203: // Internal Error
		return nil, fmt.Errorf("block sync peer errored: %s", res.Message)
	default:
		return nil, fmt.Errorf("unrecognized response code")
	}
}

func bstsToFullTipSet(bts *BSTipSet) (*store.FullTipSet, error) {
	fts := &store.FullTipSet{}
	for i, b := range bts.Blocks {
		fb := &types.FullBlock{
			Header: b,
		}
		for _, mi := range bts.MsgIncludes[i] {
			fb.Messages = append(fb.Messages, bts.Messages[mi])
		}
		fts.Blocks = append(fts.Blocks, fb)
	}

	return fts, nil
}

func (bs *BlockSync) sendRequestToPeer(ctx context.Context, p peer.ID, req *BlockSyncRequest) (*BlockSyncResponse, error) {
	s, err := bs.newStream(inet.WithNoDial(ctx, "should already have connection"), p, BlockSyncProtocolID)
	if err != nil {
		return nil, err
	}

	if err := cborrpc.WriteCborRPC(s, req); err != nil {
		return nil, err
	}

	var res BlockSyncResponse
	if err := cborrpc.ReadCborRPC(bufio.NewReader(s), &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func (bs *BlockSync) processBlocksResponse(req *BlockSyncRequest, res *BlockSyncResponse) ([]*types.TipSet, error) {
	cur, err := types.NewTipSet(res.Chain[0].Blocks)
	if err != nil {
		return nil, err
	}

	out := []*types.TipSet{cur}
	for bi := 1; bi < len(res.Chain); bi++ {
		next := res.Chain[bi].Blocks
		nts, err := types.NewTipSet(next)
		if err != nil {
			return nil, err
		}

		if !cidArrsEqual(cur.Parents(), nts.Cids()) {
			return nil, fmt.Errorf("parents of tipset[%d] were not tipset[%d]", bi-1, bi)
		}

		out = append(out, nts)
		cur = nts
	}
	return out, nil
}

func cidArrsEqual(a, b []cid.Cid) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if b[i] != v {
			return false
		}
	}
	return true
}

func (bs *BlockSync) GetBlock(ctx context.Context, c cid.Cid) (*types.BlockHeader, error) {
	sb, err := bs.bserv.GetBlock(ctx, c)
	if err != nil {
		return nil, err
	}

	return types.DecodeBlock(sb.RawData())
}

func (bs *BlockSync) AddPeer(p peer.ID) {
	bs.syncPeersLk.Lock()
	defer bs.syncPeersLk.Unlock()
	bs.syncPeers[p] = struct{}{}
}

func (bs *BlockSync) FetchMessagesByCids(cids []cid.Cid) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, len(cids))

	resp := bs.bserv.GetBlocks(context.TODO(), cids)

	m := make(map[cid.Cid]int)
	for i, c := range cids {
		m[c] = i
	}

	for i := 0; i < len(cids); i++ {
		select {
		case v, ok := <-resp:
			if !ok {
				if i == len(cids)-1 {
					break
				}

				return nil, fmt.Errorf("failed to fetch all messages")
			}

			sm, err := types.DecodeSignedMessage(v.RawData())
			if err != nil {
				return nil, err
			}
			ix, ok := m[sm.Cid()]
			if !ok {
				return nil, fmt.Errorf("received message we didnt ask for")
			}

			if out[ix] != nil {
				return nil, fmt.Errorf("received duplicate message")
			}

			out[ix] = sm
		}
	}

	return out, nil
}

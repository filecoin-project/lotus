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
	"github.com/filecoin-project/go-lotus/node/modules/dtypes"

	blocks "github.com/ipfs/go-block-format"
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

	Status  uint64
	Message string
}

type BSTipSet struct {
	Blocks []*types.BlockHeader

	BlsMessages    []*types.Message
	BlsMsgIncludes [][]uint64

	SecpkMessages    []*types.SignedMessage
	SecpkMsgIncludes [][]uint64
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
	if len(req.Start) == 0 {
		return &BlockSyncResponse{
			Status:  204,
			Message: "no cids given in blocksync request",
		}, nil
	}

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
			bmsgs, bmincl, smsgs, smincl, err := bss.gatherMessages(ts)
			if err != nil {
				return nil, xerrors.Errorf("gather messages failed: %w", err)
			}

			bst.BlsMessages = bmsgs
			bst.BlsMsgIncludes = bmincl
			bst.SecpkMessages = smsgs
			bst.SecpkMsgIncludes = smincl
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

func (bss *BlockSyncService) gatherMessages(ts *types.TipSet) ([]*types.Message, [][]uint64, []*types.SignedMessage, [][]uint64, error) {
	blsmsgmap := make(map[cid.Cid]uint64)
	secpkmsgmap := make(map[cid.Cid]uint64)
	var secpkmsgs []*types.SignedMessage
	var blsmsgs []*types.Message
	var secpkincl, blsincl [][]uint64

	for _, b := range ts.Blocks() {
		bmsgs, smsgs, err := bss.cs.MessagesForBlock(b)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		bmi := make([]uint64, 0, len(bmsgs))
		for _, m := range bmsgs {
			i, ok := blsmsgmap[m.Cid()]
			if !ok {
				i = uint64(len(blsmsgs))
				blsmsgs = append(blsmsgs, m)
				blsmsgmap[m.Cid()] = i
			}

			bmi = append(bmi, i)
		}
		blsincl = append(blsincl, bmi)

		smi := make([]uint64, 0, len(smsgs))
		for _, m := range smsgs {
			i, ok := secpkmsgmap[m.Cid()]
			if !ok {
				i = uint64(len(secpkmsgs))
				secpkmsgs = append(secpkmsgs, m)
				secpkmsgmap[m.Cid()] = i
			}

			smi = append(smi, i)
		}
		secpkincl = append(secpkincl, smi)
	}

	return blsmsgs, blsincl, secpkmsgs, secpkincl, nil
}

type BlockSync struct {
	bserv     bserv.BlockService
	newStream NewStreamFunc

	syncPeersLk sync.Mutex
	syncPeers   map[peer.ID]struct{}
}

func NewBlockSyncClient(bserv dtypes.ChainBlockService, h host.Host) *BlockSync {
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

func (bs *BlockSync) processStatus(req *BlockSyncRequest, res *BlockSyncResponse) error {
	switch res.Status {
	case 101: // Partial Response
		panic("not handled")
	case 201: // req.Start not found
		return fmt.Errorf("not found")
	case 202: // Go Away
		panic("not handled")
	case 203: // Internal Error
		return fmt.Errorf("block sync peer errored: %s", res.Message)
	case 204:
		return fmt.Errorf("block sync request invalid: %s", res.Message)
	default:
		return fmt.Errorf("unrecognized response code: %d", res.Status)
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

	var oerr error
	for _, p := range perm {
		res, err := bs.sendRequestToPeer(ctx, peers[p], req)
		if err != nil {
			oerr = err
			log.Warnf("BlockSync request failed for peer %s: %s", peers[p].String(), err)
			continue
		}

		if res.Status == 0 {
			return bs.processBlocksResponse(req, res)
		}
		oerr = bs.processStatus(req, res)
		if oerr != nil {
			log.Warnf("BlockSync peer %s response was an error: %s", peers[p].String(), err)
		}
	}
	return nil, xerrors.Errorf("GetBlocks failed with all peers: %w", oerr)
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
		return nil, fmt.Errorf("block sync peer errored: %q", res.Message)
	case 204: // Invalid Request
		return nil, fmt.Errorf("block sync request invalid: %q", res.Message)
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
		Options:       BSOptMessages | BSOptBlocks,
	}

	var err error
	for _, p := range perm {
		res, err := bs.sendRequestToPeer(ctx, peers[p], req)
		if err != nil {
			log.Warnf("BlockSync request failed for peer %s: %s", peers[p].String(), err)
			continue
		}

		if res.Status == 0 {
			return res.Chain, nil
		}
		err = bs.processStatus(req, res)
		if err != nil {
			log.Warnf("BlockSync peer %s response was an error: %s", peers[p].String(), err)
		}
	}

	// TODO: What if we have no peers (and err is nil)?
	return nil, xerrors.Errorf("GetChainMessages failed with all peers(%d): %w", len(peers), err)
}

func bstsToFullTipSet(bts *BSTipSet) (*store.FullTipSet, error) {
	fts := &store.FullTipSet{}
	for i, b := range bts.Blocks {
		fb := &types.FullBlock{
			Header: b,
		}
		for _, mi := range bts.BlsMsgIncludes[i] {
			fb.BlsMessages = append(fb.BlsMessages, bts.BlsMessages[mi])
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

		if !types.CidArrsEqual(cur.Parents(), nts.Cids()) {
			return nil, fmt.Errorf("parents of tipset[%d] were not tipset[%d]", bi-1, bi)
		}

		out = append(out, nts)
		cur = nts
	}
	return out, nil
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

func (bs *BlockSync) FetchMessagesByCids(ctx context.Context, cids []cid.Cid) ([]*types.Message, error) {
	out := make([]*types.Message, len(cids))

	err := bs.fetchCids(ctx, cids, func(i int, b blocks.Block) error {
		msg, err := types.DecodeMessage(b.RawData())
		if err != nil {
			return err
		}

		if out[i] != nil {
			return fmt.Errorf("received duplicate message")
		}

		out[i] = msg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (bs *BlockSync) FetchSignedMessagesByCids(ctx context.Context, cids []cid.Cid) ([]*types.SignedMessage, error) {
	out := make([]*types.SignedMessage, len(cids))

	err := bs.fetchCids(ctx, cids, func(i int, b blocks.Block) error {
		smsg, err := types.DecodeSignedMessage(b.RawData())
		if err != nil {
			return err
		}

		if out[i] != nil {
			return fmt.Errorf("received duplicate message")
		}

		out[i] = smsg
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (bs *BlockSync) fetchCids(ctx context.Context, cids []cid.Cid, cb func(int, blocks.Block) error) error {
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

				return fmt.Errorf("failed to fetch all messages")
			}

			ix, ok := m[v.Cid()]
			if !ok {
				return fmt.Errorf("received message we didnt ask for")
			}

			if err := cb(ix, v); err != nil {
				return err
			}
		}
	}

	return nil
}

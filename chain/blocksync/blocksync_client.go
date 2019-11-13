package blocksync

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	bserv "github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	host "github.com/libp2p/go-libp2p-host"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/cborutil"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

type BlockSync struct {
	bserv bserv.BlockService
	host  host.Host

	syncPeersLk sync.Mutex
	syncPeers   *bsPeerTracker
}

func NewBlockSyncClient(bserv dtypes.ChainBlockService, h host.Host) *BlockSync {
	return &BlockSync{
		bserv:     bserv,
		host:      h,
		syncPeers: newPeerTracker(),
	}
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
	ctx, span := trace.StartSpan(ctx, "bsync.GetBlocks")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("tipset", fmt.Sprint(tipset)),
			trace.Int64Attribute("count", int64(count)),
		)
	}

	req := &BlockSyncRequest{
		Start:         tipset,
		RequestLength: uint64(count),
		Options:       BSOptBlocks,
	}

	peers := bs.getPeers()

	var oerr error
	for _, p := range peers {
		// TODO: doing this synchronously isnt great, but fetching in parallel
		// may not be a good idea either. think about this more
		select {
		case <-ctx.Done():
			return nil, xerrors.Errorf("blocksync getblocks failed: %w", ctx.Err())
		default:
		}

		res, err := bs.sendRequestToPeer(ctx, p, req)
		if err != nil {
			oerr = err
			log.Warnf("BlockSync request failed for peer %s: %s", p.String(), err)
			continue
		}

		if res.Status == 0 {
			return bs.processBlocksResponse(req, res)
		}
		oerr = bs.processStatus(req, res)
		if oerr != nil {
			log.Warnf("BlockSync peer %s response was an error: %s", p.String(), oerr)
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
		return nil, xerrors.Errorf("partial responses are not handled")
	case 201: // req.Start not found
		return nil, fmt.Errorf("not found")
	case 202: // Go Away
		return nil, xerrors.Errorf("received 'go away' response peer")
	case 203: // Internal Error
		return nil, fmt.Errorf("block sync peer errored: %q", res.Message)
	case 204: // Invalid Request
		return nil, fmt.Errorf("block sync request invalid: %q", res.Message)
	default:
		return nil, fmt.Errorf("unrecognized response code")
	}
}

func (bs *BlockSync) GetChainMessages(ctx context.Context, h *types.TipSet, count uint64) ([]*BSTipSet, error) {
	ctx, span := trace.StartSpan(ctx, "GetChainMessages")
	defer span.End()

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

func (bs *BlockSync) sendRequestToPeer(ctx context.Context, p peer.ID, req *BlockSyncRequest) (*BlockSyncResponse, error) {
	ctx, span := trace.StartSpan(ctx, "sendRequestToPeer")
	defer span.End()

	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("peer", p.Pretty()),
		)
	}

	s, err := bs.host.NewStream(inet.WithNoDial(ctx, "should already have connection"), p, BlockSyncProtocolID)
	if err != nil {
		bs.RemovePeer(p)
		return nil, xerrors.Errorf("failed to open stream to peer: %w", err)
	}

	if err := cborutil.WriteCborRPC(s, req); err != nil {
		return nil, err
	}

	var res BlockSyncResponse
	if err := cborutil.ReadCborRPC(bufio.NewReader(s), &res); err != nil {
		return nil, err
	}

	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.Int64Attribute("resp_status", int64(res.Status)),
			trace.StringAttribute("msg", res.Message),
			trace.Int64Attribute("chain_len", int64(len(res.Chain))),
		)
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
	bs.syncPeers.addPeer(p)
}

func (bs *BlockSync) RemovePeer(p peer.ID) {
	bs.syncPeers.removePeer(p)
}

func (bs *BlockSync) getPeers() []peer.ID {
	return bs.syncPeers.prefSortedPeers()
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

type peerStats struct {
	successes int
	failures  int
	firstSeen time.Time
}

type bsPeerTracker struct {
	peers map[peer.ID]*peerStats
	lk    sync.Mutex
}

func newPeerTracker() *bsPeerTracker {
	return &bsPeerTracker{
		peers: make(map[peer.ID]*peerStats),
	}
}
func (bpt *bsPeerTracker) addPeer(p peer.ID) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	if _, ok := bpt.peers[p]; ok {
		return
	}
	bpt.peers[p] = &peerStats{
		firstSeen: time.Now(),
	}

}

func (bpt *bsPeerTracker) prefSortedPeers() []peer.ID {
	// TODO: this could probably be cached, but as long as its not too many peers, fine for now
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	out := make([]peer.ID, 0, len(bpt.peers))
	for p := range bpt.peers {
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		pi := bpt.peers[out[i]]
		pj := bpt.peers[out[j]]
		if pi.successes > pj.successes {
			return true
		}
		if pi.failures < pj.successes {
			return true
		}
		return pi.firstSeen.Before(pj.firstSeen)
	})

	return out
}

func (bpt *bsPeerTracker) logSuccess(p peer.ID) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	if pi, ok := bpt.peers[p]; !ok {
		log.Warn("log success called on peer not in tracker")
		return
	} else {
		pi.successes++
	}
}

func (bpt *bsPeerTracker) logFailure(p peer.ID) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	if pi, ok := bpt.peers[p]; !ok {
		log.Warn("log failure called on peer not in tracker")
		return
	} else {
		pi.failures++
	}
}

func (bpt *bsPeerTracker) removePeer(p peer.ID) {
	bpt.lk.Lock()
	defer bpt.lk.Unlock()
	delete(bpt.peers, p)
}

package blocksync

import (
	"bufio"
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/protocol"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("blocksync")

type NewStreamFunc func(context.Context, peer.ID, ...protocol.ID) (inet.Stream, error)

const BlockSyncProtocolID = "/fil/sync/blk/0.0.1"

const BlockSyncMaxRequestLength = 800

// BlockSyncService is the component that services BlockSync requests from
// peers.
//
// BlockSync is the basic chain synchronization protocol of Filecoin. BlockSync
// is an RPC-oriented protocol, with a single operation to request blocks.
//
// A request contains a start anchor block (referred to with a CID), and a
// amount of blocks requested beyond the anchor (including the anchor itself).
//
// A client can also pass options, encoded as a 64-bit bitfield. Lotus supports
// two options at the moment:
//
//  - include block contents
//  - include block messages
//
// The response will include a status code, an optional message, and the
// response payload in case of success. The payload is a slice of serialized
// tipsets.
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
	BSOptBlocks = 1 << iota
	BSOptMessages
)

const (
	StatusOK            = uint64(0)
	StatusPartial       = uint64(101)
	StatusNotFound      = uint64(201)
	StatusGoAway        = uint64(202)
	StatusInternalError = uint64(203)
	StatusBadRequest    = uint64(204)
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
	ctx, span := trace.StartSpan(context.Background(), "blocksync.HandleStream")
	defer span.End()

	defer s.Close() //nolint:errcheck

	var req BlockSyncRequest
	if err := cborutil.ReadCborRPC(bufio.NewReader(s), &req); err != nil {
		log.Warnf("failed to read block sync request: %s", err)
		return
	}
	log.Infow("block sync request", "start", req.Start, "len", req.RequestLength)

	resp, err := bss.processRequest(ctx, s.Conn().RemotePeer(), &req)
	if err != nil {
		log.Warn("failed to process block sync request: ", err)
		return
	}

	writeDeadline := 60 * time.Second
	_ = s.SetDeadline(time.Now().Add(writeDeadline))
	if err := cborutil.WriteCborRPC(s, resp); err != nil {
		log.Warnw("failed to write back response for handle stream", "err", err, "peer", s.Conn().RemotePeer())
		return
	}
}

func (bss *BlockSyncService) processRequest(ctx context.Context, p peer.ID, req *BlockSyncRequest) (*BlockSyncResponse, error) {
	_, span := trace.StartSpan(ctx, "blocksync.ProcessRequest")
	defer span.End()

	opts := ParseBSOptions(req.Options)
	if len(req.Start) == 0 {
		return &BlockSyncResponse{
			Status:  StatusBadRequest,
			Message: "no cids given in blocksync request",
		}, nil
	}

	span.AddAttributes(
		trace.BoolAttribute("blocks", opts.IncludeBlocks),
		trace.BoolAttribute("messages", opts.IncludeMessages),
		trace.Int64Attribute("reqlen", int64(req.RequestLength)),
	)

	reqlen := req.RequestLength
	if reqlen > BlockSyncMaxRequestLength {
		log.Warnw("limiting blocksync request length", "orig", req.RequestLength, "peer", p)
		reqlen = BlockSyncMaxRequestLength
	}

	chain, err := collectChainSegment(bss.cs, types.NewTipSetKey(req.Start...), reqlen, opts)
	if err != nil {
		log.Warn("encountered error while responding to block sync request: ", err)
		return &BlockSyncResponse{
			Status:  StatusInternalError,
			Message: err.Error(),
		}, nil
	}

	status := StatusOK
	if reqlen < req.RequestLength {
		status = StatusPartial
	}

	return &BlockSyncResponse{
		Chain:  chain,
		Status: status,
	}, nil
}

func collectChainSegment(cs *store.ChainStore, start types.TipSetKey, length uint64, opts *BSOptions) ([]*BSTipSet, error) {
	var bstips []*BSTipSet
	cur := start
	for {
		var bst BSTipSet
		ts, err := cs.LoadTipSet(cur)
		if err != nil {
			return nil, xerrors.Errorf("failed loading tipset %s: %w", cur, err)
		}

		if opts.IncludeMessages {
			bmsgs, bmincl, smsgs, smincl, err := gatherMessages(cs, ts)
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

func gatherMessages(cs *store.ChainStore, ts *types.TipSet) ([]*types.Message, [][]uint64, []*types.SignedMessage, [][]uint64, error) {
	blsmsgmap := make(map[cid.Cid]uint64)
	secpkmsgmap := make(map[cid.Cid]uint64)
	var secpkmsgs []*types.SignedMessage
	var blsmsgs []*types.Message
	var secpkincl, blsincl [][]uint64

	for _, b := range ts.Blocks() {
		bmsgs, smsgs, err := cs.MessagesForBlock(b)
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

func bstsToFullTipSet(bts *BSTipSet) (*store.FullTipSet, error) {
	fts := &store.FullTipSet{}
	for i, b := range bts.Blocks {
		fb := &types.FullBlock{
			Header: b,
		}
		for _, mi := range bts.BlsMsgIncludes[i] {
			fb.BlsMessages = append(fb.BlsMessages, bts.BlsMessages[mi])
		}
		for _, mi := range bts.SecpkMsgIncludes[i] {
			fb.SecpkMessages = append(fb.SecpkMessages, bts.SecpkMessages[mi])
		}

		fts.Blocks = append(fts.Blocks, fb)
	}

	return fts, nil
}

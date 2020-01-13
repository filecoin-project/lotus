package blocksync

import (
	"bufio"
	"context"

	"github.com/libp2p/go-libp2p-core/protocol"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	inet "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("blocksync")

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
	ctx, span := trace.StartSpan(context.Background(), "blocksync.HandleStream")
	defer span.End()

	defer s.Close()

	var req BlockSyncRequest
	if err := cborutil.ReadCborRPC(bufio.NewReader(s), &req); err != nil {
		log.Warnf("failed to read block sync request: %s", err)
		return
	}
	log.Infof("block sync request for: %s %d", req.Start, req.RequestLength)

	resp, err := bss.processRequest(ctx, &req)
	if err != nil {
		log.Warn("failed to process block sync request: ", err)
		return
	}

	if err := cborutil.WriteCborRPC(s, resp); err != nil {
		log.Warn("failed to write back response for handle stream: ", err)
		return
	}
}

func (bss *BlockSyncService) processRequest(ctx context.Context, req *BlockSyncRequest) (*BlockSyncResponse, error) {
	_, span := trace.StartSpan(ctx, "blocksync.ProcessRequest")
	defer span.End()

	opts := ParseBSOptions(req.Options)
	if len(req.Start) == 0 {
		return &BlockSyncResponse{
			Status:  204,
			Message: "no cids given in blocksync request",
		}, nil
	}

	span.AddAttributes(
		trace.BoolAttribute("blocks", opts.IncludeBlocks),
		trace.BoolAttribute("messages", opts.IncludeMessages),
	)

	chain, err := bss.collectChainSegment(types.NewTipSetKey(req.Start...), req.RequestLength, opts)
	if err != nil {
		log.Warn("encountered error while responding to block sync request: ", err)
		return &BlockSyncResponse{
			Status:  203,
			Message: err.Error(),
		}, nil
	}

	return &BlockSyncResponse{
		Chain:  chain,
		Status: 0,
	}, nil
}

func (bss *BlockSyncService) collectChainSegment(start types.TipSetKey, length uint64, opts *BSOptions) ([]*BSTipSet, error) {
	var bstips []*BSTipSet
	cur := start
	for {
		var bst BSTipSet
		ts, err := bss.cs.LoadTipSet(cur)
		if err != nil {
			return nil, xerrors.Errorf("failed loading tipset %s: %w", cur, err)
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

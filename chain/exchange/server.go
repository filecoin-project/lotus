package exchange

import (
	"bufio"
	"context"
	"fmt"
	"time"

	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	cborutil "github.com/filecoin-project/go-cbor-util"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/helpers"
	inet "github.com/libp2p/go-libp2p-core/network"
)

// server implements exchange.Server. It services requests for the
// libp2p ChainExchange protocol.
type server struct {
	cs *store.ChainStore
}

var _ Server = (*server)(nil)

// NewServer creates a new libp2p-based exchange.Server. It services requests
// for the libp2p ChainExchange protocol.
func NewServer(cs *store.ChainStore) Server {
	return &server{
		cs: cs,
	}
}

// HandleStream implements Server.HandleStream. Refer to the godocs there.
func (s *server) HandleStream(stream inet.Stream) {
	ctx, span := trace.StartSpan(context.Background(), "chainxchg.HandleStream")
	defer span.End()

	// Note: this will become just stream.Close once we've completed the go-libp2p migration to
	//       go-libp2p-core 0.7.0
	defer helpers.FullClose(stream) //nolint:errcheck

	var req Request
	if err := cborutil.ReadCborRPC(bufio.NewReader(stream), &req); err != nil {
		log.Warnf("failed to read block sync request: %s", err)
		return
	}
	log.Infow("block sync request",
		"start", req.Head, "len", req.Length)

	resp, err := s.processRequest(ctx, &req)
	if err != nil {
		log.Warn("failed to process request: ", err)
		return
	}

	_ = stream.SetDeadline(time.Now().Add(WriteResDeadline))
	if err := cborutil.WriteCborRPC(stream, resp); err != nil {
		_ = stream.SetDeadline(time.Time{})
		log.Warnw("failed to write back response for handle stream",
			"err", err, "peer", stream.Conn().RemotePeer())
		return
	}
	_ = stream.SetDeadline(time.Time{})
}

// Validate and service the request. We return either a protocol
// response or an internal error.
func (s *server) processRequest(ctx context.Context, req *Request) (*Response, error) {
	validReq, errResponse := validateRequest(ctx, req)
	if errResponse != nil {
		// The request did not pass validation, return the response
		//  indicating it.
		return errResponse, nil
	}

	return s.serviceRequest(ctx, validReq)
}

// Validate request. We either return a `validatedRequest`, or an error
// `Response` indicating why we can't process it. We do not return any
// internal errors here, we just signal protocol ones.
func validateRequest(ctx context.Context, req *Request) (*validatedRequest, *Response) {
	_, span := trace.StartSpan(ctx, "chainxchg.ValidateRequest")
	defer span.End()

	validReq := validatedRequest{}

	validReq.options = parseOptions(req.Options)
	if validReq.options.noOptionsSet() {
		return nil, &Response{
			Status:       BadRequest,
			ErrorMessage: "no options set",
		}
	}

	validReq.length = req.Length
	if validReq.length > MaxRequestLength {
		return nil, &Response{
			Status: BadRequest,
			ErrorMessage: fmt.Sprintf("request length over maximum allowed (%d)",
				MaxRequestLength),
		}
	}
	if validReq.length == 0 {
		return nil, &Response{
			Status:       BadRequest,
			ErrorMessage: "invalid request length of zero",
		}
	}

	if len(req.Head) == 0 {
		return nil, &Response{
			Status:       BadRequest,
			ErrorMessage: "no cids in request",
		}
	}
	validReq.head = types.NewTipSetKey(req.Head...)

	// FIXME: Add as a defer at the start.
	span.AddAttributes(
		trace.BoolAttribute("blocks", validReq.options.IncludeHeaders),
		trace.BoolAttribute("messages", validReq.options.IncludeMessages),
		trace.Int64Attribute("reqlen", int64(validReq.length)),
	)

	return &validReq, nil
}

func (s *server) serviceRequest(ctx context.Context, req *validatedRequest) (*Response, error) {
	_, span := trace.StartSpan(ctx, "chainxchg.ServiceRequest")
	defer span.End()

	chain, err := collectChainSegment(s.cs, req)
	if err != nil {
		log.Warn("block sync request: collectChainSegment failed: ", err)
		return &Response{
			Status:       InternalError,
			ErrorMessage: err.Error(),
		}, nil
	}

	status := Ok
	if len(chain) < int(req.length) {
		status = Partial
	}

	return &Response{
		Chain:  chain,
		Status: status,
	}, nil
}

func collectChainSegment(cs *store.ChainStore, req *validatedRequest) ([]*BSTipSet, error) {
	var bstips []*BSTipSet

	cur := req.head
	for {
		var bst BSTipSet
		ts, err := cs.LoadTipSet(cur)
		if err != nil {
			return nil, xerrors.Errorf("failed loading tipset %s: %w", cur, err)
		}

		if req.options.IncludeHeaders {
			bst.Blocks = ts.Blocks()
		}

		if req.options.IncludeMessages {
			bmsgs, bmincl, smsgs, smincl, err := gatherMessages(cs, ts)
			if err != nil {
				return nil, xerrors.Errorf("gather messages failed: %w", err)
			}

			// FIXME: Pass the response to `gatherMessages()` and set all this there.
			bst.Messages = &CompactedMessages{}
			bst.Messages.Bls = bmsgs
			bst.Messages.BlsIncludes = bmincl
			bst.Messages.Secpk = smsgs
			bst.Messages.SecpkIncludes = smincl
		}

		bstips = append(bstips, &bst)

		// If we collected the length requested or if we reached the
		// start (genesis), then stop.
		if uint64(len(bstips)) >= req.length || ts.Height() == 0 {
			return bstips, nil
		}

		cur = ts.Parents()
	}
}

func gatherMessages(cs *store.ChainStore, ts *types.TipSet) ([]*types.Message, [][]uint64, []*types.SignedMessage, [][]uint64, error) {
	blsmsgmap := make(map[cid.Cid]uint64)
	secpkmsgmap := make(map[cid.Cid]uint64)
	var secpkincl, blsincl [][]uint64

	var blscids, secpkcids []cid.Cid
	for _, block := range ts.Blocks() {
		bc, sc, err := cs.ReadMsgMetaCids(block.Messages)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		// FIXME: DRY. Use `chain.Message` interface.
		bmi := make([]uint64, 0, len(bc))
		for _, m := range bc {
			i, ok := blsmsgmap[m]
			if !ok {
				i = uint64(len(blscids))
				blscids = append(blscids, m)
				blsmsgmap[m] = i
			}

			bmi = append(bmi, i)
		}
		blsincl = append(blsincl, bmi)

		smi := make([]uint64, 0, len(sc))
		for _, m := range sc {
			i, ok := secpkmsgmap[m]
			if !ok {
				i = uint64(len(secpkcids))
				secpkcids = append(secpkcids, m)
				secpkmsgmap[m] = i
			}

			smi = append(smi, i)
		}
		secpkincl = append(secpkincl, smi)
	}

	blsmsgs, err := cs.LoadMessagesFromCids(blscids)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	secpkmsgs, err := cs.LoadSignedMessagesFromCids(secpkcids)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return blsmsgs, blsincl, secpkmsgs, secpkincl, nil
}
